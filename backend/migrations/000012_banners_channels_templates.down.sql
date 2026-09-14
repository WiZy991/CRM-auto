DROP TRIGGER IF EXISTS document_templates_set_updated_at ON document_templates;
DROP TABLE IF EXISTS document_templates;
-- Enum values (home_strip, avito, drom) нельзя безопасно удалить в Postgres.
