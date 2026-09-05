package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Sessions — хранилище refresh-сессий.
//
// Реализована схема с ротацией и семьями токенов:
//
//   - при входе создаётся сессия с новым family_id;
//   - при обновлении старая сессия помечается использованной, а новая
//     ссылается на неё через previous_id и наследует family_id;
//   - если пришёл refresh-токен, который уже был использован или отозван,
//     это означает, что токеном владеет кто-то ещё. Тогда гасится вся семья
//     целиком, а не только предъявленный токен.
//
// Без гашения семьи украденный токен даёт атакующему бессрочный доступ:
// он обновляет его раньше жертвы, а жертва просто получает ошибку и
// логинится заново.
type Sessions struct {
	pool *Pool
}

func NewSessions(pool *Pool) *Sessions { return &Sessions{pool: pool} }

// Session — запись refresh-сессии.
type Session struct {
	ID            uuid.UUID
	FamilyID      uuid.UUID
	UserID        uuid.UUID
	UserAgent     string
	IP            *string
	DeviceLabel   string
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	RevokedReason *string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
}

// Active сообщает, что сессия ещё пригодна к использованию.
func (s *Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

// CreateSessionParams — параметры создания сессии.
type CreateSessionParams struct {
	UserID      uuid.UUID
	TokenHash   []byte
	UserAgent   string
	IP          string
	DeviceLabel string
	ExpiresAt   time.Time

	// FamilyID пуст при первом входе и наследуется при обновлении.
	FamilyID uuid.UUID
	// PreviousID указывает на сессию, из которой выполнено обновление.
	PreviousID *uuid.UUID
}

// ErrSessionReused — предъявлен отозванный или уже использованный токен.
var ErrSessionReused = errors.New("refresh-токен уже был использован")

// Create создаёт сессию.
func (s *Sessions) Create(ctx context.Context, params CreateSessionParams) (*Session, error) {
	familyID := params.FamilyID
	if familyID == uuid.Nil {
		familyID = uuid.New()
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO refresh_sessions
		    (family_id, user_id, token_hash, previous_id, user_agent, ip, device_label, expires_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::inet, $7, $8)
		RETURNING id, family_id, user_id, user_agent, ip::text, device_label,
		          expires_at, revoked_at, revoked_reason, created_at, last_used_at`,
		familyID, params.UserID, params.TokenHash, params.PreviousID,
		truncate(params.UserAgent, 400), params.IP, truncate(params.DeviceLabel, 100), params.ExpiresAt,
	)

	return scanSession(row)
}

func scanSession(row pgx.Row) (*Session, error) {
	var sess Session
	err := row.Scan(
		&sess.ID, &sess.FamilyID, &sess.UserID, &sess.UserAgent, &sess.IP, &sess.DeviceLabel,
		&sess.ExpiresAt, &sess.RevokedAt, &sess.RevokedReason, &sess.CreatedAt, &sess.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение сессии: %w", err)
	}
	return &sess, nil
}

// RotateResult — результат обновления сессии.
type RotateResult struct {
	Session  *Session
	FamilyID uuid.UUID
	OldID    uuid.UUID
}

// Rotate обменивает предъявленный refresh-токен на новый.
//
// Вся операция выполняется в одной транзакции с блокировкой строки
// (FOR UPDATE): два одновременных обновления одним токеном не должны
// оба завершиться успехом.
func (s *Sessions) Rotate(
	ctx context.Context,
	presentedHash []byte,
	newHash []byte,
	newExpiresAt time.Time,
	userAgent, ip, deviceLabel string,
) (*RotateResult, error) {
	var result *RotateResult

	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		var (
			sessionID uuid.UUID
			familyID  uuid.UUID
			userID    uuid.UUID
			expiresAt time.Time
			revokedAt *time.Time
		)

		err := tx.QueryRow(ctx, `
			SELECT id, family_id, user_id, expires_at, revoked_at
			FROM refresh_sessions
			WHERE token_hash = $1
			FOR UPDATE`, presentedHash,
		).Scan(&sessionID, &familyID, &userID, &expiresAt, &revokedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("поиск сессии по токену: %w", err)
		}

		// Предъявлен отозванный токен: считаем это утечкой и гасим всю
		// семью, включая активную сессию законного владельца.
		if revokedAt != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE refresh_sessions
				SET revoked_at = now(), revoked_reason = 'token_reuse_detected'
				WHERE family_id = $1 AND revoked_at IS NULL`, familyID); err != nil {
				return fmt.Errorf("гашение семьи токенов: %w", err)
			}
			return ErrSessionReused
		}

		if !expiresAt.After(time.Now()) {
			return ErrNotFound
		}

		if _, err := tx.Exec(ctx, `
			UPDATE refresh_sessions
			SET revoked_at = now(), revoked_reason = 'rotated', last_used_at = now()
			WHERE id = $1`, sessionID); err != nil {
			return fmt.Errorf("отзыв предыдущей сессии: %w", err)
		}

		row := tx.QueryRow(ctx, `
			INSERT INTO refresh_sessions
			    (family_id, user_id, token_hash, previous_id, user_agent, ip, device_label, expires_at)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::inet, $7, $8)
			RETURNING id, family_id, user_id, user_agent, ip::text, device_label,
			          expires_at, revoked_at, revoked_reason, created_at, last_used_at`,
			familyID, userID, newHash, sessionID,
			truncate(userAgent, 400), ip, truncate(deviceLabel, 100), newExpiresAt,
		)

		created, err := scanSession(row)
		if err != nil {
			return err
		}

		result = &RotateResult{Session: created, FamilyID: familyID, OldID: sessionID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Revoke отзывает одну сессию.
func (s *Sessions) Revoke(ctx context.Context, sessionID uuid.UUID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now(), revoked_reason = $2
		WHERE id = $1 AND revoked_at IS NULL`, sessionID, reason)
	if err != nil {
		return fmt.Errorf("отзыв сессии: %w", err)
	}
	return nil
}

// RevokeByTokenHash отзывает сессию по предъявленному токену (выход).
func (s *Sessions) RevokeByTokenHash(ctx context.Context, tokenHash []byte, reason string) (uuid.UUID, error) {
	var sessionID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now(), revoked_reason = $2
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING id`, tokenHash, reason).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("отзыв сессии по токену: %w", err)
	}
	return sessionID, nil
}

