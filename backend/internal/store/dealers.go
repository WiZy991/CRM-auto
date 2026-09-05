package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Dealers — публичные профили российских импортёров.
type Dealers struct {
	pool *Pool
}

func NewDealers(pool *Pool) *Dealers { return &Dealers{pool: pool} }

// PublicDealer — карточка дилера, которую можно показать гостю.
//
// ИНН, юридический адрес и внутренние поля сюда не входят: это не витрина
// для конкурентов, а список компаний, с которыми можно открыть сделку.
type PublicDealer struct {
	UserID        uuid.UUID  `json:"user_id"`
	Slug          string     `json:"slug"`
	CompanyName   string     `json:"company_name"`
	City          string     `json:"city"`
	Description   string     `json:"description"`
	LogoURL       string     `json:"logo_url,omitempty"`
	Services      []string   `json:"services"`
	WorkCountries []string   `json:"work_countries"`
	RatingAvg     float64    `json:"rating_avg"`
	RatingCount   int        `json:"rating_count"`
	DealsWon      int        `json:"deals_won"`
	DealsTotal    int        `json:"deals_total"`
	CarsActive    int        `json:"cars_active"`
	Verified      bool       `json:"verified"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
}

// DealerProfile — полный профиль, доступный владельцу.
type DealerProfile struct {
	PublicDealer
	LegalName string `json:"legal_name,omitempty"`
	INN       string `json:"inn,omitempty"`
	Address   string `json:"address,omitempty"`
	Website   string `json:"website,omitempty"`
	CoverURL  string `json:"cover_url,omitempty"`
}

// DealerFilter — выборка публичного списка.
type DealerFilter struct {
	City   string
	Limit  int
	Offset int
}

const publicDealerSelect = `
	dp.user_id, dp.slug, dp.company_name, dp.city, dp.description,
	COALESCE(dp.logo_url, ''),
	dp.services, dp.work_countries::text[],
	dp.rating_avg, dp.rating_count, dp.deals_won, dp.deals_total,
	(SELECT count(*) FROM cars c
	  WHERE c.dealer_id = dp.user_id AND c.status IN ('active', 'reserved'))::int,
	(dp.verified_at IS NOT NULL), dp.verified_at`

func scanPublicDealer(row pgx.Row) (*PublicDealer, error) {
	var item PublicDealer
	var servicesRaw []byte
	err := row.Scan(
		&item.UserID, &item.Slug, &item.CompanyName, &item.City, &item.Description,
		&item.LogoURL, &servicesRaw, &item.WorkCountries,
		&item.RatingAvg, &item.RatingCount, &item.DealsWon, &item.DealsTotal,
		&item.CarsActive, &item.Verified, &item.VerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение профиля дилера: %w", err)
	}
	item.Services = decodeStringArray(servicesRaw)
	if item.Services == nil {
		item.Services = []string{}
	}
	if item.WorkCountries == nil {
		item.WorkCountries = []string{}
	}
	return &item, nil
}

// List возвращает страницу публичных профилей.
func (d *Dealers) List(ctx context.Context, filter DealerFilter) ([]PublicDealer, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	city := strings.TrimSpace(filter.City)
	args := []any{}
	where := []string{
		"u.deleted_at IS NULL",
		"u.status = 'active'",
		"u.role = 'dealer'",
	}
	if city != "" {
		args = append(args, "%"+city+"%")
		where = append(where, fmt.Sprintf("dp.city ILIKE $%d", len(args)))
	}

	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM dealer_profiles dp
		JOIN users u ON u.id = dp.user_id
		WHERE %s`, strings.Join(where, " AND "))

	var total int
	if err := d.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт дилеров: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT %s
		FROM dealer_profiles dp
		JOIN users u ON u.id = dp.user_id
		WHERE %s
		ORDER BY (dp.verified_at IS NOT NULL) DESC, dp.rating_avg DESC, dp.deals_won DESC, dp.company_name
		LIMIT $%d OFFSET $%d`,
		publicDealerSelect, strings.Join(where, " AND "), len(args)-1, len(args))

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("список дилеров: %w", err)
	}
	defer rows.Close()

	items := make([]PublicDealer, 0, limit)
	for rows.Next() {
		item, err := scanPublicDealer(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("чтение списка дилеров: %w", err)
	}
	return items, total, nil
}

// BySlug возвращает публичную карточку по адресу.
func (d *Dealers) BySlug(ctx context.Context, slug string) (*PublicDealer, error) {
	row := d.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s
		FROM dealer_profiles dp
		JOIN users u ON u.id = dp.user_id
		WHERE dp.slug = $1 AND u.deleted_at IS NULL AND u.status = 'active'`, publicDealerSelect),
		strings.ToLower(strings.TrimSpace(slug)))
	return scanPublicDealer(row)
}

// ByUserID возвращает полный профиль владельца.
func (d *Dealers) ByUserID(ctx context.Context, userID uuid.UUID) (*DealerProfile, error) {
	return d.scanFull(ctx, userID)
}

