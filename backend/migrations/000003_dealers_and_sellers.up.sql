-- ===========================================================================
--  Миграция 3. Профили дилеров и база продавцов Китая и Японии.
--
--  Разделение важное: dealer_profiles — российские дилеры-импортёры,
--  зарегистрированные на платформе; sellers — справочник зарубежных
--  поставщиков (аукционы, экспортёры, заводы), который может пополняться
--  как самими продавцами, так и дилерами или администраторами.
-- ===========================================================================

CREATE TABLE dealer_profiles (
    user_id        uuid        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    slug           text        NOT NULL,
    company_name   text        NOT NULL,
    legal_name     text,
    inn            text,
    city           text        NOT NULL,
    address        text,
    description    text        NOT NULL DEFAULT '',
    logo_url       text,
    cover_url      text,
    website        text,

    -- Услуги дилера: подбор, выкуп с аукциона, доставка, растаможка и т.д.
    -- jsonb, потому что набор услуг у каждого дилера свой и меняется чаще,
    -- чем допустимо менять схему.
    services       jsonb       NOT NULL DEFAULT '[]'::jsonb,
    work_countries origin_country[] NOT NULL DEFAULT ARRAY['cn','jp']::origin_country[],

    -- Денормализованные агрегаты. Пересчитываются фоновой задачей и при
    -- закрытии сделки: считать AVG по отзывам на каждый показ каталога дорого.
    rating_avg     numeric(3,2) NOT NULL DEFAULT 0,
    rating_count   integer      NOT NULL DEFAULT 0,
    deals_won      integer      NOT NULL DEFAULT 0,
    deals_total    integer      NOT NULL DEFAULT 0,
    avg_lead_days  numeric(5,1),

    verified_at    timestamptz,
    verified_by    uuid        REFERENCES users(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT dealer_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    CONSTRAINT dealer_inn_format CHECK (inn IS NULL OR inn ~ '^[0-9]{10}$|^[0-9]{12}$'),
    CONSTRAINT dealer_rating_range CHECK (rating_avg >= 0 AND rating_avg <= 5)
);

CREATE UNIQUE INDEX dealer_profiles_slug_unique ON dealer_profiles (slug);
CREATE INDEX dealer_profiles_city_idx ON dealer_profiles (city);
CREATE INDEX dealer_profiles_rating_idx ON dealer_profiles (rating_avg DESC, rating_count DESC);
CREATE INDEX dealer_profiles_verified_idx ON dealer_profiles (verified_at) WHERE verified_at IS NOT NULL;

CREATE TRIGGER dealer_profiles_set_updated_at
    BEFORE UPDATE ON dealer_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  Справочник зарубежных продавцов (п. 3.6 ТЗ).
-- ---------------------------------------------------------------------------
CREATE TABLE sellers (
    id           uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Заполнен, если продавец сам зарегистрировался на платформе.
    user_id      uuid           UNIQUE REFERENCES users(id) ON DELETE SET NULL,
    created_by   uuid           REFERENCES users(id) ON DELETE SET NULL,

    country      origin_country NOT NULL,
    kind         seller_kind    NOT NULL,
    name         text           NOT NULL,
    name_local   text,
    region       text           NOT NULL,
    city         text,
    address      text,

    brands       text[]         NOT NULL DEFAULT '{}',
    description  text           NOT NULL DEFAULT '',
    website      text,
    -- Контакты в jsonb: у японских аукционов это ID и логин, у китайских
    -- экспортёров — WeChat и телефон. Единой структуры не существует.
    contacts     jsonb          NOT NULL DEFAULT '{}'::jsonb,
    logo_url     text,

    min_order_qty smallint      NOT NULL DEFAULT 1,
    export_experience_years smallint,

    rating_avg   numeric(3,2)   NOT NULL DEFAULT 0,
    rating_count integer        NOT NULL DEFAULT 0,

    verified_at  timestamptz,
    verified_by  uuid           REFERENCES users(id) ON DELETE SET NULL,
    is_active    boolean        NOT NULL DEFAULT true,

    search_vector tsvector,

    created_at   timestamptz    NOT NULL DEFAULT now(),
    updated_at   timestamptz    NOT NULL DEFAULT now(),

    CONSTRAINT sellers_name_len CHECK (length(name) BETWEEN 2 AND 200),
    CONSTRAINT sellers_brands_limit CHECK (array_length(brands, 1) IS NULL OR array_length(brands, 1) <= 60),
    CONSTRAINT sellers_rating_range CHECK (rating_avg >= 0 AND rating_avg <= 5)
);

-- Фильтрация из ТЗ: страна + регион + тип + бренды. Составной индекс покрывает
-- самый частый набор условий, GIN по массиву брендов — поиск «кто везёт Zeekr».
CREATE INDEX sellers_country_kind_region_idx ON sellers (country, kind, region) WHERE is_active;
CREATE INDEX sellers_brands_gin ON sellers USING gin (brands);
CREATE INDEX sellers_search_gin ON sellers USING gin (search_vector);
CREATE INDEX sellers_name_trgm ON sellers USING gin (name gin_trgm_ops);
CREATE INDEX sellers_rating_idx ON sellers (rating_avg DESC, rating_count DESC) WHERE is_active;

CREATE TRIGGER sellers_set_updated_at
    BEFORE UPDATE ON sellers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Поисковый вектор пересобирается триггером, а не приложением: любой путь
-- изменения строки (миграция, админка, seed) оставит индекс корректным.
CREATE OR REPLACE FUNCTION sellers_refresh_search_vector()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('simple', coalesce(NEW.name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.name_local, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(array_to_string(NEW.brands, ' '), '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.region, '')), 'C') ||
        setweight(to_tsvector('simple', coalesce(NEW.city, '')), 'C') ||
        setweight(to_tsvector('russian', coalesce(NEW.description, '')), 'D');
    RETURN NEW;
END;
$$;

CREATE TRIGGER sellers_search_vector_trigger
    BEFORE INSERT OR UPDATE OF name, name_local, brands, region, city, description ON sellers
    FOR EACH ROW EXECUTE FUNCTION sellers_refresh_search_vector();

-- ---------------------------------------------------------------------------
--  Отметки дилеров о работе с продавцами: личный список проверенных
--  поставщиков и приватные заметки, недоступные другим дилерам.
-- ---------------------------------------------------------------------------
CREATE TABLE dealer_seller_links (
    dealer_id  uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seller_id  uuid        NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
    note       text        NOT NULL DEFAULT '',
    is_trusted boolean     NOT NULL DEFAULT false,
    deals_count integer    NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (dealer_id, seller_id)
);

CREATE INDEX dealer_seller_links_seller_idx ON dealer_seller_links (seller_id);
