package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

// Sellers — справочник зарубежных поставщиков.
type Sellers struct {
	pool *Pool
}

func NewSellers(pool *Pool) *Sellers { return &Sellers{pool: pool} }

const sellerColumns = `
	sellers.id, sellers.user_id, sellers.created_by,
	sellers.country, sellers.kind,
	sellers.name, COALESCE(sellers.name_local, ''), sellers.region,
	COALESCE(sellers.city, ''), COALESCE(sellers.address, ''),
	sellers.brands, sellers.description, COALESCE(sellers.website, ''),
	sellers.contacts, COALESCE(sellers.logo_url, ''),
	sellers.min_order_qty, sellers.export_experience_years,
	sellers.rating_avg, sellers.rating_count,
	sellers.verified_at, sellers.is_active,
	sellers.created_at, sellers.updated_at`

func scanSeller(row pgx.Row) (*domain.Seller, error) {
	var (
		seller   domain.Seller
		contacts []byte
	)

	err := row.Scan(
		&seller.ID, &seller.UserID, &seller.CreatedBy,
		&seller.Country, &seller.Kind,
		&seller.Name, &seller.NameLocal, &seller.Region,
		&seller.City, &seller.Address,
		&seller.Brands, &seller.Description, &seller.Website,
		&contacts, &seller.LogoURL,
		&seller.MinOrderQty, &seller.ExportExperienceYears,
		&seller.RatingAvg, &seller.RatingCount,
		&seller.VerifiedAt, &seller.IsActive,
		&seller.CreatedAt, &seller.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение продавца: %w", err)
	}

	seller.Contacts = decodeContacts(contacts)
	return &seller, nil
}

// decodeContacts читает произвольный набор контактов.
//
// Повреждённый jsonb не должен ронять выдачу списка: карточка покажется
// без контактов, и это лучше, чем ошибка на всю страницу.
func decodeContacts(raw []byte) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	contacts := map[string]string{}
	if err := json.Unmarshal(raw, &contacts); err != nil {
		return map[string]string{}
	}
	return contacts
}

// SellerFilter — параметры выборки продавцов.
type SellerFilter struct {
	Countries []string
	Kinds     []string
	Regions   []string
	Brands    []string

	Search string
	// VerifiedOnly оставляет только проверенных администрацией.
	VerifiedOnly bool
	// OwnerID показывает карточки конкретного продавца, включая скрытые.
	OwnerID *uuid.UUID
	// IncludeInactive допустим только для админки.
	IncludeInactive bool

	Sort   string
	Limit  int
	Offset int
}

// List возвращает страницу справочника продавцов.
func (s *Sellers) List(ctx context.Context, filter SellerFilter) ([]domain.Seller, int, error) {
	builder := &argBuilder{}
	where := make([]string, 0, 8)

	if !filter.IncludeInactive {
		where = append(where, "sellers.is_active")
	}
	if filter.OwnerID != nil {
		where = append(where, "sellers.user_id = "+builder.add(*filter.OwnerID))
	}
	if len(filter.Countries) > 0 {
		where = append(where, fmt.Sprintf(
			"sellers.country = ANY(%s::origin_country[])", builder.add(filter.Countries)))
	}
	if len(filter.Kinds) > 0 {
		where = append(where, fmt.Sprintf(
			"sellers.kind = ANY(%s::seller_kind[])", builder.add(filter.Kinds)))
	}
	if len(filter.Regions) > 0 {
		where = append(where, "sellers.region = ANY("+builder.add(filter.Regions)+")")
	}
	if len(filter.Brands) > 0 {
		// Пересечение массивов: продавец подходит, если везёт хотя бы одну
		// из выбранных марок. Оператор && использует индекс GIN по brands.
		where = append(where, "sellers.brands && "+builder.add(filter.Brands))
	}
	if filter.VerifiedOnly {
		where = append(where, "sellers.verified_at IS NOT NULL")
	}
	if filter.Search != "" {
		// Полнотекстовый поиск по вектору плюс триграммы по названию:
		// вектор находит слова целиком, триграммы — опечатки и части слов.
		placeholder := builder.add(filter.Search)
		where = append(where, fmt.Sprintf(
			"(sellers.search_vector @@ plainto_tsquery('simple', %s) OR sellers.name %% %s)",
			placeholder, placeholder))
	}

	whereSQL := "true"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 24
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM sellers WHERE `+whereSQL, builder.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт продавцов: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT %s FROM sellers
		WHERE %s
		ORDER BY %s
		LIMIT %s OFFSET %s`,
		sellerColumns, whereSQL, sellerOrderBy(filter.Sort),
		builder.add(limit), builder.add(filter.Offset))

	rows, err := s.pool.Query(ctx, query, builder.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("выборка продавцов: %w", err)
	}
	defer rows.Close()

	var out []domain.Seller
	for rows.Next() {
		seller, err := scanSeller(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *seller)
	}
	return out, total, rows.Err()
}

