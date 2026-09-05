-- ===========================================================================
--  Миграция 10. SMS-подтверждение телефона отключено: достаточно почты.
-- ===========================================================================

-- Активируем тех, кто уже подтвердил почту, но ждал SMS.
UPDATE users
SET status = 'active'
WHERE deleted_at IS NULL
  AND status = 'pending'
  AND email_verified_at IS NOT NULL;

COMMENT ON COLUMN users.phone_verified_at IS
    'Устарело для гейтов: SMS-подтверждение отключено, достаточно email_verified_at';
