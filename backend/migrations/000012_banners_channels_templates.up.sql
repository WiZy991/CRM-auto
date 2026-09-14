-- Слот главной: editorial-полоса вместо конкуренции с hero.
ALTER TYPE banner_placement ADD VALUE IF NOT EXISTS 'home_strip';

-- Авито и Дром.
ALTER TYPE social_network ADD VALUE IF NOT EXISTS 'avito';
ALTER TYPE social_network ADD VALUE IF NOT EXISTS 'drom';

-- Шаблоны документов дилера (DOCX).
CREATE TABLE document_templates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    dealer_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        text NOT NULL,
    kind         document_kind NOT NULL DEFAULT 'other',
    stage        deal_stage,
    storage_key  text NOT NULL,
    bytes        integer NOT NULL CHECK (bytes > 0),
    field_map    jsonb NOT NULL DEFAULT '{}'::jsonb,
    placeholders text[] NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'archived')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX document_templates_dealer_idx
    ON document_templates (dealer_id, status);

CREATE TRIGGER document_templates_set_updated_at
    BEFORE UPDATE ON document_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