// sellerOrderBy переводит имя сортировки в выражение SQL.
//
// Значение приходит из белого списка на уровне обработчика, но соответствие
// задаётся и здесь: строка из запроса ни при каких условиях не попадает в
// текст SQL напрямую.
func sellerOrderBy(sort string) string {
	switch sort {
	case "name":
		return "sellers.name, sellers.id"
	case "new":
		return "sellers.created_at DESC, sellers.id DESC"
	case "experience":
		return "sellers.export_experience_years DESC NULLS LAST, sellers.id"
	default:
		// По умолчанию проверенные и с высоким рейтингом идут первыми:
		// именно с ними дилеру безопаснее начинать работу.
		return "(sellers.verified_at IS NOT NULL) DESC, sellers.rating_avg DESC, sellers.rating_count DESC, sellers.id"
	}
}

// ByID возвращает карточку продавца.
func (s *Sellers) ByID(ctx context.Context, sellerID uuid.UUID) (*domain.Seller, error) {
	return scanSeller(s.pool.QueryRow(ctx,
		`SELECT `+sellerColumns+` FROM sellers WHERE sellers.id = $1`, sellerID))
}

// ByUserID возвращает карточку, принадлежащую пользователю.
func (s *Sellers) ByUserID(ctx context.Context, userID uuid.UUID) (*domain.Seller, error) {
	return scanSeller(s.pool.QueryRow(ctx,
		`SELECT `+sellerColumns+` FROM sellers WHERE sellers.user_id = $1`, userID))
}

// SellerParams — данные карточки продавца.
type SellerParams struct {
	UserID    *uuid.UUID
	CreatedBy uuid.UUID

	Country domain.Origin
	Kind    domain.SellerKind

	Name      string
	NameLocal string
	Region    string
	City      string
	Address   string

	Brands      []string
	Description string
	Website     string
	Contacts    map[string]string
	LogoURL     string

	MinOrderQty           int
	ExportExperienceYears *int
}

