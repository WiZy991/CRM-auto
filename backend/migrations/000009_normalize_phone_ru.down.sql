CREATE OR REPLACE FUNCTION normalize_phone(raw text)
RETURNS text
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT regexp_replace(raw, '[^0-9]', '', 'g');
$$;
