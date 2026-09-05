-- ===========================================================================
--  Миграция 1. Расширения, перечисления, общие функции.
--
--  Все справочные значения вынесены в enum, а не в отдельные таблицы:
--  набор фиксирован бизнес-логикой (этапы воронки, страны импорта), а enum
--  занимает 4 байта, индексируется дешевле и проверяется на уровне базы.
-- ===========================================================================

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- --- Пользователи и доступ ---------------------------------------------------
CREATE TYPE user_role   AS ENUM ('client', 'dealer', 'seller', 'admin');
CREATE TYPE user_status AS ENUM ('pending', 'active', 'suspended', 'deleted');
CREATE TYPE verify_channel AS ENUM ('email', 'phone');

-- --- Автомобили -------------------------------------------------------------
CREATE TYPE origin_country AS ENUM ('cn', 'jp');
CREATE TYPE car_status   AS ENUM ('draft', 'moderation', 'active', 'reserved', 'sold', 'archived');
CREATE TYPE transmission AS ENUM ('at', 'mt', 'cvt', 'dct', 'amt');
CREATE TYPE drivetrain   AS ENUM ('fwd', 'rwd', 'awd');
CREATE TYPE body_type    AS ENUM ('sedan', 'suv', 'crossover', 'hatchback', 'wagon',
                                  'coupe', 'minivan', 'pickup', 'van', 'liftback');
CREATE TYPE fuel_type    AS ENUM ('petrol', 'diesel', 'hybrid', 'phev', 'electric');
CREATE TYPE currency_code AS ENUM ('rub', 'usd', 'cny', 'jpy');

-- --- Продавцы за рубежом ----------------------------------------------------
CREATE TYPE seller_kind AS ENUM ('auction', 'exporter', 'dealership', 'factory', 'broker');

-- --- Заявки, сделки, воронка ------------------------------------------------
CREATE TYPE request_status AS ENUM ('new', 'in_progress', 'answered', 'converted', 'rejected', 'closed');

-- Порядок значений соответствует порядку этапов воронки из ТЗ, поэтому
-- сравнение и сортировка по стадии работают без дополнительных таблиц.
CREATE TYPE deal_stage AS ENUM ('lead', 'needs', 'contract', 'payment', 'shipping', 'customs', 'handover');
CREATE TYPE deal_outcome AS ENUM ('open', 'won', 'lost');
CREATE TYPE document_kind AS ENUM ('contract', 'invoice', 'payment_order', 'customs_declaration',
                                   'passport', 'vehicle_certificate', 'acceptance_act', 'other');

-- --- Реклама ----------------------------------------------------------------
CREATE TYPE banner_placement AS ENUM ('home_hero', 'home_inline', 'catalog_top', 'catalog_sidebar', 'car_page');
CREATE TYPE banner_status AS ENUM ('draft', 'moderation', 'active', 'paused', 'rejected', 'expired');

-- --- Уведомления ------------------------------------------------------------
CREATE TYPE notification_kind AS ENUM (
    'request_created', 'request_answered', 'deal_created', 'deal_stage_changed',
    'deal_message', 'deal_task_due', 'review_received', 'moderation_result', 'security_alert'
);

-- ---------------------------------------------------------------------------
--  Единая функция обновления updated_at.
--  Триггер вместо ответственности приложения: колонка не разъедется, даже
--  если строку изменят миграцией или вручную.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
--  Нормализация телефона к цифровому виду для сравнения и уникальности.
--  IMMUTABLE — обязательное условие для использования в индексе.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION normalize_phone(raw text)
RETURNS text
LANGUAGE sql
IMMUTABLE
STRICT
AS $$
    SELECT regexp_replace(raw, '[^0-9]', '', 'g');
$$;
