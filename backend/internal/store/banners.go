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

// Banners — рекламные баннеры дилеров.
type Banners struct {
	pool *Pool
}

func NewBanners(pool *Pool) *Banners { return &Banners{pool: pool} }

const bannerColumns = `
	banners.id, banners.dealer_id, banners.placement, banners.status,
	banners.title, banners.subtitle, banners.image_url,
	COALESCE(banners.image_mobile_url, ''), banners.href, banners.cta_label,
	banners.starts_at, banners.ends_at, banners.weight,
	banners.impressions, banners.clicks,
	COALESCE(banners.reject_reason, ''),
	banners.created_at, banners.updated_at`

func scanBanner(row pgx.Row) (*domain.Banner, error) {
	var banner domain.Banner
	err := row.Scan(
		&banner.ID, &banner.DealerID, &banner.Placement, &banner.Status,
		&banner.Title, &banner.Subtitle, &banner.ImageURL,
		&banner.ImageMobileURL, &banner.Href, &banner.CTALabel,
		&banner.StartsAt, &banner.EndsAt, &banner.Weight,
		&banner.Impressions, &banner.Clicks,
		&banner.RejectReason,
		&banner.CreatedAt, &banner.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение баннера: %w", err)
	}
	return &banner, nil
}

// BannerParams — данные баннера.
type BannerParams struct {
	DealerID  uuid.UUID
	Placement domain.BannerPlacement

	Title          string
	Subtitle       string
	ImageURL       string
	ImageMobileURL string
	Href           string
	CTALabel       string

	StartsAt time.Time
	EndsAt   time.Time
	Weight   int
}

// Create создаёт баннер в состоянии черновика.
func (b *Banners) Create(ctx context.Context, params BannerParams) (*domain.Banner, error) {
	return scanBanner(b.pool.QueryRow(ctx, `
		INSERT INTO banners (
			dealer_id, placement, title, subtitle,
			image_url, image_mobile_url, href, cta_label,
			starts_at, ends_at, weight
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11)
		RETURNING `+bannerColumns,
		params.DealerID, params.Placement, params.Title, params.Subtitle,
		params.ImageURL, params.ImageMobileURL, params.Href, params.CTALabel,
		params.StartsAt, params.EndsAt, params.Weight))
}

