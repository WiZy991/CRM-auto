-- ===========================================================================
--  Миграция 4. Каталог автомобилей.
--
--  Деньги хранятся в минорных единицах (копейки, центы, фэни) типом bigint.
--  numeric для денег в горячем каталоге дороже по сравнению и сортировке,
--  а float для денег недопустим.
--
--  Дополнительно ведётся price_rub_minor — цена, приведённая к рублям на
--  момент публикации. Без неё сортировка по цене в смешанной валюте
--  требовала бы пересчёта на каждый запрос каталога.
-- ===========================================================================

CREATE TABLE cars (
    id            uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    dealer_id     uuid           REFERENCES users(id) ON DELETE CASCADE,
    seller_id     uuid           REFERENCES sellers(id) ON DELETE SET NULL,

    status        car_status     NOT NULL DEFAULT 'draft',
    origin        origin_country NOT NULL,

    brand         text           NOT NULL,
    model         text           NOT NULL,
    generation     text,
    trim_level    text,
    year          smallint       NOT NULL,

    mileage_km    integer        NOT NULL DEFAULT 0,
    engine_cc     integer,
    power_hp      smallint,
    fuel          fuel_type      NOT NULL,
    gearbox       transmission   NOT NULL,
    drive         drivetrain     NOT NULL,
    body          body_type      NOT NULL,
    color         text,
    seats         smallint,
    steering_right boolean       NOT NULL DEFAULT false,

    -- Оценка аукционного листа: '4.5' по кузову и 'B' по салону.
    -- Для китайских авто обычно пусто, для японских — ключевой параметр выбора.
    auction_grade      text,
    interior_grade     text,
    auction_lot_number text,
    auction_date       date,

    vin            text,
    -- VIN виден только участникам сделки; в публичном каталоге отдаётся маска.
    vin_visible    boolean       NOT NULL DEFAULT false,

    price_minor      bigint       NOT NULL,
    currency         currency_code NOT NULL DEFAULT 'rub',
    price_rub_minor  bigint       NOT NULL,
    -- Цена «под ключ во Владивостоке»: с доставкой и растаможкой.
    turnkey_rub_minor bigint,
    customs_rub_minor bigint,
    delivery_days     smallint,

    title         text           NOT NULL,
    description   text           NOT NULL DEFAULT '',
    equipment     jsonb          NOT NULL DEFAULT '[]'::jsonb,

    views_count   integer        NOT NULL DEFAULT 0,
    requests_count integer       NOT NULL DEFAULT 0,

    search_vector tsvector,

    published_at  timestamptz,
    sold_at       timestamptz,
    created_at    timestamptz    NOT NULL DEFAULT now(),
    updated_at    timestamptz    NOT NULL DEFAULT now(),

    CONSTRAINT cars_year_range CHECK (year BETWEEN 1980 AND 2100),
    CONSTRAINT cars_mileage_sane CHECK (mileage_km BETWEEN 0 AND 2000000),
    CONSTRAINT cars_price_positive CHECK (price_minor > 0 AND price_rub_minor > 0),
    CONSTRAINT cars_engine_sane CHECK (engine_cc IS NULL OR engine_cc BETWEEN 0 AND 10000),
    CONSTRAINT cars_power_sane CHECK (power_hp IS NULL OR power_hp BETWEEN 0 AND 2000),
    CONSTRAINT cars_vin_format CHECK (vin IS NULL OR vin ~ '^[A-HJ-NPR-Z0-9]{11,17}$'),
    CONSTRAINT cars_title_len CHECK (length(title) BETWEEN 4 AND 200),
    CONSTRAINT cars_description_len CHECK (length(description) <= 8000),
    -- Объявление принадлежит либо дилеру, либо зарубежному продавцу,
    -- но не может быть «бесхозным».
    CONSTRAINT cars_owner_present CHECK (dealer_id IS NOT NULL OR seller_id IS NOT NULL)
);

-- --- Индексы под фильтры каталога ------------------------------------------
-- Частичные индексы только по активным объявлениям: черновики и проданные
-- машины составляют большую часть таблицы со временем, но в каталог не попадают.
CREATE INDEX cars_catalog_main_idx
    ON cars (origin, brand, price_rub_minor)
    WHERE status = 'active';

CREATE INDEX cars_catalog_fresh_idx
    ON cars (published_at DESC)
    WHERE status = 'active';

CREATE INDEX cars_catalog_price_idx
    ON cars (price_rub_minor, id)
    WHERE status = 'active';

CREATE INDEX cars_catalog_year_mileage_idx
    ON cars (year DESC, mileage_km)
    WHERE status = 'active';

CREATE INDEX cars_body_gearbox_drive_idx
    ON cars (body, gearbox, drive)
    WHERE status = 'active';

CREATE INDEX cars_dealer_status_idx ON cars (dealer_id, status, updated_at DESC);
CREATE INDEX cars_seller_idx ON cars (seller_id) WHERE seller_id IS NOT NULL;
CREATE INDEX cars_search_gin ON cars USING gin (search_vector);
CREATE INDEX cars_brand_model_trgm ON cars USING gin ((brand || ' ' || model) gin_trgm_ops);
-- VIN должен быть уникален среди живых объявлений: одна машина не может
-- продаваться двумя дилерами одновременно.
CREATE UNIQUE INDEX cars_vin_unique
    ON cars (vin)
    WHERE vin IS NOT NULL AND status NOT IN ('archived', 'sold');

CREATE TRIGGER cars_set_updated_at
    BEFORE UPDATE ON cars
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE FUNCTION cars_refresh_search_vector()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('simple', coalesce(NEW.brand, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.model, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.generation, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.trim_level, '')), 'B') ||
        setweight(to_tsvector('russian', coalesce(NEW.title, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(NEW.year::text, '')), 'C') ||
        setweight(to_tsvector('russian', coalesce(NEW.description, '')), 'D');
    RETURN NEW;
END;
$$;

CREATE TRIGGER cars_search_vector_trigger
    BEFORE INSERT OR UPDATE OF brand, model, generation, trim_level, title, year, description ON cars
    FOR EACH ROW EXECUTE FUNCTION cars_refresh_search_vector();

-- ---------------------------------------------------------------------------
--  Фотографии.
--  Загруженные файлы всегда перекодируются приложением, поэтому здесь
--  хранятся только пути к уже безопасным изображениям.
-- ---------------------------------------------------------------------------
CREATE TABLE car_photos (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    car_id     uuid        NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    url        text        NOT NULL,
    thumb_url  text,
    width      smallint,
    height     smallint,
    bytes      integer,
    sort_order smallint    NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX car_photos_car_order_idx ON car_photos (car_id, sort_order);

CREATE TABLE favorites (
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    car_id     uuid        NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, car_id)
);

CREATE INDEX favorites_car_idx ON favorites (car_id);

-- ---------------------------------------------------------------------------
--  Сохранённые поисковые запросы клиента с уведомлением о новых машинах.
-- ---------------------------------------------------------------------------
CREATE TABLE saved_searches (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text        NOT NULL,
    filters    jsonb       NOT NULL,
    notify     boolean     NOT NULL DEFAULT true,
    last_notified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT saved_searches_title_len CHECK (length(title) BETWEEN 1 AND 120)
);

CREATE INDEX saved_searches_user_idx ON saved_searches (user_id);
