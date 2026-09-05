package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

// Cars — доступ к каталогу автомобилей.
type Cars struct {
	pool *Pool
}

func NewCars(pool *Pool) *Cars { return &Cars{pool: pool} }

// CarFilter — параметры выборки каталога.
//
// Все поля — уже проверенные значения: разбор и валидация выполняются в
// транспортном слое. Хранилище только строит запрос, поэтому здесь нет
// ни одной подстановки строки в SQL — только нумерованные параметры.
type CarFilter struct {
	Origins   []string
	Brands    []string
	Models    []string
	Bodies    []string
	Fuels     []string
	Gearboxes []string
	Drives    []string

	YearFrom, YearTo         *int
	PriceRubFrom, PriceRubTo *int64
	MileageTo                *int
	EngineFrom, EngineTo     *int
	PowerFrom                *int

	SteeringRight *bool
	Search        string

	// Statuses ограничивает выборку статусами. Пустое значение означает
	// только публично видимые объявления.
	Statuses []string
	DealerID *uuid.UUID
	SellerID *uuid.UUID

	// FavoriteUserID ограничивает выдачу лотами из избранного этого
	// пользователя. Пустое значение фильтр не включает.
	FavoriteUserID *uuid.UUID

	Sort   string
	Limit  int
	Cursor string
}

// Допустимые варианты сортировки. Ключ приходит от клиента, значение —
// фрагмент SQL. Клиент не может передать произвольное выражение: неизвестный
// ключ приводит к сортировке по умолчанию.
var carSortColumns = map[string]struct {
	column string
	desc   bool
}{
	"fresh":       {"published_at", true},
	"price_asc":   {"price_rub_minor", false},
	"price_desc":  {"price_rub_minor", true},
	"year_desc":   {"year", true},
	"mileage_asc": {"mileage_km", false},
}

// CarListItem — элемент выдачи каталога.
//
// Отдельный от domain.Car тип: в списке не нужны описание, комплектация и
// VIN, а тащить их для сотни карточек — лишний трафик и лишняя работа базы.
type CarListItem struct {
	ID     uuid.UUID
	Status domain.CarStatus
	Origin domain.Origin

	Brand      string
	Model      string
	Generation string
	Year       int
	MileageKM  int

	Fuel    domain.FuelType
	Gearbox domain.Transmission
	Drive   domain.Drivetrain
	Body    domain.BodyType

	EngineCC *int
	PowerHP  *int

	SteeringRight bool
	AuctionGrade  string

	PriceMinor      int64
	Currency        domain.Currency
	PriceRubMinor   int64
	TurnkeyRubMinor *int64

	Title      string
	CoverURL   string
	PhotoCount int
	PhotoURLs  []string

	DealerID *uuid.UUID
	SellerID *uuid.UUID

	IsFavorite bool

	PublishedAt *time.Time
	CreatedAt   time.Time
}

// CarPage — страница каталога.
type CarPage struct {
	Items      []CarListItem
	NextCursor string
	// Total заполняется только на первой странице: точный подсчёт по
	// многомиллионной таблице дороже самой выдачи, а на второй странице
	// клиенту он уже не нужен.
	Total *int
}

// argBuilder накапливает параметры запроса.
//
// Нужен, чтобы номера параметров ($1, $2, ...) не приходилось считать
// вручную: при десятке необязательных фильтров сбитая нумерация — самая
// частая и самая незаметная ошибка.
type argBuilder struct {
	args []any
}

func (b *argBuilder) add(value any) string {
	b.args = append(b.args, value)
	return "$" + strconv.Itoa(len(b.args))
}

