-- ===========================================================================
--  Миграция 8. Поля этапов сделки: услуги, логистика, СБКТС, первый контакт.
-- ===========================================================================

ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS services_note text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS destination_port text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS shipping_tracking text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS customs_duties_rub_minor bigint,
    ADD COLUMN IF NOT EXISTS sbkts_number text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sbkts_issued_at timestamptz,
    ADD COLUMN IF NOT EXISTS first_contacted_at timestamptz;

ALTER TABLE deals
    DROP CONSTRAINT IF EXISTS deals_customs_duties_non_negative;

ALTER TABLE deals
    ADD CONSTRAINT deals_customs_duties_non_negative
    CHECK (customs_duties_rub_minor IS NULL OR customs_duties_rub_minor >= 0);

COMMENT ON COLUMN deals.services_note IS 'Состав услуг по договору (подбор, доставка, оформление)';
COMMENT ON COLUMN deals.destination_port IS 'Порт назначения привоза';
COMMENT ON COLUMN deals.shipping_tracking IS 'Трекинг / примечание по пути автомобиля';
COMMENT ON COLUMN deals.customs_duties_rub_minor IS 'Сумма пошлин и сборов в копейках RUB';
COMMENT ON COLUMN deals.sbkts_number IS 'Номер СБКТС';
COMMENT ON COLUMN deals.sbkts_issued_at IS 'Дата выдачи СБКТС';
COMMENT ON COLUMN deals.first_contacted_at IS 'Первый контакт дилера с клиентом (лид)';