// Create добавляет продавца в справочник.
func (s *Sellers) Create(ctx context.Context, params SellerParams) (*domain.Seller, error) {
	contacts, err := json.Marshal(params.Contacts)
	if err != nil {
		return nil, fmt.Errorf("сериализация контактов: %w", err)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO sellers (
			user_id, created_by, country, kind,
			name, name_local, region, city, address,
			brands, description, website, contacts, logo_url,
			min_order_qty, export_experience_years
		) VALUES (
			$1, $2, $3, $4,
			$5, NULLIF($6, ''), $7, NULLIF($8, ''), NULLIF($9, ''),
			$10, $11, NULLIF($12, ''), $13, NULLIF($14, ''),
			$15, $16
		)
		RETURNING `+sellerColumns,
		params.UserID, params.CreatedBy, params.Country, params.Kind,
		params.Name, params.NameLocal, params.Region, params.City, params.Address,
		params.Brands, params.Description, params.Website, contacts, params.LogoURL,
		params.MinOrderQty, params.ExportExperienceYears)

	seller, err := scanSeller(row)
	if err != nil {
		if isUniqueViolation(err, "sellers_user_id_key") {
			return nil, ErrSellerProfileExists
		}
		return nil, err
	}
	return seller, nil
}

// EnsureDefault создаёт каркас карточки при регистрации продавца.
func (s *Sellers) EnsureDefault(ctx context.Context, userID uuid.UUID, fullName string) error {
	name := strings.TrimSpace(fullName)
	if name == "" {
		name = "Поставщик"
	}
	_, err := s.Create(ctx, SellerParams{
		UserID:    &userID,
		CreatedBy: userID,
		Country:   domain.OriginChina,
		Kind:      domain.SellerExporter,
		Name:      name,
		Region:    "не указан",
		Brands:    []string{},
		Contacts:  map[string]string{},
	})
	if errors.Is(err, ErrSellerProfileExists) {
		return nil
	}
	return err
}

// ErrSellerProfileExists — у пользователя уже есть карточка продавца.
var ErrSellerProfileExists = errors.New("карточка продавца уже создана")

// Update изменяет карточку продавца.
//
// Условие доступа встроено в запрос: продавец правит только свою карточку,
// администратор — любую. Проверка после выборки давала бы окно, в котором
// изменение уже применено, а право ещё не проверено.
func (s *Sellers) Update(ctx context.Context, sellerID uuid.UUID, editorID uuid.UUID, isAdmin bool, params SellerParams) (*domain.Seller, error) {
	contacts, err := json.Marshal(params.Contacts)
	if err != nil {
		return nil, fmt.Errorf("сериализация контактов: %w", err)
	}

	accessCondition := "sellers.user_id = $2"
	if isAdmin {
		accessCondition = "($2 IS NOT NULL)"
	}

	return scanSeller(s.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE sellers SET
			kind = $3,
			name = $4,
			name_local = NULLIF($5, ''),
			region = $6,
			city = NULLIF($7, ''),
			address = NULLIF($8, ''),
			brands = $9,
			description = $10,
			website = NULLIF($11, ''),
			contacts = $12,
			logo_url = NULLIF($13, ''),
			min_order_qty = $14,
			export_experience_years = $15
		WHERE sellers.id = $1 AND %s
		RETURNING %s`, accessCondition, sellerColumns),
		sellerID, editorID,
		params.Kind, params.Name, params.NameLocal, params.Region,
		params.City, params.Address, params.Brands, params.Description,
		params.Website, contacts, params.LogoURL,
		params.MinOrderQty, params.ExportExperienceYears))
}

