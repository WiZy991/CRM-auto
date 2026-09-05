package httpx

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// CarHandler — обработчики каталога автомобилей.
type CarHandler struct {
	catalog *service.Catalog
	social  *service.Social
}

func NewCarHandler(catalog *service.Catalog, social *service.Social) *CarHandler {
	return &CarHandler{catalog: catalog, social: social}
}

// Допустимые значения фильтров. Списки перечислены явно, чтобы значение из
// запроса не могло попасть в SQL без проверки.
var (
	allowedOrigins      = []string{"cn", "jp"}
	allowedBodies       = []string{"sedan", "suv", "crossover", "hatchback", "wagon", "coupe", "minivan", "pickup", "van", "liftback"}
	allowedFuels        = []string{"petrol", "diesel", "hybrid", "phev", "electric"}
	allowedGearboxes    = []string{"at", "mt", "cvt", "dct", "amt"}
	allowedDrives       = []string{"fwd", "rwd", "awd"}
	allowedCarStatuses  = []string{"draft", "moderation", "active", "reserved", "sold", "archived"}
	allowedCatalogSorts = []string{"fresh", "price_asc", "price_desc", "year_desc", "mileage_asc"}
)

// parseCarFilter читает фильтры каталога из строки запроса.
func parseCarFilter(r *http.Request) (store.CarFilter, error) {
	q := NewQuery(r)

	filter := store.CarFilter{
		Origins:   q.EnumList("origin", 2, allowedOrigins...),
		Brands:    q.StringList("brand", 20, 60),
		Models:    q.StringList("model", 30, 60),
		Bodies:    q.EnumList("body", 10, allowedBodies...),
		Fuels:     q.EnumList("fuel", 5, allowedFuels...),
		Gearboxes: q.EnumList("gearbox", 5, allowedGearboxes...),
		Drives:    q.EnumList("drive", 3, allowedDrives...),

		YearFrom:  q.Int("year_from", 1980, 2100),
		YearTo:    q.Int("year_to", 1980, 2100),
		MileageTo: q.Int("mileage_to", 0, 2_000_000),

		// Цены в запросе указываются в рублях, внутри — в копейках.
		PriceRubFrom: rublesToMinor(q.Int64("price_from", 0, 1_000_000_000)),
		PriceRubTo:   rublesToMinor(q.Int64("price_to", 0, 1_000_000_000)),

		EngineFrom: q.Int("engine_from", 0, 10_000),
		EngineTo:   q.Int("engine_to", 0, 10_000),
		PowerFrom:  q.Int("power_from", 0, 2_000),

		SteeringRight: q.Bool("steering_right"),
		Search:        q.String("q", 200),
		Sort:          q.Enum("sort", allowedCatalogSorts...),
	}

	page := q.Pagination(24, 60)
	filter.Limit = page.Limit
	filter.Cursor = page.Cursor

	if err := q.Err(); err != nil {
		return store.CarFilter{}, err
	}

	// Перепутанные границы диапазона — частая ошибка в интерфейсе фильтров.
	// Молча вернуть пустой результат хуже, чем сказать, в чём дело.
	if filter.YearFrom != nil && filter.YearTo != nil && *filter.YearFrom > *filter.YearTo {
		return store.CarFilter{}, apierr.Validation(map[string]string{
			"year_from": "начало диапазона больше конца",
		})
	}
	if filter.PriceRubFrom != nil && filter.PriceRubTo != nil && *filter.PriceRubFrom > *filter.PriceRubTo {
		return store.CarFilter{}, apierr.Validation(map[string]string{
			"price_from": "начало диапазона больше конца",
		})
	}
	return filter, nil
}

func rublesToMinor(value *int64) *int64 {
	if value == nil {
		return nil
	}
	minor := *value * 100
	return &minor
}

func favoritesOnly(r *http.Request) bool {
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("favorites")))
	return value == "1" || value == "true" || value == "yes"
}

