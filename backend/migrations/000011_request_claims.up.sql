-- ===========================================================================
--  Миграция 11. Мульти-взятие заявок: до 5 дилеров на одну заявку.
-- ===========================================================================

CREATE TYPE request_claim_status AS ENUM (
    'active',
    'lost',
    'refused_by_client',
    'converted'
);

CREATE TABLE request_claims (
    id          uuid                  PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id  uuid                  NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
    dealer_id   uuid                  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deal_id     uuid                  REFERENCES deals(id) ON DELETE SET NULL,
    status      request_claim_status  NOT NULL DEFAULT 'active',
    created_at  timestamptz           NOT NULL DEFAULT now(),
    updated_at  timestamptz           NOT NULL DEFAULT now(),
    UNIQUE (request_id, dealer_id)
);

CREATE INDEX request_claims_request_active_idx
    ON request_claims (request_id)
    WHERE status = 'active';

CREATE INDEX request_claims_dealer_idx
    ON request_claims (dealer_id, status);

CREATE TRIGGER request_claims_set_updated_at
    BEFORE UPDATE ON request_claims
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Перенос уже закреплённых заявок в claims (активные).
INSERT INTO request_claims (request_id, dealer_id, status)
SELECT r.id, r.dealer_id, 'active'::request_claim_status
FROM requests r
WHERE r.dealer_id IS NOT NULL
  AND r.status IN ('new', 'in_progress', 'answered')
ON CONFLICT (request_id, dealer_id) DO NOTHING;

-- Сконвертированные: claim converted + deal_id если есть.
INSERT INTO request_claims (request_id, dealer_id, deal_id, status)
SELECT r.id, r.dealer_id, d.id, 'converted'::request_claim_status
FROM requests r
JOIN deals d ON d.request_id = r.id AND d.dealer_id = r.dealer_id
WHERE r.dealer_id IS NOT NULL
  AND r.status = 'converted'
ON CONFLICT (request_id, dealer_id) DO UPDATE
SET status = 'converted',
    deal_id = COALESCE(request_claims.deal_id, EXCLUDED.deal_id);
