-- Откат полей этапов сделки.

ALTER TABLE deals
    DROP CONSTRAINT IF EXISTS deals_customs_duties_non_negative;

ALTER TABLE deals
    DROP COLUMN IF EXISTS services_note,
    DROP COLUMN IF EXISTS destination_port,
    DROP COLUMN IF EXISTS shipping_tracking,
    DROP COLUMN IF EXISTS customs_duties_rub_minor,
    DROP COLUMN IF EXISTS sbkts_number,
    DROP COLUMN IF EXISTS sbkts_issued_at,
    DROP COLUMN IF EXISTS first_contacted_at;
