-- ===========================================================================
--  Миграция 6. Переписка по сделке, отзывы, рекламные баннеры,
--  уведомления и аудит.
-- ===========================================================================

CREATE TABLE deal_messages (
    id         bigserial   PRIMARY KEY,
    deal_id    uuid        NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body       text        NOT NULL,
    -- Системные сообщения («сделка перешла на этап Растаможка») пишутся
    -- в ту же ленту, чтобы клиент видел единую хронологию.
    is_system  boolean     NOT NULL DEFAULT false,
    attachment_url text,
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_messages_body_len CHECK (length(body) BETWEEN 1 AND 5000)
);

CREATE INDEX deal_messages_deal_idx ON deal_messages (deal_id, id DESC);
-- Счётчик непрочитанного в интерфейсе: только по нужным строкам.
CREATE INDEX deal_messages_unread_idx ON deal_messages (deal_id, author_id) WHERE read_at IS NULL;

-- ---------------------------------------------------------------------------
--  Отзывы. Один отзыв на сделку, и только по завершённой сделке — иначе
--  рейтинг дилеров можно накрутить пустыми сделками.
-- ---------------------------------------------------------------------------
CREATE TABLE reviews (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    deal_id    uuid        NOT NULL UNIQUE REFERENCES deals(id) ON DELETE CASCADE,
    dealer_id  uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating     smallint    NOT NULL,
    text       text        NOT NULL DEFAULT '',
    -- Оценки по отдельным аспектам: клиенту важны сроки и честность цены.
    rating_speed   smallint,
    rating_price   smallint,
    rating_support smallint,
    dealer_reply   text,
    replied_at     timestamptz,
    is_published   boolean  NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reviews_rating_range CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT reviews_aspects_range CHECK (
        (rating_speed   IS NULL OR rating_speed   BETWEEN 1 AND 5) AND
        (rating_price   IS NULL OR rating_price   BETWEEN 1 AND 5) AND
        (rating_support IS NULL OR rating_support BETWEEN 1 AND 5)
    ),
    CONSTRAINT reviews_text_len CHECK (length(text) <= 4000)
);

CREATE INDEX reviews_dealer_idx ON reviews (dealer_id, created_at DESC) WHERE is_published;
CREATE INDEX reviews_author_idx ON reviews (author_id);

CREATE TRIGGER reviews_set_updated_at
    BEFORE UPDATE ON reviews
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  Рекламные баннеры дилеров (п. 6 ТЗ).
--
--  Счётчики показов и кликов инкрементируются в Redis и сбрасываются в базу
--  фоновой задачей: писать UPDATE на каждый показ означало бы блокировки
--  на одной строке при любом заметном трафике.
-- ---------------------------------------------------------------------------
CREATE TABLE banners (
    id          uuid             PRIMARY KEY DEFAULT gen_random_uuid(),
    dealer_id   uuid             NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    placement   banner_placement NOT NULL,
    status      banner_status    NOT NULL DEFAULT 'draft',

    title       text             NOT NULL,
    subtitle    text             NOT NULL DEFAULT '',
    image_url   text             NOT NULL,
    image_mobile_url text,
    href        text             NOT NULL,
    cta_label   text             NOT NULL DEFAULT 'Подробнее',

    starts_at   timestamptz      NOT NULL,
    ends_at     timestamptz      NOT NULL,
    weight      smallint         NOT NULL DEFAULT 10,

    impressions bigint           NOT NULL DEFAULT 0,
    clicks      bigint           NOT NULL DEFAULT 0,

    moderated_by uuid            REFERENCES users(id) ON DELETE SET NULL,
    reject_reason text,

    created_at  timestamptz      NOT NULL DEFAULT now(),
    updated_at  timestamptz      NOT NULL DEFAULT now(),

    CONSTRAINT banners_period_order CHECK (starts_at < ends_at),
    CONSTRAINT banners_title_len CHECK (length(title) BETWEEN 2 AND 120),
    CONSTRAINT banners_weight_range CHECK (weight BETWEEN 1 AND 100),
    -- Ссылка обязана быть абсолютной http(s): защита от javascript: и data:
    -- в атрибуте href на публичной странице.
    CONSTRAINT banners_href_scheme CHECK (href ~* '^https?://')
);

CREATE INDEX banners_active_idx
    ON banners (placement, weight DESC)
    WHERE status = 'active';
CREATE INDEX banners_dealer_idx ON banners (dealer_id, created_at DESC);
CREATE INDEX banners_period_idx ON banners (ends_at) WHERE status = 'active';

CREATE TRIGGER banners_set_updated_at
    BEFORE UPDATE ON banners
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  Уведомления в интерфейсе.
-- ---------------------------------------------------------------------------
CREATE TABLE notifications (
    id         bigserial         PRIMARY KEY,
    user_id    uuid              NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       notification_kind NOT NULL,
    title      text              NOT NULL,
    body       text              NOT NULL DEFAULT '',
    link       text,
    payload    jsonb             NOT NULL DEFAULT '{}'::jsonb,
    read_at    timestamptz,
    created_at timestamptz       NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_unread_idx ON notifications (user_id, id DESC) WHERE read_at IS NULL;
CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);

-- ---------------------------------------------------------------------------
--  Аудит изменений.
--
--  Пишется для всех значимых мутаций: кто, что, когда и с какого адреса.
--  diff содержит только изменённые поля, персональные данные маскируются
--  на уровне приложения перед записью.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_log (
    id         bigserial   PRIMARY KEY,
    actor_id   uuid        REFERENCES users(id) ON DELETE SET NULL,
    actor_role user_role,
    action     text        NOT NULL,
    entity     text        NOT NULL,
    entity_id  text,
    diff       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ip         inet,
    user_agent text,
    request_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id, created_at DESC);
CREATE INDEX audit_log_entity_idx ON audit_log (entity, entity_id, created_at DESC);