// SetVerified отмечает продавца проверенным или снимает отметку.
func (s *Sellers) SetVerified(ctx context.Context, sellerID, adminID uuid.UUID, verified bool) error {
	var query string
	if verified {
		query = `UPDATE sellers SET verified_at = now(), verified_by = $2 WHERE id = $1`
	} else {
		query = `UPDATE sellers SET verified_at = NULL, verified_by = $2 WHERE id = $1`
	}

	tag, err := s.pool.Exec(ctx, query, sellerID, adminID)
	if err != nil {
		return fmt.Errorf("изменение отметки проверки продавца: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetActive включает или отключает карточку продавца.
func (s *Sellers) SetActive(ctx context.Context, sellerID, editorID uuid.UUID, isAdmin, active bool) error {
	accessCondition := "user_id = $2"
	if isAdmin {
		accessCondition = "($2 IS NOT NULL)"
	}

	tag, err := s.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE sellers SET is_active = $3 WHERE id = $1 AND %s`, accessCondition),
		sellerID, editorID, active)
	if err != nil {
		return fmt.Errorf("изменение видимости продавца: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RegionFacet — регион с числом продавцов.
type RegionFacet struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	Count   int    `json:"count"`
}

// Regions возвращает регионы для фильтра.
func (s *Sellers) Regions(ctx context.Context, countries []string) ([]RegionFacet, error) {
	condition := ""
	args := []any{}
	if len(countries) > 0 {
		condition = " AND country = ANY($1::origin_country[])"
		args = append(args, countries)
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT country::text, region, count(*)
		FROM sellers
		WHERE is_active%s
		GROUP BY country, region
		ORDER BY count(*) DESC, region
		LIMIT 200`, condition), args...)
	if err != nil {
		return nil, fmt.Errorf("выборка регионов продавцов: %w", err)
	}
	defer rows.Close()

	var out []RegionFacet
	for rows.Next() {
		var facet RegionFacet
		if err := rows.Scan(&facet.Country, &facet.Region, &facet.Count); err != nil {
			return nil, fmt.Errorf("разбор региона: %w", err)
		}
		out = append(out, facet)
	}
	return out, rows.Err()
}

// SellerBrands возвращает марки, встречающиеся у продавцов.
func (s *Sellers) SellerBrands(ctx context.Context, countries []string) ([]BrandFacet, error) {
	condition := ""
	args := []any{}
	if len(countries) > 0 {
		condition = " AND country = ANY($1::origin_country[])"
		args = append(args, countries)
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT brand, count(*)
		FROM sellers, unnest(brands) AS brand
		WHERE is_active%s
		GROUP BY brand
		ORDER BY count(*) DESC, brand
		LIMIT 300`, condition), args...)
	if err != nil {
		return nil, fmt.Errorf("выборка марок продавцов: %w", err)
	}
	defer rows.Close()

	var out []BrandFacet
	for rows.Next() {
		var facet BrandFacet
		if err := rows.Scan(&facet.Brand, &facet.Count); err != nil {
			return nil, fmt.Errorf("разбор марки: %w", err)
		}
		out = append(out, facet)
	}
	return out, rows.Err()
}

// --- Личные отметки дилеров -------------------------------------------------

// SellerLink — отметка дилера о работе с продавцом.
type SellerLink struct {
	SellerID   uuid.UUID `json:"seller_id"`
	Note       string    `json:"note"`
	IsTrusted  bool      `json:"is_trusted"`
	DealsCount int       `json:"deals_count"`
}

// UpsertLink сохраняет заметку дилера о продавце.
//
// Заметка приватная: другие дилеры её не видят. Это осознанно — список
// проверенных поставщиков и есть основной актив импортёра.
func (s *Sellers) UpsertLink(ctx context.Context, dealerID, sellerID uuid.UUID, note string, trusted bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dealer_seller_links (dealer_id, seller_id, note, is_trusted)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (dealer_id, seller_id) DO UPDATE
		SET note = excluded.note, is_trusted = excluded.is_trusted`,
		dealerID, sellerID, truncate(note, 2000), trusted)
	if err != nil {
		return fmt.Errorf("сохранение заметки о продавце: %w", err)
	}
	return nil
}

// Links возвращает отметки дилера по списку продавцов.
func (s *Sellers) Links(ctx context.Context, dealerID uuid.UUID, sellerIDs []uuid.UUID) (map[uuid.UUID]SellerLink, error) {
	if len(sellerIDs) == 0 {
		return map[uuid.UUID]SellerLink{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT seller_id, note, is_trusted, deals_count
		FROM dealer_seller_links
		WHERE dealer_id = $1 AND seller_id = ANY($2)`, dealerID, sellerIDs)
	if err != nil {
		return nil, fmt.Errorf("чтение заметок о продавцах: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]SellerLink, len(sellerIDs))
	for rows.Next() {
		var link SellerLink
		if err := rows.Scan(&link.SellerID, &link.Note, &link.IsTrusted, &link.DealsCount); err != nil {
			return nil, fmt.Errorf("разбор заметки: %w", err)
		}
		out[link.SellerID] = link
	}
	return out, rows.Err()
}

// DeleteLink удаляет отметку дилера.
func (s *Sellers) DeleteLink(ctx context.Context, dealerID, sellerID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM dealer_seller_links WHERE dealer_id = $1 AND seller_id = $2`,
		dealerID, sellerID)
	if err != nil {
		return fmt.Errorf("удаление заметки о продавце: %w", err)
	}
	return nil
}