// List возвращает страницу каталога.
func (c *Cars) List(ctx context.Context, filter CarFilter, viewerID uuid.UUID, withTotal bool) (*CarPage, error) {
	builder := &argBuilder{}
	where := c.buildWhere(filter, builder)

	sort, ok := carSortColumns[filter.Sort]
	if !ok {
		sort = carSortColumns["fresh"]
	}

	// Курсорная постраничность: условие «строго дальше последней строки
	// предыдущей страницы». В отличие от OFFSET, база не читает и не
	// отбрасывает предыдущие строки, поэтому сотая страница стоит столько
	// же, сколько первая.
	if filter.Cursor != "" {
		condition, err := c.cursorCondition(filter.Cursor, sort.column, sort.desc, builder)
		if err != nil {
			return nil, err
		}
		where = append(where, condition)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 24
	}

	direction := "ASC"
	if sort.desc {
		direction = "DESC"
	}

	// NULLS LAST для published_at: неопубликованные объявления в
	// пользовательской выдаче не должны оказываться первыми.
	orderBy := fmt.Sprintf("cars.%s %s NULLS LAST, cars.id %s", sort.column, direction, direction)

	favoriteExpr := "false"
	if viewerID != uuid.Nil {
		favoriteExpr = fmt.Sprintf(
			`EXISTS (SELECT 1 FROM favorites f WHERE f.car_id = cars.id AND f.user_id = %s)`,
			builder.add(viewerID))
	}

	query := fmt.Sprintf(`
		SELECT
			cars.id, cars.status, cars.origin,
			cars.brand, cars.model, COALESCE(cars.generation, ''), cars.year, cars.mileage_km,
			cars.fuel, cars.gearbox, cars.drive, cars.body,
			cars.engine_cc, cars.power_hp,
			cars.steering_right, COALESCE(cars.auction_grade, ''),
			cars.price_minor, cars.currency, cars.price_rub_minor, cars.turnkey_rub_minor,
			cars.title,
			COALESCE(photos.cover, ''), COALESCE(photos.count, 0), COALESCE(photos.urls, '{}'::text[]),
			cars.dealer_id, cars.seller_id,
			%s AS is_favorite,
			cars.published_at, cars.created_at
		FROM cars
		LEFT JOIN LATERAL (
			SELECT
				(ARRAY_AGG(url ORDER BY sort_order, created_at))[1] AS cover,
				COUNT(*)::int AS count,
				(ARRAY_AGG(url ORDER BY sort_order, created_at))[1:8] AS urls
			FROM car_photos
			WHERE car_photos.car_id = cars.id
		) AS photos ON true
		WHERE %s
		ORDER BY %s
		LIMIT %s`,
		favoriteExpr,
		strings.Join(where, " AND "),
		orderBy,
		builder.add(limit+1), // лишняя строка показывает, есть ли следующая страница
	)

	rows, err := c.pool.Query(ctx, query, builder.args...)
	if err != nil {
		return nil, fmt.Errorf("выборка каталога: %w", err)
	}
	defer rows.Close()

	items := make([]CarListItem, 0, limit)
	for rows.Next() {
		var item CarListItem
		if err := rows.Scan(
			&item.ID, &item.Status, &item.Origin,
			&item.Brand, &item.Model, &item.Generation, &item.Year, &item.MileageKM,
			&item.Fuel, &item.Gearbox, &item.Drive, &item.Body,
			&item.EngineCC, &item.PowerHP,
			&item.SteeringRight, &item.AuctionGrade,
			&item.PriceMinor, &item.Currency, &item.PriceRubMinor, &item.TurnkeyRubMinor,
			&item.Title,
			&item.CoverURL, &item.PhotoCount, &item.PhotoURLs,
			&item.DealerID, &item.SellerID,
			&item.IsFavorite,
			&item.PublishedAt, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("разбор строки каталога: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение каталога: %w", err)
	}

	page := &CarPage{}
	if len(items) > limit {
		last := items[limit-1]
		page.NextCursor = encodeCursor(last, sort.column)
		items = items[:limit]
	}
	page.Items = items

	if withTotal {
		total, err := c.count(ctx, filter)
		if err != nil {
			return nil, err
		}
		page.Total = &total
	}
	return page, nil
}

func (c *Cars) count(ctx context.Context, filter CarFilter) (int, error) {
	builder := &argBuilder{}
	where := c.buildWhere(filter, builder)

	// Подсчёт ограничен сверху: для фильтра, под который подходит полтаблицы,
	// точное число не нужно — интерфейс покажет «более 5000».
	query := fmt.Sprintf(`
		SELECT count(*) FROM (
			SELECT 1 FROM cars WHERE %s LIMIT 5000
		) AS limited`, strings.Join(where, " AND "))

	var total int
	if err := c.pool.QueryRow(ctx, query, builder.args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("подсчёт объявлений: %w", err)
	}
	return total, nil
}

func (c *Cars) buildWhere(filter CarFilter, builder *argBuilder) []string {
	where := make([]string, 0, 16)

	if len(filter.Statuses) > 0 {
		where = append(where, fmt.Sprintf("cars.status = ANY(%s::car_status[])", builder.add(filter.Statuses)))
	} else {
		// По умолчанию видны только объявления в продаже и брони: черновики
		// и архив не должны утекать в публичный каталог даже при ошибке в
		// вызывающем коде.
		where = append(where, "cars.status IN ('active', 'reserved')")
	}

	if len(filter.Origins) > 0 {
		where = append(where, fmt.Sprintf("cars.origin = ANY(%s::origin_country[])", builder.add(filter.Origins)))
	}
	if len(filter.Brands) > 0 {
		where = append(where, fmt.Sprintf("cars.brand = ANY(%s)", builder.add(filter.Brands)))
	}
	if len(filter.Models) > 0 {
		where = append(where, fmt.Sprintf("cars.model = ANY(%s)", builder.add(filter.Models)))
	}
	if len(filter.Bodies) > 0 {
		where = append(where, fmt.Sprintf("cars.body = ANY(%s::body_type[])", builder.add(filter.Bodies)))
	}
	if len(filter.Fuels) > 0 {
		where = append(where, fmt.Sprintf("cars.fuel = ANY(%s::fuel_type[])", builder.add(filter.Fuels)))
	}
	if len(filter.Gearboxes) > 0 {
		where = append(where, fmt.Sprintf("cars.gearbox = ANY(%s::transmission[])", builder.add(filter.Gearboxes)))
	}
	if len(filter.Drives) > 0 {
		where = append(where, fmt.Sprintf("cars.drive = ANY(%s::drivetrain[])", builder.add(filter.Drives)))
	}

	if filter.YearFrom != nil {
		where = append(where, "cars.year >= "+builder.add(*filter.YearFrom))
	}
	if filter.YearTo != nil {
		where = append(where, "cars.year <= "+builder.add(*filter.YearTo))
	}
	if filter.PriceRubFrom != nil {
		where = append(where, "cars.price_rub_minor >= "+builder.add(*filter.PriceRubFrom))
	}
	if filter.PriceRubTo != nil {
		where = append(where, "cars.price_rub_minor <= "+builder.add(*filter.PriceRubTo))
	}
	if filter.MileageTo != nil {
		where = append(where, "cars.mileage_km <= "+builder.add(*filter.MileageTo))
	}
	if filter.EngineFrom != nil {
		where = append(where, "cars.engine_cc >= "+builder.add(*filter.EngineFrom))
	}
	if filter.EngineTo != nil {
		where = append(where, "cars.engine_cc <= "+builder.add(*filter.EngineTo))
	}
	if filter.PowerFrom != nil {
		where = append(where, "cars.power_hp >= "+builder.add(*filter.PowerFrom))
	}
	if filter.SteeringRight != nil {
		where = append(where, "cars.steering_right = "+builder.add(*filter.SteeringRight))
	}

	if filter.DealerID != nil {
		where = append(where, "cars.dealer_id = "+builder.add(*filter.DealerID))
	}
	if filter.SellerID != nil {
		where = append(where, "cars.seller_id = "+builder.add(*filter.SellerID))
	}
	if filter.FavoriteUserID != nil {
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM favorites f WHERE f.car_id = cars.id AND f.user_id = %s)",
			builder.add(*filter.FavoriteUserID)))
	}

	if filter.Search != "" {
		// websearch_to_tsquery принимает пользовательский ввод как есть:
		// кавычки, минусы и лишние операторы не приводят к ошибке разбора,
		// в отличие от to_tsquery.
		where = append(where, fmt.Sprintf(
			"cars.search_vector @@ websearch_to_tsquery('russian', %s)", builder.add(filter.Search)))
	}

	return where
}

