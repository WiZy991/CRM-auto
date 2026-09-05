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

// SocialAccounts — подключения каналов дилера и очередь автопостинга.
type SocialAccounts struct {
	pool *Pool
}

func NewSocialAccounts(pool *Pool) *SocialAccounts {
	return &SocialAccounts{pool: pool}
}

// SocialAccountRow — строка dealer_social_accounts.
type SocialAccountRow struct {
	ID          uuid.UUID
	DealerID    uuid.UUID
	Network     domain.SocialNetwork
	Credentials []byte
	ExternalID  string
	Status      domain.SocialAccountStatus
	LastError   string
	AutoPost    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SocialOutboxRow — строка очереди публикаций.
type SocialOutboxRow struct {
	ID             uuid.UUID
	DealerID       uuid.UUID
	CarID          uuid.UUID
	Network        domain.SocialNetwork
	Status         string
	ExternalPostID string
	Attempts       int
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

const socialAccountColumns = `
	id, dealer_id, network, credentials, external_id, status, last_error, auto_post, created_at, updated_at`

func scanSocialAccount(row pgx.Row) (SocialAccountRow, error) {
	var a SocialAccountRow
	var network, status string
	err := row.Scan(
		&a.ID, &a.DealerID, &network, &a.Credentials, &a.ExternalID,
		&status, &a.LastError, &a.AutoPost, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return SocialAccountRow{}, err
	}
	a.Network = domain.SocialNetwork(network)
	a.Status = domain.SocialAccountStatus(status)
	return a, nil
}

// ListByDealer возвращает все каналы дилера.
func (s *SocialAccounts) ListByDealer(ctx context.Context, dealerID uuid.UUID) ([]SocialAccountRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+socialAccountColumns+`
		FROM dealer_social_accounts
		WHERE dealer_id = $1
		ORDER BY network`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("список каналов дилера: %w", err)
	}
	defer rows.Close()

	var out []SocialAccountRow
	for rows.Next() {
		item, err := scanSocialAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение канала: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ByDealerNetwork возвращает канал или ErrNotFound.
func (s *SocialAccounts) ByDealerNetwork(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork) (SocialAccountRow, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+socialAccountColumns+`
		FROM dealer_social_accounts
		WHERE dealer_id = $1 AND network = $2`, dealerID, string(network))
	item, err := scanSocialAccount(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SocialAccountRow{}, ErrNotFound
		}
		return SocialAccountRow{}, fmt.Errorf("чтение канала: %w", err)
	}
	return item, nil
}

// Upsert создаёт или обновляет канал.
func (s *SocialAccounts) Upsert(ctx context.Context, row SocialAccountRow) (SocialAccountRow, error) {
	scanned, err := scanSocialAccount(s.pool.QueryRow(ctx, `
		INSERT INTO dealer_social_accounts (
			dealer_id, network, credentials, external_id, status, last_error, auto_post
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (dealer_id, network) DO UPDATE SET
			credentials = COALESCE(EXCLUDED.credentials, dealer_social_accounts.credentials),
			external_id = EXCLUDED.external_id,
			status = EXCLUDED.status,
			last_error = EXCLUDED.last_error,
			auto_post = EXCLUDED.auto_post
		RETURNING `+socialAccountColumns, row.DealerID, string(row.Network), row.Credentials,
		row.ExternalID, string(row.Status), row.LastError, row.AutoPost))
	if err != nil {
		return SocialAccountRow{}, fmt.Errorf("сохранение канала: %w", err)
	}
	return scanned, nil
}

// SetStatus обновляет статус и текст ошибки после проверки или публикации.
func (s *SocialAccounts) SetStatus(
	ctx context.Context,
	dealerID uuid.UUID,
	network domain.SocialNetwork,
	status domain.SocialAccountStatus,
	lastError string,
	externalID string,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dealer_social_accounts
		SET status = $3, last_error = $4, external_id = CASE WHEN $5 = '' THEN external_id ELSE $5 END
		WHERE dealer_id = $1 AND network = $2`,
		dealerID, string(network), string(status), lastError, externalID)
	if err != nil {
		return fmt.Errorf("обновление статуса канала: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCredentials переписывает только шифротекст, не трогая auto_post.
func (s *SocialAccounts) UpdateCredentials(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork, blob []byte) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dealer_social_accounts
		SET credentials = $3
		WHERE dealer_id = $1 AND network = $2`, dealerID, string(network), blob)
	if err != nil {
		return fmt.Errorf("обновление ключа канала: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Clear удаляет ключи и переводит канал в disconnected.
func (s *SocialAccounts) Clear(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dealer_social_accounts
		SET credentials = NULL, external_id = '', status = 'disconnected', last_error = '', auto_post = true
		WHERE dealer_id = $1 AND network = $2`, dealerID, string(network))
	if err != nil {
		return fmt.Errorf("отключение канала: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AutoPostNetworks — сети дилера с включённым автопостом и живым ключом.
func (s *SocialAccounts) AutoPostNetworks(ctx context.Context, dealerID uuid.UUID) ([]SocialAccountRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+socialAccountColumns+`
		FROM dealer_social_accounts
		WHERE dealer_id = $1
		  AND auto_post
		  AND status IN ('connected', 'error')
		  AND credentials IS NOT NULL`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("каналы автопоста: %w", err)
	}
	defer rows.Close()

	var out []SocialAccountRow
	for rows.Next() {
		item, err := scanSocialAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение канала автопоста: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

const socialOutboxColumns = `
	id, dealer_id, car_id, network, status, external_post_id, attempts, last_error, created_at, updated_at`

func scanOutbox(row pgx.Row) (SocialOutboxRow, error) {
	var o SocialOutboxRow
	var network string
	err := row.Scan(
		&o.ID, &o.DealerID, &o.CarID, &network, &o.Status,
		&o.ExternalPostID, &o.Attempts, &o.LastError, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return SocialOutboxRow{}, err
	}
	o.Network = domain.SocialNetwork(network)
	return o, nil
}

// Enqueue ставит публикацию в очередь. Повтор по той же паре лот+сеть
// возвращает запись в pending, чтобы кнопка «Опубликовать» работала снова.
func (s *SocialAccounts) Enqueue(ctx context.Context, dealerID, carID uuid.UUID, network domain.SocialNetwork) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO social_outbox (dealer_id, car_id, network, status, attempts, last_error)
		VALUES ($1, $2, $3, 'pending', 0, '')
		ON CONFLICT (car_id, network) DO UPDATE SET
			status = 'pending',
			attempts = 0,
			last_error = '',
			external_post_id = ''`, dealerID, carID, string(network))
	if err != nil {
		return fmt.Errorf("постановка публикации в очередь: %w", err)
	}
	return nil
}

const outboxMaxAttempts = 8

// ClaimPending забирает пачку pending без гонки между воркерами.
func (s *SocialAccounts) ClaimPending(ctx context.Context, limit int) ([]SocialOutboxRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		WITH picked AS (
			SELECT id FROM social_outbox
			WHERE status = 'pending' AND attempts < $2
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE social_outbox o
		SET attempts = o.attempts + 1
		FROM picked
		WHERE o.id = picked.id
		RETURNING o.id, o.dealer_id, o.car_id, o.network, o.status, o.external_post_id, o.attempts, o.last_error, o.created_at, o.updated_at`, limit, outboxMaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("захват очереди публикаций: %w", err)
	}
	defer rows.Close()

	var out []SocialOutboxRow
	for rows.Next() {
		item, err := scanOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("чтение очереди публикаций: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// FinishOutbox фиксирует результат публикации.
func (s *SocialAccounts) FinishOutbox(ctx context.Context, id uuid.UUID, status, postID, lastError string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE social_outbox
		SET status = $2, external_post_id = $3, last_error = $4
		WHERE id = $1`, id, status, postID, lastError)
	if err != nil {
		return fmt.Errorf("завершение публикации: %w", err)
	}
	return nil
}

// RetryOrFail оставляет запись pending либо помечает failed по потолку попыток.
func (s *SocialAccounts) RetryOrFail(ctx context.Context, row SocialOutboxRow, lastError string) error {
	status := "pending"
	if row.Attempts >= outboxMaxAttempts {
		status = "failed"
	}
	return s.FinishOutbox(ctx, row.ID, status, "", lastError)
}