func (d *Dealers) scanFull(ctx context.Context, userID uuid.UUID) (*DealerProfile, error) {
	row := d.pool.QueryRow(ctx, `
		SELECT
			dp.user_id, dp.slug, dp.company_name, dp.city, dp.description,
			COALESCE(dp.logo_url, ''),
			dp.services, dp.work_countries::text[],
			dp.rating_avg, dp.rating_count, dp.deals_won, dp.deals_total,
			(SELECT count(*) FROM cars c
			  WHERE c.dealer_id = dp.user_id AND c.status IN ('active', 'reserved'))::int,
			(dp.verified_at IS NOT NULL), dp.verified_at,
			COALESCE(dp.legal_name, ''), COALESCE(dp.inn, ''),
			COALESCE(dp.address, ''), COALESCE(dp.website, ''), COALESCE(dp.cover_url, '')
		FROM dealer_profiles dp
		WHERE dp.user_id = $1`, userID)

	var profile DealerProfile
	var servicesRaw []byte
	err := row.Scan(
		&profile.UserID, &profile.Slug, &profile.CompanyName, &profile.City, &profile.Description,
		&profile.LogoURL, &servicesRaw, &profile.WorkCountries,
		&profile.RatingAvg, &profile.RatingCount, &profile.DealsWon, &profile.DealsTotal,
		&profile.CarsActive, &profile.Verified, &profile.VerifiedAt,
		&profile.LegalName, &profile.INN, &profile.Address, &profile.Website, &profile.CoverURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение профиля дилера: %w", err)
	}
	profile.Services = decodeStringArray(servicesRaw)
	if profile.Services == nil {
		profile.Services = []string{}
	}
	if profile.WorkCountries == nil {
		profile.WorkCountries = []string{}
	}
	return &profile, nil
}

// DealerWrite — поля, которые дилер может править сам.
type DealerWrite struct {
	Slug          string
	CompanyName   string
	LegalName     string
	INN           string
	City          string
	Address       string
	Description   string
	Website       string
	LogoURL       string
	CoverURL      string
	Services      []string
	WorkCountries []string
}

// Upsert создаёт или обновляет профиль.
func (d *Dealers) Upsert(ctx context.Context, userID uuid.UUID, write DealerWrite) (*DealerProfile, error) {
	countries := write.WorkCountries
	if len(countries) == 0 {
		countries = []string{"cn", "jp"}
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO dealer_profiles (
			user_id, slug, company_name, legal_name, inn, city, address,
			description, website, logo_url, cover_url, services, work_countries)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, ''),
		        $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''),
		        $12::jsonb, $13::origin_country[])
		ON CONFLICT (user_id) DO UPDATE SET
			slug = EXCLUDED.slug,
			company_name = EXCLUDED.company_name,
			legal_name = EXCLUDED.legal_name,
			inn = EXCLUDED.inn,
			city = EXCLUDED.city,
			address = EXCLUDED.address,
			description = EXCLUDED.description,
			website = EXCLUDED.website,
			logo_url = EXCLUDED.logo_url,
			cover_url = EXCLUDED.cover_url,
			services = EXCLUDED.services,
			work_countries = EXCLUDED.work_countries`,
		userID, write.Slug, write.CompanyName, write.LegalName, write.INN, write.City, write.Address,
		write.Description, write.Website, write.LogoURL, write.CoverURL,
		encodeStringArray(write.Services), countries)
	if err != nil {
		if isUniqueViolation(err, "dealer_profiles_slug_unique") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("сохранение профиля дилера: %w", err)
	}
	return d.scanFull(ctx, userID)
}

// EnsureDefault создаёт каркас профиля при регистрации дилера.
func (d *Dealers) EnsureDefault(ctx context.Context, userID uuid.UUID, fullName string) error {
	slug := defaultDealerSlug(userID)
	company := strings.TrimSpace(fullName)
	if company == "" {
		company = "Дилер"
	}
	_, err := d.pool.Exec(ctx, `
		INSERT INTO dealer_profiles (user_id, slug, company_name, city, description, services, work_countries)
		VALUES ($1, $2, $3, '', '', '[]'::jsonb, ARRAY['cn','jp']::origin_country[])
		ON CONFLICT (user_id) DO NOTHING`, userID, slug, company)
	if err != nil {
		if isUniqueViolation(err, "dealer_profiles_slug_unique") {
			_, err = d.pool.Exec(ctx, `
				INSERT INTO dealer_profiles (user_id, slug, company_name, city, description, services, work_countries)
				VALUES ($1, $2, $3, '', '', '[]'::jsonb, ARRAY['cn','jp']::origin_country[])
				ON CONFLICT (user_id) DO NOTHING`,
				userID, slug+"-"+userID.String()[24:32], company)
		}
		if err != nil {
			return fmt.Errorf("профиль дилера по умолчанию: %w", err)
		}
	}
	return nil
}

func defaultDealerSlug(id uuid.UUID) string {
	hex := strings.ReplaceAll(id.String(), "-", "")
	if len(hex) < 12 {
		return "d-" + hex
	}
	return "d-" + hex[:12]
}

// SlugOK проверяет формат адреса карточки: латиница, цифры, дефис.
func SlugOK(slug string) bool {
	if len(slug) < 2 || len(slug) > 63 {
		return false
	}
	for i, r := range slug {
		if i == 0 {
			if !unicode.IsLower(r) && !unicode.IsDigit(r) {
				return false
			}
			continue
		}
		if r != '-' && !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
