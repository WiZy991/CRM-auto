package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Review — отзыв клиента по завершённой сделке.
type Review struct {
	ID        uuid.UUID
	DealID    uuid.UUID
	DealerID  uuid.UUID
	AuthorID  uuid.UUID
	Rating    int
	Text      string
	AuthorName string
	DealerReply string
	RepliedAt *time.Time
	CreatedAt time.Time
}

type Reviews struct {
	pool *Pool
}

func NewReviews(pool *Pool) *Reviews { return &Reviews{pool: pool} }

func (r *Reviews) ByDeal(ctx context.Context, dealID uuid.UUID) (*Review, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT r.id, r.deal_id, r.dealer_id, r.author_id, r.rating, r.text,
		       COALESCE(split_part(u.full_name, ' ', 1), 'Клиент'),
		       COALESCE(r.dealer_reply, ''), r.replied_at, r.created_at
		FROM reviews r
		JOIN users u ON u.id = r.author_id
		WHERE r.deal_id = $1`, dealID)
	rev, err := scanReview(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rev, nil
}

func (r *Reviews) ListByDealer(ctx context.Context, dealerID uuid.UUID, publishedOnly bool, limit int) ([]Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	condition := ""
	if publishedOnly {
		condition = " AND r.is_published"
	}
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.deal_id, r.dealer_id, r.author_id, r.rating, r.text,
		       COALESCE(split_part(u.full_name, ' ', 1), 'Клиент'),
		       COALESCE(r.dealer_reply, ''), r.replied_at, r.created_at
		FROM reviews r
		JOIN users u ON u.id = r.author_id
		WHERE r.dealer_id = $1%s
		ORDER BY r.created_at DESC
		LIMIT $2`, condition), dealerID, limit)
	if err != nil {
		return nil, fmt.Errorf("список отзывов: %w", err)
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		rev, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rev)
	}
	return out, rows.Err()
}

func (r *Reviews) Reply(ctx context.Context, reviewID, dealerID uuid.UUID, reply string) (*Review, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE reviews SET dealer_reply = $3, replied_at = now()
		WHERE id = $1 AND dealer_id = $2
		RETURNING id, deal_id, dealer_id, author_id, rating, text,
		          '', COALESCE(dealer_reply, ''), replied_at, created_at`,
		reviewID, dealerID, reply)
	return scanReview(row)
}

type CreateReviewParams struct {
	DealID   uuid.UUID
	DealerID uuid.UUID
	AuthorID uuid.UUID
	Rating   int
	Text     string
}

func (r *Reviews) Create(ctx context.Context, params CreateReviewParams) (*Review, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO reviews (deal_id, dealer_id, author_id, rating, text)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, deal_id, dealer_id, author_id, rating, text, '', '', NULL::timestamptz, created_at`,
		params.DealID, params.DealerID, params.AuthorID, params.Rating, params.Text)
	rev, err := scanReview(row)
	if err != nil {
		return nil, fmt.Errorf("создание отзыва: %w", err)
	}
	_, _ = r.pool.Exec(ctx, `
		UPDATE dealer_profiles SET
		  rating_avg = (rating_avg * rating_count + $2) / GREATEST(rating_count + 1, 1),
		  rating_count = rating_count + 1
		WHERE user_id = $1`, params.DealerID, params.Rating)
	return rev, nil
}

func scanReview(row interface{ Scan(dest ...any) error }) (*Review, error) {
	var rev Review
	err := row.Scan(&rev.ID, &rev.DealID, &rev.DealerID, &rev.AuthorID, &rev.Rating, &rev.Text,
		&rev.AuthorName, &rev.DealerReply, &rev.RepliedAt, &rev.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение отзыва: %w", err)
	}
	return &rev, nil
}