// --- Представления ----------------------------------------------------------

type carListItemResponse struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
	Origin string    `json:"origin"`

	Brand      string `json:"brand"`
	Model      string `json:"model"`
	Generation string `json:"generation,omitempty"`
	Year       int    `json:"year"`
	MileageKM  int    `json:"mileage_km"`

	Fuel    string `json:"fuel"`
	Gearbox string `json:"gearbox"`
	Drive   string `json:"drive"`
	Body    string `json:"body"`

	EngineCC *int `json:"engine_cc,omitempty"`
	PowerHP  *int `json:"power_hp,omitempty"`

	SteeringRight bool   `json:"steering_right"`
	AuctionGrade  string `json:"auction_grade,omitempty"`

	PriceMinor    int64  `json:"price_minor"`
	Currency      string `json:"currency"`
	PriceRubMinor int64  `json:"price_rub_minor"`
	PriceLabel    string `json:"price_label"`
	TurnkeyLabel  string `json:"turnkey_label,omitempty"`

	Title      string   `json:"title"`
	CoverURL   string   `json:"cover_url,omitempty"`
	PhotoCount int      `json:"photo_count"`
	PhotoURLs  []string `json:"photo_urls,omitempty"`

	IsFavorite  bool       `json:"is_favorite"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

func toCarListItem(item store.CarListItem) carListItemResponse {
	response := carListItemResponse{
		ID:     item.ID,
		Status: string(item.Status),
		Origin: string(item.Origin),

		Brand:      item.Brand,
		Model:      item.Model,
		Generation: item.Generation,
		Year:       item.Year,
		MileageKM:  item.MileageKM,

		Fuel:    string(item.Fuel),
		Gearbox: string(item.Gearbox),
		Drive:   string(item.Drive),
		Body:    string(item.Body),

		EngineCC: item.EngineCC,
		PowerHP:  item.PowerHP,

		SteeringRight: item.SteeringRight,
		AuctionGrade:  item.AuctionGrade,

		PriceMinor:    item.PriceMinor,
		Currency:      string(item.Currency),
		PriceRubMinor: item.PriceRubMinor,
		// Готовая подпись приходит с сервера: правила разделения разрядов и
		// символ валюты не должны дублироваться во фронтенде.
		PriceLabel: money.FormatRub(item.PriceRubMinor),

		Title:      item.Title,
		CoverURL:   item.CoverURL,
		PhotoCount: item.PhotoCount,
		PhotoURLs:  item.PhotoURLs,

		IsFavorite:  item.IsFavorite,
		PublishedAt: item.PublishedAt,
	}

	if item.TurnkeyRubMinor != nil {
		response.TurnkeyLabel = money.FormatRub(*item.TurnkeyRubMinor)
	}
	return response
}

type carDetailResponse struct {
	carListItemResponse

	TrimLevel string `json:"trim_level,omitempty"`
	Color     string `json:"color,omitempty"`
	Seats     *int   `json:"seats,omitempty"`

	InteriorGrade    string     `json:"interior_grade,omitempty"`
	AuctionLotNumber string     `json:"auction_lot_number,omitempty"`
	AuctionDate      *time.Time `json:"auction_date,omitempty"`

	VIN string `json:"vin,omitempty"`

	CustomsRubMinor *int64 `json:"customs_rub_minor,omitempty"`
	DeliveryDays    *int   `json:"delivery_days,omitempty"`

	Description string   `json:"description"`
	Equipment   []string `json:"equipment"`

	Photos []domain.CarPhoto `json:"photos"`

	ViewsCount    int  `json:"views_count"`
	RequestsCount int  `json:"requests_count"`
	CanEdit       bool `json:"can_edit"`

	DisplayName string `json:"display_name"`
}

func toCarDetail(view *service.CarView) carDetailResponse {
	car := view.Car

	base := carListItemResponse{
		ID:     car.ID,
		Status: string(car.Status),
		Origin: string(car.Origin),

		Brand:      car.Brand,
		Model:      car.Model,
		Generation: car.Generation,
		Year:       car.Year,
		MileageKM:  car.MileageKM,

		Fuel:    string(car.Fuel),
		Gearbox: string(car.Gearbox),
		Drive:   string(car.Drive),
		Body:    string(car.Body),

		EngineCC: car.EngineCC,
		PowerHP:  car.PowerHP,

		SteeringRight: car.SteeringRight,
		AuctionGrade:  car.AuctionGrade,

		PriceMinor:    car.PriceMinor,
		Currency:      string(car.Currency),
		PriceRubMinor: car.PriceRubMinor,
		PriceLabel:    money.FormatRub(car.PriceRubMinor),

		Title:      car.Title,
		PhotoCount: len(car.Photos),

		PublishedAt: car.PublishedAt,
	}
	if len(car.Photos) > 0 {
		base.CoverURL = car.Photos[0].URL
	}
	if car.TurnkeyRubMinor != nil {
		base.TurnkeyLabel = money.FormatRub(*car.TurnkeyRubMinor)
	}

	equipment := car.Equipment
	if equipment == nil {
		equipment = []string{}
	}
	photos := car.Photos
	if photos == nil {
		photos = []domain.CarPhoto{}
	}

	return carDetailResponse{
		carListItemResponse: base,

		TrimLevel: car.TrimLevel,
		Color:     car.Color,
		Seats:     car.Seats,

		InteriorGrade:    car.InteriorGrade,
		AuctionLotNumber: car.AuctionLotNumber,
		AuctionDate:      car.AuctionDate,

		VIN: view.VIN,

		CustomsRubMinor: car.CustomsRubMinor,
		DeliveryDays:    car.DeliveryDays,

		Description: car.Description,
		Equipment:   equipment,
		Photos:      photos,

		ViewsCount:    car.ViewsCount,
		RequestsCount: car.RequestsCount,
		CanEdit:       view.CanEdit,

		DisplayName: car.DisplayName(),
	}
}

// --- Обработчики ------------------------------------------------------------

// List — GET /api/v1/cars
func (h *CarHandler) List(w http.ResponseWriter, r *http.Request) {
	filter, err := parseCarFilter(r)
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if favoritesOnly(r) {
		if actor.IsZero() {
			Error(w, r, apierr.Unauthorized("Войдите, чтобы открыть избранное"))
			return
		}
		filter.FavoriteUserID = &actor.UserID
	}
	// Общее число считается только на первой странице.
	page, err := h.catalog.List(r.Context(), filter, actor.UserID, filter.Cursor == "")
	if err != nil {
		Error(w, r, err)
		return
	}

	items := make([]carListItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toCarListItem(item))
	}

	JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": page.NextCursor,
		"total":       page.Total,
	})
}

// Get — GET /api/v1/cars/{id}
func (h *CarHandler) Get(w http.ResponseWriter, r *http.Request) {
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	view, err := h.catalog.Get(r.Context(), carID, service.Viewer{
		UserID: actor.UserID, Role: actor.Role,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"car": toCarDetail(view)})
}

// Dictionaries — GET /api/v1/cars/dictionaries
func (h *CarHandler) Dictionaries(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	origins := q.EnumList("origin", 2, allowedOrigins...)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	brands, err := h.catalog.Brands(r.Context(), origins)
	if err != nil {
		Error(w, r, err)
		return
	}

	brandOpts := make([]map[string]any, 0, len(brands))
	for _, brand := range brands {
		brandOpts = append(brandOpts, map[string]any{
			"value": brand.Brand, "title": brand.Brand, "count": brand.Count,
		})
	}

	JSON(w, http.StatusOK, map[string]any{
		"dictionaries": domain.CarDictionaries(),
		"brands":       brandOpts,
		"sorts": []map[string]string{
			{"value": "fresh", "title": "Сначала новые"},
			{"value": "price_asc", "title": "Дешевле"},
			{"value": "price_desc", "title": "Дороже"},
			{"value": "year_desc", "title": "Год выпуска"},
			{"value": "mileage_asc", "title": "Меньше пробег"},
		},
	})
}

// MyCars — GET /api/v1/cars/my
func (h *CarHandler) MyCars(w http.ResponseWriter, r *http.Request) {
	filter, err := parseCarFilter(r)
	if err != nil {
		Error(w, r, err)
		return
	}

	q := NewQuery(r)
	filter.Statuses = q.EnumList("status", 6, allowedCarStatuses...)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	page, err := h.catalog.ListForDealer(r.Context(), actor.UserID, filter)
	if err != nil {
		Error(w, r, err)
		return
	}

	items := make([]carListItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toCarListItem(item))
	}

	JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": page.NextCursor,
		"total":       page.Total,
	})
}

// carFormRequest — тело запроса создания и изменения объявления.
type carFormRequest struct {
	Origin string `json:"origin"`

	Brand      string `json:"brand"`
	Model      string `json:"model"`
	Generation string `json:"generation"`
	TrimLevel  string `json:"trim_level"`
	Year       int    `json:"year"`

	MileageKM int  `json:"mileage_km"`
	EngineCC  *int `json:"engine_cc"`
	PowerHP   *int `json:"power_hp"`

	Fuel          string `json:"fuel"`
	Gearbox       string `json:"gearbox"`
	Drive         string `json:"drive"`
	Body          string `json:"body"`
	Color         string `json:"color"`
	Seats         *int   `json:"seats"`
	SteeringRight bool   `json:"steering_right"`

	AuctionGrade     string `json:"auction_grade"`
	InteriorGrade    string `json:"interior_grade"`
	AuctionLotNumber string `json:"auction_lot_number"`
	AuctionDate      string `json:"auction_date"`

	VIN        string `json:"vin"`
	VINVisible bool   `json:"vin_visible"`

	PriceMinor      int64  `json:"price_minor"`
	Currency        string `json:"currency"`
	TurnkeyRubMinor *int64 `json:"turnkey_rub_minor"`
	CustomsRubMinor *int64 `json:"customs_rub_minor"`
	DeliveryDays    *int   `json:"delivery_days"`

	Title       string   `json:"title"`
	Description string   `json:"description"`
	Equipment   []string `json:"equipment"`

	PhotoURLs []string `json:"photo_urls"`
}

func (req carFormRequest) toForm() (service.CarForm, error) {
	form := service.CarForm{
		Origin:     req.Origin,
		Brand:      req.Brand,
		Model:      req.Model,
		Generation: req.Generation,
		TrimLevel:  req.TrimLevel,
		Year:       req.Year,

		MileageKM: req.MileageKM,
		EngineCC:  req.EngineCC,
		PowerHP:   req.PowerHP,

		Fuel:          req.Fuel,
		Gearbox:       req.Gearbox,
		Drive:         req.Drive,
		Body:          req.Body,
		Color:         req.Color,
		Seats:         req.Seats,
		SteeringRight: req.SteeringRight,

		AuctionGrade:     req.AuctionGrade,
		InteriorGrade:    req.InteriorGrade,
		AuctionLotNumber: req.AuctionLotNumber,

		VIN:        req.VIN,
		VINVisible: req.VINVisible,

		PriceMinor:      req.PriceMinor,
		Currency:        req.Currency,
		TurnkeyRubMinor: req.TurnkeyRubMinor,
		CustomsRubMinor: req.CustomsRubMinor,
		DeliveryDays:    req.DeliveryDays,

		Title:       req.Title,
		Description: req.Description,
		Equipment:   req.Equipment,
	}

	if req.AuctionDate != "" {
		parsed, err := time.Parse("2006-01-02", req.AuctionDate)
		if err != nil {
			return service.CarForm{}, apierr.Validation(map[string]string{
				"auction_date": "ожидалась дата в формате ГГГГ-ММ-ДД",
			})
		}
		form.AuctionDate = &parsed
	}

	if len(req.PhotoURLs) > 30 {
		return service.CarForm{}, apierr.Validation(map[string]string{
			"photo_urls": "не более 30 фотографий на объявление",
		})
	}

	// Принимаются только пути, выданные сервисом загрузки: внешний адрес в
	// этом поле означал бы, что каталог отдаёт изображения с чужого домена
	// и подгружает содержимое, которое мы не контролируем.
	for _, url := range req.PhotoURLs {
		if !isUploadedPath(url) {
			return service.CarForm{}, apierr.Validation(map[string]string{
				"photo_urls": "допустимы только файлы, загруженные через сервис загрузки",
			})
		}
		form.Photos = append(form.Photos, domain.CarPhoto{URL: url})
	}
	return form, nil
}

// Create — POST /api/v1/cars
func (h *CarHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req carFormRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	form, err := req.toForm()
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	car, err := h.catalog.Create(r.Context(), actor.UserID, form, requestMeta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{
		"car": toCarDetail(&service.CarView{Car: car, VIN: car.VIN, CanEdit: true}),
	})
}

// Update — PUT /api/v1/cars/{id}
func (h *CarHandler) Update(w http.ResponseWriter, r *http.Request) {
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var req carFormRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	form, err := req.toForm()
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	car, err := h.catalog.Update(r.Context(), carID, actor.UserID, form, requestMeta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"car": toCarDetail(&service.CarView{Car: car, VIN: car.VIN, CanEdit: true}),
	})
}

// ChangeStatus — PATCH /api/v1/cars/{id}/status
func (h *CarHandler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	status := domain.CarStatus(req.Status)
	if !status.Valid() {
		Error(w, r, apierr.Validation(map[string]string{"status": "недопустимый статус"}))
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.catalog.ChangeStatus(r.Context(), carID,
		service.Viewer{UserID: actor.UserID, Role: actor.Role}, status, requestMeta(r)); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"status": string(status), "title": status.Title()})
}

// PublishSocial — POST /api/v1/cars/{id}/publish-social
func (h *CarHandler) PublishSocial(w http.ResponseWriter, r *http.Request) {
	if h.social == nil {
		Error(w, r, apierr.Unavailable("Автопост временно недоступен"))
		return
	}
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	actor := ActorFrom(r.Context())
	if err := h.social.EnqueueManual(r.Context(), actor.UserID, carID); err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusAccepted, map[string]any{"queued": true})
}

// Delete — DELETE /api/v1/cars/{id}
func (h *CarHandler) Delete(w http.ResponseWriter, r *http.Request) {
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.catalog.Delete(r.Context(), carID, actor.UserID, requestMeta(r)); err != nil {
		Error(w, r, err)
		return
	}
	NoContent(w)
}

// SetFavorite — PUT/DELETE /api/v1/cars/{id}/favorite
func (h *CarHandler) SetFavorite(favorite bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		carID, err := UUIDParam(r, "id")
		if err != nil {
			Error(w, r, err)
			return
		}

		actor := ActorFrom(r.Context())
		if err := h.catalog.SetFavorite(r.Context(), actor.UserID, carID, favorite); err != nil {
			Error(w, r, err)
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"is_favorite": favorite})
	}
}

// isUploadedPath проверяет, что путь выдан сервисом загрузки файлов.
func isUploadedPath(path string) bool {
	const prefix = "/uploads/"
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return false
	}
	// Обход каталога и абсолютные адреса недопустимы.
	for i := 0; i+1 < len(path); i++ {
		if path[i] == '.' && path[i+1] == '.' {
			return false
		}
	}
	return len(path) < 300
}

func requestMeta(r *http.Request) service.RequestMeta {
	return service.RequestMeta{
		IP:        ClientIPString(r.Context()),
		UserAgent: r.UserAgent(),
		RequestID: RequestIDFrom(r.Context()),
	}
}