// RevokeAllForUser отзывает все сессии пользователя и возвращает их
// идентификаторы, чтобы сервис положил их в список отозванных в Redis.
func (s *Sessions) RevokeAllForUser(ctx context.Context, userID uuid.UUID, reason string) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now(), revoked_reason = $2
		WHERE user_id = $1 AND revoked_at IS NULL
		RETURNING id`, userID, reason)
	if err != nil {
		return nil, fmt.Errorf("отзыв всех сессий пользователя: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("чтение идентификатора сессии: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RevokeFamily гасит семью токенов.
func (s *Sessions) RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now(), revoked_reason = $2
		WHERE family_id = $1 AND revoked_at IS NULL
		RETURNING id`, familyID, reason)
	if err != nil {
		return nil, fmt.Errorf("гашение семьи токенов: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("чтение идентификатора сессии: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListActiveForUser возвращает активные устройства пользователя.
func (s *Sessions) ListActiveForUser(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, family_id, user_id, user_agent, ip::text, device_label,
		       expires_at, revoked_at, revoked_reason, created_at, last_used_at
		FROM refresh_sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 50`, userID)
	if err != nil {
		return nil, fmt.Errorf("список активных сессий: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sess)
	}
	return out, rows.Err()
}

// DeleteExpired удаляет старые записи. Вызывается фоновой задачей: таблица
// сессий иначе растёт неограниченно.
func (s *Sessions) DeleteExpired(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM refresh_sessions
		WHERE expires_at < now() - $1::interval
		   OR (revoked_at IS NOT NULL AND revoked_at < now() - $1::interval)`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("очистка устаревших сессий: %w", err)
	}
	return tag.RowsAffected(), nil
}
