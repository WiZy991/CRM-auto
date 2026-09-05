package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

// Verification — хранилище кодов подтверждения email и телефона.
type Verification struct {
	pool *Pool
}

func NewVerification(pool *Pool) *Verification { return &Verification{pool: pool} }

// Ошибки проверки кода. Разделены, потому что интерфейс реагирует на них
// по-разному: истёкший код предлагает запросить новый, неверный —
// показывает счётчик оставшихся попыток.
var (
	ErrCodeNotFound = errors.New("код подтверждения не найден")
	ErrCodeExpired  = errors.New("срок действия кода истёк")
	ErrCodeMismatch = errors.New("код указан неверно")
	ErrCodeAttempts = errors.New("превышено число попыток ввода кода")
)

// CreateCodeParams — параметры выпуска кода.
type CreateCodeParams struct {
	UserID      uuid.UUID
	Channel     domain.VerifyChannel
	Destination string
	CodeHash    string
	ExpiresAt   time.Time
	MaxAttempts int
	CreatedIP   string
}

// Issue выпускает новый код, гася предыдущий активный.
//
// Частичный уникальный индекс не позволяет иметь два активных кода на канал,
// поэтому старый помечается использованным в той же транзакции. Иначе
// атакующий мог бы запросить сотню кодов и перебирать их одновременно.
func (v *Verification) Issue(ctx context.Context, params CreateCodeParams) error {
	if params.MaxAttempts <= 0 {
		params.MaxAttempts = 5
	}

	return v.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE verification_codes
			SET consumed_at = now()
			WHERE user_id = $1 AND channel = $2 AND consumed_at IS NULL`,
			params.UserID, params.Channel); err != nil {
			return fmt.Errorf("гашение предыдущего кода: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO verification_codes
			    (user_id, channel, destination, code_hash, expires_at, max_attempts, created_ip)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::inet)`,
			params.UserID, params.Channel, params.Destination, params.CodeHash,
			params.ExpiresAt, params.MaxAttempts, params.CreatedIP); err != nil {
			return fmt.Errorf("создание кода подтверждения: %w", err)
		}
		return nil
	})
}

// Consume проверяет код и, при совпадении, гасит его.
//
// compare получает хеш из базы и решает, подходит ли предъявленный код.
// Функция передаётся снаружи, чтобы хранилище не занималось криптографией,
// а сравнение оставалось за постоянное время.
func (v *Verification) Consume(
	ctx context.Context,
	userID uuid.UUID,
	channel domain.VerifyChannel,
	compare func(storedHash string) bool,
) (attemptsLeft int, err error) {
	err = v.pool.InTx(ctx, func(tx pgx.Tx) error {
		var (
			id          uuid.UUID
			storedHash  string
			attempts    int
			maxAttempts int
			expiresAt   time.Time
		)

		queryErr := tx.QueryRow(ctx, `
			SELECT id, code_hash, attempts, max_attempts, expires_at
			FROM verification_codes
			WHERE user_id = $1 AND channel = $2 AND consumed_at IS NULL
			FOR UPDATE`, userID, channel,
		).Scan(&id, &storedHash, &attempts, &maxAttempts, &expiresAt)
		if queryErr != nil {
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return ErrCodeNotFound
			}
			return fmt.Errorf("чтение кода подтверждения: %w", queryErr)
		}

		if !expiresAt.After(time.Now()) {
			if _, execErr := tx.Exec(ctx,
				`UPDATE verification_codes SET consumed_at = now() WHERE id = $1`, id); execErr != nil {
				return fmt.Errorf("гашение истёкшего кода: %w", execErr)
			}
			return ErrCodeExpired
		}

		if attempts >= maxAttempts {
			if _, execErr := tx.Exec(ctx,
				`UPDATE verification_codes SET consumed_at = now() WHERE id = $1`, id); execErr != nil {
				return fmt.Errorf("гашение кода после исчерпания попыток: %w", execErr)
			}
			return ErrCodeAttempts
		}

		if !compare(storedHash) {
			if execErr := tx.QueryRow(ctx, `
				UPDATE verification_codes
				SET attempts = attempts + 1
				WHERE id = $1
				RETURNING max_attempts - attempts`, id).Scan(&attemptsLeft); execErr != nil {
				return fmt.Errorf("учёт неудачной попытки: %w", execErr)
			}
			return ErrCodeMismatch
		}

		if _, execErr := tx.Exec(ctx,
			`UPDATE verification_codes SET consumed_at = now() WHERE id = $1`, id); execErr != nil {
			return fmt.Errorf("гашение использованного кода: %w", execErr)
		}
		return nil
	})

	return attemptsLeft, err
}

// ActiveCodeIssuedAt возвращает время выпуска активного кода.
// Используется, чтобы не отправлять повторный код слишком часто.
func (v *Verification) ActiveCodeIssuedAt(ctx context.Context, userID uuid.UUID, channel domain.VerifyChannel) (time.Time, bool, error) {
	var createdAt time.Time
	err := v.pool.QueryRow(ctx, `
		SELECT created_at FROM verification_codes
		WHERE user_id = $1 AND channel = $2 AND consumed_at IS NULL AND expires_at > now()`,
		userID, channel).Scan(&createdAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, fmt.Errorf("чтение времени выпуска кода: %w", err)
	default:
		return createdAt, true, nil
	}
}

// DeleteExpired очищает истёкшие и использованные коды.
func (v *Verification) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := v.pool.Exec(ctx, `
		DELETE FROM verification_codes
		WHERE expires_at < now() - interval '1 day'
		   OR (consumed_at IS NOT NULL AND consumed_at < now() - interval '1 day')`)
	if err != nil {
		return 0, fmt.Errorf("очистка кодов подтверждения: %w", err)
	}
	return tag.RowsAffected(), nil
}