// --- Курсор -----------------------------------------------------------------

// encodeCursor упаковывает значение сортировки и идентификатор последней
// строки. Кодирование base64 не является защитой, оно лишь избавляет от
// экранирования при передаче в URL.
func encodeCursor(item CarListItem, column string) string {
	var value string
	switch column {
	case "price_rub_minor":
		value = strconv.FormatInt(item.PriceRubMinor, 10)
	case "year":
		value = strconv.Itoa(item.Year)
	case "mileage_km":
		value = strconv.Itoa(item.MileageKM)
	default:
		if item.PublishedAt != nil {
			value = item.PublishedAt.UTC().Format(time.RFC3339Nano)
		} else {
			value = item.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value + "|" + item.ID.String()))
}

var errBadCursor = errors.New("некорректный курсор постраничной выдачи")

// cursorCondition строит условие продолжения выдачи.
//
// Сравнение идёт по паре (значение сортировки, id): без второго поля строки
// с одинаковой ценой либо дублировались бы между страницами, либо пропадали.
func (c *Cars) cursorCondition(cursor, column string, desc bool, builder *argBuilder) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", errBadCursor
	}

	value, idPart, found := strings.Cut(string(raw), "|")
	if !found {
		return "", errBadCursor
	}

	lastID, err := uuid.Parse(idPart)
	if err != nil {
		return "", errBadCursor
	}

	comparison := ">"
	if desc {
		comparison = "<"
	}

	var typedValue any
	switch column {
	case "price_rub_minor":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", errBadCursor
		}
		typedValue = parsed
	case "year", "mileage_km":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "", errBadCursor
		}
		typedValue = parsed
	default:
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return "", errBadCursor
		}
		typedValue = parsed
	}

	valuePlaceholder := builder.add(typedValue)
	idPlaceholder := builder.add(lastID)

	return fmt.Sprintf("(cars.%s, cars.id) %s (%s, %s)",
		column, comparison, valuePlaceholder, idPlaceholder), nil
}

