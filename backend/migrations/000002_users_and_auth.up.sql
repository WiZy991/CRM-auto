-- ===========================================================================
--  Миграция 2. Пользователи, верификация, сессии, антибрутфорс.
-- ===========================================================================

CREATE TABLE users (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    role             user_role   NOT NULL,
    status           user_status NOT NULL DEFAULT 'pending',

    email            citext      NOT NULL,
    phone            text        NOT NULL,
    password_hash    text        NOT NULL,

    full_name        text        NOT NULL,
    -- Персональные данные повышенной чувствительности хранятся зашифрованными
    -- на стороне приложения (AES-256-GCM), база видит только байты.
    passport_encrypted bytea,
    address_encrypted  bytea,

    avatar_url       text,
    locale           text        NOT NULL DEFAULT 'ru',

    email_verified_at timestamptz,
    phone_verified_at timestamptz,

    -- Счётчик неудачных входов и время блокировки: защита от подбора пароля
    -- живёт в базе, а не только в Redis, чтобы сброс кеша не снимал защиту.
    failed_login_count smallint   NOT NULL DEFAULT 0,
    locked_until       timestamptz,
    last_login_at      timestamptz,
    last_login_ip      inet,

    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz,

    CONSTRAINT users_email_format CHECK (email ~ '^[^@\s]+@[^@\s]+\.[a-zA-Z]{2,}$'),
    CONSTRAINT users_phone_digits CHECK (length(normalize_phone(phone)) BETWEEN 10 AND 15),
    CONSTRAINT users_full_name_len CHECK (length(full_name) BETWEEN 2 AND 160)
);

-- Уникальность только среди живых записей: удалённый аккаунт не блокирует
-- повторную регистрацию того же адреса.
CREATE UNIQUE INDEX users_email_unique ON users (email) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_phone_unique ON users (normalize_phone(phone)) WHERE deleted_at IS NULL;
CREATE INDEX users_role_status_idx ON users (role, status) WHERE deleted_at IS NULL;
CREATE INDEX users_created_at_idx ON users (created_at DESC);

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
--  Коды подтверждения email и телефона.
--  Хранится только хеш кода: утечка таблицы не позволяет подтвердить чужой
--  контакт. Попытки ввода ограничены счётчиком attempts.
-- ---------------------------------------------------------------------------
CREATE TABLE verification_codes (
    id           uuid           PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel      verify_channel NOT NULL,
    destination  text           NOT NULL,
    code_hash    text           NOT NULL,
    attempts     smallint       NOT NULL DEFAULT 0,
    max_attempts smallint       NOT NULL DEFAULT 5,
    expires_at   timestamptz    NOT NULL,
    consumed_at  timestamptz,
    created_at   timestamptz    NOT NULL DEFAULT now(),
    created_ip   inet
);

-- Активный код на канал существует ровно один: повторная отправка гасит
-- предыдущий, поэтому перебор нескольких кодов одновременно невозможен.
CREATE UNIQUE INDEX verification_codes_active_unique
    ON verification_codes (user_id, channel)
    WHERE consumed_at IS NULL;
CREATE INDEX verification_codes_expires_idx ON verification_codes (expires_at);

-- ---------------------------------------------------------------------------
--  Refresh-сессии.
--
--  token_hash — SHA-256 от токена, сам токен в базе не лежит.
--  Поля previous_id и revoked_reason реализуют детекцию повторного
--  использования: если пришёл уже отозванный refresh, вся цепочка семьи
--  токенов гасится, а пользователь получает уведомление о безопасности.
-- ---------------------------------------------------------------------------
CREATE TABLE refresh_sessions (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id      uuid        NOT NULL,
    user_id        uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash     bytea       NOT NULL,
    previous_id    uuid        REFERENCES refresh_sessions(id) ON DELETE SET NULL,

    user_agent     text        NOT NULL DEFAULT '',
    ip             inet,
    device_label   text        NOT NULL DEFAULT '',

    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    revoked_reason text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_used_at   timestamptz
);

CREATE UNIQUE INDEX refresh_sessions_token_hash_unique ON refresh_sessions (token_hash);
CREATE INDEX refresh_sessions_user_active_idx
    ON refresh_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;
CREATE INDEX refresh_sessions_family_idx ON refresh_sessions (family_id);

-- ---------------------------------------------------------------------------
--  Журнал событий безопасности.
--
--  Отдельно от общего аудита: сюда пишутся попытки входа, срабатывания
--  rate-limit, баны IP и подозрительные запросы. Нужен для разбора
--  инцидентов и для адаптивной блокировки.
-- ---------------------------------------------------------------------------
CREATE TABLE security_events (
    id         bigserial   PRIMARY KEY,
    kind       text        NOT NULL,
    severity   smallint    NOT NULL DEFAULT 1,
    user_id    uuid        REFERENCES users(id) ON DELETE SET NULL,
    ip         inet,
    user_agent text,
    route      text,
    details    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX security_events_created_idx ON security_events (created_at DESC);
CREATE INDEX security_events_ip_created_idx ON security_events (ip, created_at DESC);
CREATE INDEX security_events_kind_created_idx ON security_events (kind, created_at DESC);

-- ---------------------------------------------------------------------------
--  Блокировки адресов.
--
--  Основной механизм бана живёт в Redis (быстрая проверка на каждый запрос),
--  но постоянные блокировки дублируются в базу, чтобы переживать перезапуск
--  и попадать в отчёты администратора.
-- ---------------------------------------------------------------------------
CREATE TABLE ip_blocks (
    ip          inet        PRIMARY KEY,
    reason      text        NOT NULL,
    hits        integer     NOT NULL DEFAULT 1,
    blocked_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz,
    created_by  uuid        REFERENCES users(id) ON DELETE SET NULL,
    permanent   boolean     NOT NULL DEFAULT false
);

CREATE INDEX ip_blocks_expires_idx ON ip_blocks (expires_at) WHERE permanent = false;