// Update изменяет баннер дилера.
//
// Правка возвращает баннер на модерацию: иначе после одобрения можно
// заменить картинку и ссылку на что угодно, и модерация теряет смысл.
func (b *Banners) Update(ctx context.Context, bannerID, dealerID uuid.UUID, isAdmin bool, params BannerParams) (*domain.Banner, error) {
	accessCondition := "banners.dealer_id = $2"
	if isAdmin {
		accessCondition = "($2 IS NOT NULL)"
	}

	return scanBanner(b.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE banners SET
			placement = $3,
			title = $4,
			subtitle = $5,
			image_url = $6,
			image_mobile_url = NULLIF($7, ''),
			href = $8,
			cta_label = $9,
			starts_at = $10,
			ends_at = $11,
			weight = $12,
			status = CASE WHEN status IN ('active', 'rejected')
			              THEN 'moderation'::banner_status
			              ELSE status END,
			reject_reason = NULL
		WHERE banners.id = $1 AND %s
		RETURNING %s`, accessCondition, bannerColumns),
		bannerID, dealerID,
		params.Placement, params.Title, params.Subtitle,
		params.ImageURL, params.ImageMobileURL, params.Href, params.CTALabel,
		params.StartsAt, params.EndsAt, params.Weight))
}

// SubmitForModeration отправляет черновик на проверку.
func (b *Banners) SubmitForModeration(ctx context.Context, bannerID, dealerID uuid.UUID) (*domain.Banner, error) {
	return scanBanner(b.pool.QueryRow(ctx, `
		UPDATE banners SET status = 'moderation', reject_reason = NULL
		WHERE id = $1 AND dealer_id = $2 AND status IN ('draft', 'rejected', 'paused')
		RETURNING `+bannerColumns, bannerID, dealerID))
}

// Moderate одобряет или отклоняет баннер.
func (b *Banners) Moderate(ctx context.Context, bannerID, adminID uuid.UUID, approve bool, reason string) (*domain.Banner, error) {
	status := domain.BannerRejected
	if approve {
		status = domain.BannerActive
	}

	return scanBanner(b.pool.QueryRow(ctx, `
		UPDATE banners SET status = $3, moderated_by = $2, reject_reason = NULLIF($4, '')
		WHERE id = $1 AND status = 'moderation'
		RETURNING `+bannerColumns,
		bannerID, adminID, status, truncate(reason, 500)))
}

// SetPaused приостанавливает или возобновляет показ.
func (b *Banners) SetPaused(ctx context.Context, bannerID, dealerID uuid.UUID, paused bool) (*domain.Banner, error) {
	if paused {
		return scanBanner(b.pool.QueryRow(ctx, `
			UPDATE banners SET status = 'paused'
			WHERE id = $1 AND dealer_id = $2 AND status = 'active'
			RETURNING `+bannerColumns, bannerID, dealerID))
	}

	// Возобновление не проходит модерацию заново: содержимое не менялось,
	// а повторная проверка того же баннера только задерживает дилера.
	return scanBanner(b.pool.QueryRow(ctx, `
		UPDATE banners SET status = 'active'
		WHERE id = $1 AND dealer_id = $2 AND status = 'paused'
		RETURNING `+bannerColumns, bannerID, dealerID))
}

// Delete удаляет баннер дилера.
func (b *Banners) Delete(ctx context.Context, bannerID, dealerID uuid.UUID, isAdmin bool) error {
	accessCondition := "dealer_id = $2"
	if isAdmin {
		accessCondition = "($2 IS NOT NULL)"
	}

	tag, err := b.pool.Exec(ctx,
		fmt.Sprintf(`DELETE FROM banners WHERE id = $1 AND %s`, accessCondition), bannerID, dealerID)
	if err != nil {
		return fmt.Errorf("удаление баннера: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ByID возвращает баннер.
func (b *Banners) ByID(ctx context.Context, bannerID uuid.UUID) (*domain.Banner, error) {
	return scanBanner(b.pool.QueryRow(ctx,
		`SELECT `+bannerColumns+` FROM banners WHERE banners.id = $1`, bannerID))
}

// ListForDealer возвращает баннеры дилера.
func (b *Banners) ListForDealer(ctx context.Context, dealerID uuid.UUID) ([]domain.Banner, error) {
	return b.query(ctx,
		`SELECT `+bannerColumns+` FROM banners
		 WHERE banners.dealer_id = $1
		 ORDER BY banners.created_at DESC`, dealerID)
}

// ListForModeration возвращает баннеры, ожидающие проверки.
func (b *Banners) ListForModeration(ctx context.Context, limit int) ([]domain.Banner, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return b.query(ctx,
		`SELECT `+bannerColumns+` FROM banners
		 WHERE banners.status = 'moderation'
		 ORDER BY banners.created_at
		 LIMIT $1`, limit)
}

// Active возвращает баннеры для показа в заданном месте.
//
// Отбор идёт по индексу с условием status = 'active', а срок действия
// проверяется здесь же: просроченный баннер не должен показываться до того,
// как фоновая задача переведёт его в состояние «истёк».
func (b *Banners) Active(ctx context.Context, placement domain.BannerPlacement, limit int) ([]domain.Banner, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	return b.query(ctx, `
		SELECT `+bannerColumns+` FROM banners
		WHERE banners.status = 'active'
		  AND banners.placement = $1
		  AND banners.starts_at <= now()
		  AND banners.ends_at > now()
		ORDER BY banners.weight DESC, banners.id
		LIMIT $2`, placement, limit)
}

func (b *Banners) query(ctx context.Context, sql string, args ...any) ([]domain.Banner, error) {
	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("выборка баннеров: %w", err)
	}
	defer rows.Close()

	var out []domain.Banner
	for rows.Next() {
		banner, err := scanBanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *banner)
	}
	return out, rows.Err()
}

// ExpireOutdated переводит баннеры с истёкшим сроком в состояние «истёк».
func (b *Banners) ExpireOutdated(ctx context.Context) (int64, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE banners SET status = 'expired'
		WHERE status IN ('active', 'paused') AND ends_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("завершение просроченных баннеров: %w", err)
	}
	return tag.RowsAffected(), nil
}

// FlushCounters переносит накопленные показы и клики в базу.
//
// Счётчики копятся в Redis и сбрасываются пачкой: UPDATE на каждый показ
// баннера означал бы конкуренцию за одну строку при любом заметном трафике,
// а сами показы — не те данные, ради которых стоит держать блокировку.
func (b *Banners) FlushCounters(ctx context.Context, impressions, clicks map[uuid.UUID]int64) error {
	if len(impressions) == 0 && len(clicks) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(impressions)+len(clicks))
	seen := make(map[uuid.UUID]bool, len(impressions)+len(clicks))
	for id := range impressions {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range clicks {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	batch := &pgx.Batch{}
	for _, id := range ids {
		batch.Queue(`
			UPDATE banners
			SET impressions = impressions + $2, clicks = clicks + $3
			WHERE id = $1`, id, impressions[id], clicks[id])
	}

	results := b.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for range ids {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("сброс счётчиков баннеров: %w", err)
		}
	}
	return nil
}

// BannerStats — сводка по баннерам дилера.
type BannerStats struct {
	Total       int     `json:"total"`
	Active      int     `json:"active"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	CTR         float64 `json:"ctr"`
}

// StatsForDealer считает сводку по баннерам дилера.
func (b *Banners) StatsForDealer(ctx context.Context, dealerID uuid.UUID) (*BannerStats, error) {
	var stats BannerStats

	err := b.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'active'),
		       COALESCE(sum(impressions), 0),
		       COALESCE(sum(clicks), 0)
		FROM banners WHERE dealer_id = $1`, dealerID,
	).Scan(&stats.Total, &stats.Active, &stats.Impressions, &stats.Clicks)
	if err != nil {
		return nil, fmt.Errorf("сводка по баннерам: %w", err)
	}

	if stats.Impressions > 0 {
		stats.CTR = float64(stats.Clicks) / float64(stats.Impressions)
	}
	return &stats, nil
}