// --- Одно объявление --------------------------------------------------------

const carColumns = `
	cars.id, cars.dealer_id, cars.seller_id, cars.status, cars.origin,
	cars.brand, cars.model, COALESCE(cars.generation, ''), COALESCE(cars.trim_level, ''), cars.year,
	cars.mileage_km, cars.engine_cc, cars.power_hp,
	cars.fuel, cars.gearbox, cars.drive, cars.body, COALESCE(cars.color, ''), cars.seats, cars.steering_right,
	COALESCE(cars.auction_grade, ''), COALESCE(cars.interior_grade, ''),
	COALESCE(cars.auction_lot_number, ''), cars.auction_date,
	COALESCE(cars.vin, ''), cars.vin_visible,
	cars.price_minor, cars.currency, cars.price_rub_minor,
	cars.turnkey_rub_minor, cars.customs_rub_minor, cars.delivery_days,
	cars.title, cars.description, cars.equipment,
	cars.views_count, cars.requests_count,
	cars.published_at, cars.sold_at, cars.created_at, cars.updated_at`

func scanCar(row pgx.Row) (*domain.Car, error) {
	var car domain.Car
	var equipment []byte

	err := row.Scan(
		&car.ID, &car.DealerID, &car.SellerID, &car.Status, &car.Origin,
		&car.Brand, &car.Model, &car.Generation, &car.TrimLevel, &car.Year,
		&car.MileageKM, &car.EngineCC, &car.PowerHP,
		&car.Fuel, &car.Gearbox, &car.Drive, &car.Body, &car.Color, &car.Seats, &car.SteeringRight,
		&car.AuctionGrade, &car.InteriorGrade,
		&car.AuctionLotNumber, &car.AuctionDate,
		&car.VIN, &car.VINVisible,
		&car.PriceMinor, &car.Currency, &car.PriceRubMinor,
		&car.TurnkeyRubMinor, &car.CustomsRubMinor, &car.DeliveryDays,
		&car.Title, &car.Description, &equipment,
		&car.ViewsCount, &car.RequestsCount,
		&car.PublishedAt, &car.SoldAt, &car.CreatedAt, &car.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение объявления: %w", err)
	}

	car.Equipment = decodeStringArray(equipment)
	return &car, nil
}

