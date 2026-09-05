-- ===========================================================================
--  Миграция 7. Каналы дилера для автопостинга объявлений.
-- ===========================================================================

CREATE TYPE social_network AS ENUM (
    'telegram', 'vk', 'whatsapp', 'instagram', 'youtube', 'rutube'
);

CREATE TYPE social_account_status AS ENUM (
    'disconnected', 'connected', 'needs_reauth', 'error'
);

CREATE TYPE social_outbox_status AS ENUM (
    'pending', 'sent', 'failed', 'skipped'
);

CREATE TABLE dealer_social_accounts (
    id            uuid                   PRIMARY KEY DEFAULT gen_random_uuid(),
    dealer_id     uuid                   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    network       social_network         NOT NULL,
    -- AES-GCM от JSON с токенами. В ответах API plaintext не отдаётся.
    credentials   bytea,
    external_id   text                   NOT NULL DEFAULT '',
    status        social_account_status  NOT NULL DEFAULT 'disconnected',
    last_error    text                   NOT NULL DEFAULT '',
    auto_post     boolean                NOT NULL DEFAULT true,
    created_at    timestamptz            NOT NULL DEFAULT now(),
    updated_at    timestamptz            NOT NULL DEFAULT now(),
    UNIQUE (dealer_id, network)
);

CREATE INDEX dealer_social_accounts_dealer_idx
    ON dealer_social_accounts (dealer_id);

CREATE TRIGGER dealer_social_accounts_set_updated_at
    BEFORE UPDATE ON dealer_social_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE social_outbox (
    id                uuid                  PRIMARY KEY DEFAULT gen_random_uuid(),
    dealer_id         uuid                  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    car_id            uuid                  NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    network           social_network        NOT NULL,
    status            social_outbox_status  NOT NULL DEFAULT 'pending',
    external_post_id  text                  NOT NULL DEFAULT '',
    attempts          integer               NOT NULL DEFAULT 0,
    last_error        text                  NOT NULL DEFAULT '',
    created_at        timestamptz           NOT NULL DEFAULT now(),
    updated_at        timestamptz           NOT NULL DEFAULT now(),
    UNIQUE (car_id, network)
);

CREATE INDEX social_outbox_pending_idx
    ON social_outbox (created_at)
    WHERE status = 'pending';

CREATE TRIGGER social_outbox_set_updated_at
    BEFORE UPDATE ON social_outbox
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
