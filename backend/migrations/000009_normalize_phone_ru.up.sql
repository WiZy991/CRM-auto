-- ===========================================================================
--  Миграция 9. Нормализация RU-телефонов: 8… и 10 цифр → как 7…
--  Иначе вход по «8…» не находил учётку, созданную с тем же номером.
-- ===========================================================================

CREATE OR REPLACE FUNCTION normalize_phone(raw text)
RETURNS text
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT CASE
        WHEN length(digits) = 11 AND left(digits, 1) = '8' THEN '7' || substr(digits, 2)
        WHEN length(digits) = 10 THEN '7' || digits
        ELSE digits
    END
    FROM (SELECT regexp_replace(raw, '[^0-9]', '', 'g') AS digits) AS t;
$$;