// ByID возвращает объявление вместе с фотографиями.
func (c *Cars) ByID(ctx context.Context, id uuid.UUID) (*domain.Car, error) {
	car, err := scanCar(c.pool.QueryRow(ctx, `SELECT `+carColumns+` FROM cars WHERE cars.id = $1`, id))
	if err != nil {
		return nil, err
	}

	photos, err := c.photos(ctx, id)
	if err != nil {
		return nil, err
	}
	car.Photos = photos
	return car, nil
}

func (c *Cars) photos(ctx context.Context, carID uuid.UUID) ([]domain.CarPhoto, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, url, COALESCE(thumb_url, ''), COALESCE(width, 0), COALESCE(height, 0), sort_order
		FROM car_photos WHERE car_id = $1
		ORDER BY sort_order, created_at`, carID)
	if err != nil {
		return nil, fmt.Errorf("чтение фотографий объявления: %w", err)
	}
	defer rows.Close()

	var out []domain.CarPhoto
	for rows.Next() {
		var photo domain.CarPhoto
		if err := rows.Scan(&photo.ID, &photo.URL, &photo.ThumbURL,
			&photo.Width, &photo.Height, &photo.SortOrder); err != nil {
			return nil, fmt.Errorf("разбор фотографии: %w", err)
		}
		out = append(out, photo)
	}
	return out, rows.Err()
}

// IncrementViews увеличивает счётчик просмотров.
//
// Отдельный запрос без блокировки: точность счётчика просмотров не
// критична, а строка объявления не должна блокироваться на каждом
// открытии карточки.
func (c *Cars) IncrementViews(ctx context.Context, id uuid.UUID) {
	_, _ = c.pool.Exec(ctx, `UPDATE cars SET views_count = views_count + 1 WHERE id = $1`, id)
}

// CarInput — данные для создания и изменения объявления.
type CarInput struct {
	DealerID *uuid.UUID
	SellerID *uuid.UUID

	Origin domain.Origin

	Brand      string
	Model      string
	Generation string
	TrimLevel  string
	Year       int

	MileageKM int
	EngineCC  *int
	PowerHP   *int

	Fuel          domain.FuelType
	Gearbox       domain.Transmission
	Drive         domain.Drivetrain
	Body          domain.BodyType
	Color         string
	Seats         *int
	SteeringRight bool

	AuctionGrade     string
	InteriorGrade    string
	AuctionLotNumber string
	AuctionDate      *time.Time

	VIN        string
	VINVisible bool

	PriceMinor      int64
	Currency        domain.Currency
	PriceRubMinor   int64
	TurnkeyRubMinor *int64
	CustomsRubMinor *int64
	DeliveryDays    *int

	Title       string
	Description string
	Equipment   []string
}

// Create создаёт объявление в статусе черновика.
func (c *Cars) Create(ctx context.Context, input CarInput) (*domain.Car, error) {
	row := c.pool.QueryRow(ctx, `
		INSERT INTO cars (
			dealer_id, seller_id, status, origin,
			brand, model, generation, trim_level, year,
			mileage_km, engine_cc, power_hp,
			fuel, gearbox, drive, body, color, seats, steering_right,
			auction_grade, interior_grade, auction_lot_number, auction_date,
			vin, vin_visible,
			price_minor, currency, price_rub_minor,
			turnkey_rub_minor, customs_rub_minor, delivery_days,
			title, description, equipment
		) VALUES (
			$1, $2, 'draft', $3,
			$4, $5, NULLIF($6, ''), NULLIF($7, ''), $8,
			$9, $10, $11,
			$12, $13, $14, $15, NULLIF($16, ''), $17, $18,
			NULLIF($19, ''), NULLIF($20, ''), NULLIF($21, ''), $22,
			NULLIF($23, ''), $24,
			$25, $26, $27,
			$28, $29, $30,
			$31, $32, $33
		)
		RETURNING `+carColumns,
		input.DealerID, input.SellerID, input.Origin,
		input.Brand, input.Model, input.Generation, input.TrimLevel, input.Year,
		input.MileageKM, input.EngineCC, input.PowerHP,
		input.Fuel, input.Gearbox, input.Drive, input.Body, input.Color, input.Seats, input.SteeringRight,
		input.AuctionGrade, input.InteriorGrade, input.AuctionLotNumber, input.AuctionDate,
		strings.ToUpper(input.VIN), input.VINVisible,
		input.PriceMinor, input.Currency, input.PriceRubMinor,
		input.TurnkeyRubMinor, input.CustomsRubMinor, input.DeliveryDays,
		input.Title, input.Description, encodeStringArray(input.Equipment),
	)

	car, err := scanCar(row)
	if err != nil {
		return nil, mapCarConstraint(err)
	}
	return car, nil
}

// Update изменяет объявление. Условие по dealer_id обязательно: без него
// любой дилер мог бы отредактировать чужое объявление, подставив его
// идентификатор.
func (c *Cars) Update(ctx context.Context, carID uuid.UUID, dealerID uuid.UUID, input CarInput) (*domain.Car, error) {
	row := c.pool.QueryRow(ctx, `
		UPDATE cars SET
			origin = $3,
			brand = $4, model = $5, generation = NULLIF($6, ''), trim_level = NULLIF($7, ''), year = $8,
			mileage_km = $9, engine_cc = $10, power_hp = $11,
			fuel = $12, gearbox = $13, drive = $14, body = $15, color = NULLIF($16, ''),
			seats = $17, steering_right = $18,
			auction_grade = NULLIF($19, ''), interior_grade = NULLIF($20, ''),
			auction_lot_number = NULLIF($21, ''), auction_date = $22,
			vin = NULLIF($23, ''), vin_visible = $24,
			price_minor = $25, currency = $26, price_rub_minor = $27,
			turnkey_rub_minor = $28, customs_rub_minor = $29, delivery_days = $30,
			title = $31, description = $32, equipment = $33
		WHERE cars.id = $1 AND cars.dealer_id = $2
		RETURNING `+carColumns,
		carID, dealerID, input.Origin,
		input.Brand, input.Model, input.Generation, input.TrimLevel, input.Year,
		input.MileageKM, input.EngineCC, input.PowerHP,
		input.Fuel, input.Gearbox, input.Drive, input.Body, input.Color, input.Seats, input.SteeringRight,
		input.AuctionGrade, input.InteriorGrade, input.AuctionLotNumber, input.AuctionDate,
		strings.ToUpper(input.VIN), input.VINVisible,
		input.PriceMinor, input.Currency, input.PriceRubMinor,
		input.TurnkeyRubMinor, input.CustomsRubMinor, input.DeliveryDays,
		input.Title, input.Description, encodeStringArray(input.Equipment),
	)

	car, err := scanCar(row)
	if err != nil {
		return nil, mapCarConstraint(err)
	}
	return car, nil
}

// SetStatus меняет статус объявления.
func (c *Cars) SetStatus(ctx context.Context, carID uuid.UUID, dealerID *uuid.UUID, status domain.CarStatus) error {
	query := `
		UPDATE cars SET
			status = $2::car_status,
			published_at = CASE WHEN $2::car_status = 'active' AND published_at IS NULL THEN now() ELSE published_at END,
			sold_at = CASE WHEN $2::car_status = 'sold' THEN now() ELSE sold_at END
		WHERE id = $1`
	args := []any{carID, string(status)}

	if dealerID != nil {
		query += ` AND dealer_id = $3`
		args = append(args, *dealerID)
	}

	tag, err := c.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("изменение статуса объявления: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete архивирует объявление.
//
// Физическое удаление невозможно: на объявление ссылаются заявки и сделки,
// а история сделки должна оставаться читаемой.
func (c *Cars) Delete(ctx context.Context, carID, dealerID uuid.UUID) error {
	tag, err := c.pool.Exec(ctx,
		`UPDATE cars SET status = 'archived' WHERE id = $1 AND dealer_id = $2`, carID, dealerID)
	if err != nil {
		return fmt.Errorf("архивация объявления: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplacePhotos заменяет набор фотографий объявления.
func (c *Cars) ReplacePhotos(ctx context.Context, carID, dealerID uuid.UUID, photos []domain.CarPhoto) error {
	return c.pool.InTx(ctx, func(tx pgx.Tx) error {
		var owned uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM cars WHERE id = $1 AND dealer_id = $2`, carID, dealerID,
		).Scan(&owned); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("проверка владельца объявления: %w", err)
		}

		if _, err := tx.Exec(ctx, `DELETE FROM car_photos WHERE car_id = $1`, carID); err != nil {
			return fmt.Errorf("удаление старых фотографий: %w", err)
		}

		for i, photo := range photos {
			if _, err := tx.Exec(ctx, `
				INSERT INTO car_photos (car_id, url, thumb_url, width, height, sort_order)
				VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)`,
				carID, photo.URL, photo.ThumbURL, photo.Width, photo.Height, i); err != nil {
				return fmt.Errorf("добавление фотографии: %w", err)
			}
		}
		return nil
	})
}

// --- Избранное --------------------------------------------------------------

// AddFavorite добавляет объявление в избранное.
func (c *Cars) AddFavorite(ctx context.Context, userID, carID uuid.UUID) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO favorites (user_id, car_id) VALUES ($1, $2)
		ON CONFLICT (user_id, car_id) DO NOTHING`, userID, carID)
	if err != nil {
		return fmt.Errorf("добавление в избранное: %w", err)
	}
	return nil
}

// RemoveFavorite убирает объявление из избранного.
func (c *Cars) RemoveFavorite(ctx context.Context, userID, carID uuid.UUID) error {
	_, err := c.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id = $1 AND car_id = $2`, userID, carID)
	if err != nil {
		return fmt.Errorf("удаление из избранного: %w", err)
	}
	return nil
}

// BrandFacet — марка с числом объявлений.
type BrandFacet struct {
	Brand string `json:"brand"`
	Count int    `json:"count"`
}

// Brands возвращает список марок с количеством активных объявлений.
//
// Нужен для фильтра каталога: показывать марку, по которой нет ни одной
// машины, — прямой путь к пустой выдаче.
func (c *Cars) Brands(ctx context.Context, origins []string) ([]BrandFacet, error) {
	builder := &argBuilder{}
	where := []string{"status IN ('active', 'reserved')"}
	if len(origins) > 0 {
		where = append(where, fmt.Sprintf("origin = ANY(%s::origin_country[])", builder.add(origins)))
	}

	rows, err := c.pool.Query(ctx, fmt.Sprintf(`
		SELECT brand, count(*) AS count
		FROM cars WHERE %s
		GROUP BY brand
		ORDER BY count DESC, brand`, strings.Join(where, " AND ")), builder.args...)
	if err != nil {
		return nil, fmt.Errorf("выборка марок: %w", err)
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

func mapCarConstraint(err error) error {
	if isUniqueViolation(err, "cars_vin_unique") {
		return errors.New("автомобиль с таким VIN уже размещён в каталоге")
	}
	return err
}
