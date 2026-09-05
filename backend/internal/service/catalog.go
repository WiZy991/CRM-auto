package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/store"
)

// Catalog — сценарии каталога автомобилей.
type Catalog struct {
	cars      *store.Cars
	converter *money.Converter
	audit     *store.SecurityLog
	social    *Social
	log       *slog.Logger
}

func NewCatalog(cars *store.Cars, converter *money.Converter, audit *store.SecurityLog, log *slog.Logger) *Catalog {
	return &Catalog{cars: cars, converter: converter, audit: audit, log: log}
}

// SetSocial подключает очередь автопостинга после сборки сервисов.
func (c *Catalog) SetSocial(social *Social) {
	c.social = social
}

// List возвращает страницу каталога.
func (c *Catalog) List(ctx context.Context, filter store.CarFilter, viewerID uuid.UUID, withTotal bool) (*store.CarPage, error) {
	// Публичная выдача жёстко ограничена статусами независимо от того, что
	// пришло в запросе: иначе достаточно передать status=draft, чтобы
	// прочитать чужие неопубликованные объявления.
	filter.Statuses = nil
	filter.DealerID = nil

	page, err := c.cars.List(ctx, filter, viewerID, withTotal)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return page, nil
}

// ListForDealer возвращает объявления дилера, включая черновики.
func (c *Catalog) ListForDealer(ctx context.Context, dealerID uuid.UUID, filter store.CarFilter) (*store.CarPage, error) {
	filter.DealerID = &dealerID
	if len(filter.Statuses) == 0 {
		filter.Statuses = []string{
			string(domain.CarDraft), string(domain.CarModeration), string(domain.CarActive),
			string(domain.CarReserved), string(domain.CarSold), string(domain.CarArchived),
		}
	}

	page, err := c.cars.List(ctx, filter, dealerID, true)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return page, nil
}

// ListForAdmin возвращает объявления по статусам, включая очередь модерации.
//
// Публичный List намеренно обнуляет статусы. Админская выборка — единственное
// место, где moderation/draft видны не владельцу.
func (c *Catalog) ListForAdmin(ctx context.Context, filter store.CarFilter) (*store.CarPage, error) {
	if len(filter.Statuses) == 0 {
		filter.Statuses = []string{string(domain.CarModeration)}
	}
	page, err := c.cars.List(ctx, filter, uuid.Nil, true)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return page, nil
}

// CarView — объявление, подготовленное к отдаче наружу.
type CarView struct {
	Car *domain.Car
	// VIN уже приведён к правам запрашивающего.
	VIN string
	// CanEdit сообщает интерфейсу, показывать ли кнопки управления.
	CanEdit bool
}

// Get возвращает карточку автомобиля.
func (c *Catalog) Get(ctx context.Context, carID uuid.UUID, viewer Viewer) (*CarView, error) {
	car, err := c.cars.ByID(ctx, carID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Объявление")
		}
		return nil, apierr.Internal(err)
	}

	canEdit := viewer.Role == domain.RoleAdmin || car.OwnedBy(viewer.UserID)

	// Неопубликованное объявление видит только его владелец и админ.
	// Для остальных ответ такой же, как для несуществующего: иначе по коду
	// ответа можно перечислить чужие черновики.
	if !car.Status.PubliclyVisible() && !canEdit {
		return nil, apierr.NotFound("Объявление")
	}

	if car.Status.PubliclyVisible() && !canEdit {
		c.cars.IncrementViews(ctx, carID)
	}

	return &CarView{
		Car:     car,
		VIN:     car.VINFor(canEdit),
		CanEdit: canEdit,
	}, nil
}

// Viewer — сведения о запрашивающем для проверки прав.
type Viewer struct {
	UserID uuid.UUID
	Role   domain.Role
}

// Brands возвращает марки для фильтра каталога.
func (c *Catalog) Brands(ctx context.Context, origins []string) ([]store.BrandFacet, error) {
	brands, err := c.cars.Brands(ctx, origins)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return brands, nil
}

// --- Создание и изменение ---------------------------------------------------

// CarForm — данные объявления, приходящие от дилера.
type CarForm struct {
	Origin string

	Brand      string
	Model      string
	Generation string
	TrimLevel  string
	Year       int

	MileageKM int
	EngineCC  *int
	PowerHP   *int

	Fuel          string
	Gearbox       string
	Drive         string
	Body          string
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
	Currency        string
	TurnkeyRubMinor *int64
	CustomsRubMinor *int64
	DeliveryDays    *int

	Title       string
	Description string
	Equipment   []string

	Photos []domain.CarPhoto
}

// Create создаёт объявление дилера.
func (c *Catalog) Create(ctx context.Context, dealerID uuid.UUID, form CarForm, meta RequestMeta) (*domain.Car, error) {
	input, err := c.toInput(form)
	if err != nil {
		return nil, err
	}
	input.DealerID = &dealerID

	car, err := c.cars.Create(ctx, input)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.Internal(err)
		}
		// Нарушение уникальности VIN — ошибка данных пользователя,
		// а не сбой сервера.
		if strings.Contains(err.Error(), "VIN") {
			return nil, apierr.Conflict(err.Error())
		}
		return nil, apierr.Internal(err)
	}

	if len(form.Photos) > 0 {
		if err := c.cars.ReplacePhotos(ctx, car.ID, dealerID, form.Photos); err != nil {
			c.log.ErrorContext(ctx, "не удалось сохранить фотографии объявления",
				slog.String("error", err.Error()))
		}
		car.Photos = form.Photos
	}

	c.recordAudit(ctx, store.AuditEntry{
		ActorID: &dealerID, ActorRole: string(domain.RoleDealer),
		Action: "car.create", Entity: "cars", EntityID: car.ID.String(),
		Diff: map[string]any{"brand": car.Brand, "model": car.Model, "price_rub_minor": car.PriceRubMinor},
		IP:   meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID,
	})

	return car, nil
}

// Update изменяет объявление дилера.
func (c *Catalog) Update(ctx context.Context, carID, dealerID uuid.UUID, form CarForm, meta RequestMeta) (*domain.Car, error) {
	input, err := c.toInput(form)
	if err != nil {
		return nil, err
	}

	car, err := c.cars.Update(ctx, carID, dealerID, input)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Объявление либо не существует, либо принадлежит другому
			// дилеру. Оба случая для клиента выглядят одинаково.
			return nil, apierr.NotFound("Объявление")
		}
		if strings.Contains(err.Error(), "VIN") {
			return nil, apierr.Conflict(err.Error())
		}
		return nil, apierr.Internal(err)
	}

	if form.Photos != nil {
		if err := c.cars.ReplacePhotos(ctx, car.ID, dealerID, form.Photos); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, apierr.NotFound("Объявление")
			}
			return nil, apierr.Internal(err)
		}
		car.Photos = form.Photos
	}

	c.recordAudit(ctx, store.AuditEntry{
		ActorID: &dealerID, ActorRole: string(domain.RoleDealer),
		Action: "car.update", Entity: "cars", EntityID: car.ID.String(),
		IP: meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID,
	})

	return car, nil
}

// ChangeStatus меняет статус объявления с проверкой правил перехода.
func (c *Catalog) ChangeStatus(
	ctx context.Context,
	carID uuid.UUID,
	viewer Viewer,
	target domain.CarStatus,
	meta RequestMeta,
) error {
	car, err := c.cars.ByID(ctx, carID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Объявление")
		}
		return apierr.Internal(err)
	}

	isOwner := car.OwnedBy(viewer.UserID)
	if !isOwner && viewer.Role != domain.RoleAdmin {
		return apierr.NotFound("Объявление")
	}

	if err := domain.ValidateCarStatusTransition(car.Status, target); err != nil {
		return apierr.Conflict(err.Error())
	}

	// Полноту карточки проверяем, когда дилер отправляет лот на проверку.
	// Администратор уже смотрит очередь глазами: отказ по числу фото
	// после ручного «Одобрить» только прячет причину в общий тост.
	if target == domain.CarModeration || (target == domain.CarActive && viewer.Role != domain.RoleAdmin) {
		if err := car.CanBePublished(); err != nil {
			return apierr.Validation(map[string]string{"car": err.Error()})
		}
	}

	var dealerScope *uuid.UUID
	if viewer.Role != domain.RoleAdmin {
		dealerScope = &viewer.UserID
	}

	if err := c.cars.SetStatus(ctx, carID, dealerScope, target); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Объявление")
		}
		return apierr.Internal(err)
	}

	c.recordAudit(ctx, store.AuditEntry{
		ActorID: &viewer.UserID, ActorRole: viewer.Role.String(),
		Action: "car.status", Entity: "cars", EntityID: carID.String(),
		Diff: map[string]any{"from": string(car.Status), "to": string(target)},
		IP:   meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID,
	})

	if target == domain.CarActive && car.Status != domain.CarActive && c.social != nil && car.DealerID != nil {
		if err := c.social.EnqueuePublishedCar(ctx, *car.DealerID, carID); err != nil {
			c.log.Error("не удалось поставить лот в очередь каналов",
				slog.String("car_id", carID.String()),
				slog.String("error", err.Error()))
		}
	}
	return nil
}

// Delete архивирует объявление дилера.
func (c *Catalog) Delete(ctx context.Context, carID, dealerID uuid.UUID, meta RequestMeta) error {
	if err := c.cars.Delete(ctx, carID, dealerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Объявление")
		}
		return apierr.Internal(err)
	}

	c.recordAudit(ctx, store.AuditEntry{
		ActorID: &dealerID, ActorRole: string(domain.RoleDealer),
		Action: "car.archive", Entity: "cars", EntityID: carID.String(),
		IP: meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID,
	})
	return nil
}

// --- Избранное --------------------------------------------------------------

// SetFavorite добавляет или убирает объявление из избранного.
func (c *Catalog) SetFavorite(ctx context.Context, userID, carID uuid.UUID, favorite bool) error {
	// Проверка существования и видимости объявления обязательна: иначе
	// избранное превращается в способ проверить, существует ли черновик
	// с заданным идентификатором.
	car, err := c.cars.ByID(ctx, carID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Объявление")
		}
		return apierr.Internal(err)
	}
	if !car.Status.PubliclyVisible() {
		return apierr.NotFound("Объявление")
	}

	if favorite {
		if err := c.cars.AddFavorite(ctx, userID, carID); err != nil {
			return apierr.Internal(err)
		}
		return nil
	}
	if err := c.cars.RemoveFavorite(ctx, userID, carID); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// --- Проверка данных --------------------------------------------------------

// toInput проверяет форму и переводит её в параметры хранилища.
//
// Валидация собрана в одном месте и возвращает все ошибки сразу: заполнение
// объявления — длинная форма, и показывать ошибки по одной за раз означает
// заставлять дилера отправлять её пять раз.
func (c *Catalog) toInput(form CarForm) (store.CarInput, error) {
	problems := map[string]string{}

	origin := domain.Origin(strings.ToLower(strings.TrimSpace(form.Origin)))
	if !origin.Valid() {
		problems["origin"] = "укажите страну: cn или jp"
	}

	fuel := domain.FuelType(strings.ToLower(strings.TrimSpace(form.Fuel)))
	if !fuel.Valid() {
		problems["fuel"] = "недопустимый тип топлива"
	}
	gearbox := domain.Transmission(strings.ToLower(strings.TrimSpace(form.Gearbox)))
	if !gearbox.Valid() {
		problems["gearbox"] = "недопустимый тип трансмиссии"
	}
	drive := domain.Drivetrain(strings.ToLower(strings.TrimSpace(form.Drive)))
	if !drive.Valid() {
		problems["drive"] = "недопустимый тип привода"
	}
	body := domain.BodyType(strings.ToLower(strings.TrimSpace(form.Body)))
	if !body.Valid() {
		problems["body"] = "недопустимый тип кузова"
	}

	currency := domain.Currency(strings.ToLower(strings.TrimSpace(form.Currency)))
	if !currency.Valid() {
		problems["currency"] = "недопустимая валюта"
	}

	brand := cleanText(form.Brand, 60)
	if len([]rune(brand)) < 2 {
		problems["brand"] = "укажите марку автомобиля"
	}
	model := cleanText(form.Model, 60)
	if len([]rune(model)) < 1 {
		problems["model"] = "укажите модель автомобиля"
	}

	title := cleanText(form.Title, 200)
	if title == "" {
		title = cleanText(strings.TrimSpace(brand+" "+model+" "+fmt.Sprintf("%d", form.Year)), 200)
	}
	if len([]rune(title)) < 4 {
		problems["title"] = "заголовок должен содержать не менее четырёх символов"
	}

	description := cleanText(form.Description, 8000)

	currentYear := time.Now().Year()
	if form.Year < 1980 || form.Year > currentYear+1 {
		problems["year"] = fmt.Sprintf("год выпуска должен быть между 1980 и %d", currentYear+1)
	}
	if form.MileageKM < 0 || form.MileageKM > 2_000_000 {
		problems["mileage_km"] = "пробег указан вне допустимого диапазона"
	}
	if form.PriceMinor <= 0 {
		problems["price_minor"] = "укажите цену больше нуля"
	}

	if form.EngineCC != nil && (*form.EngineCC < 0 || *form.EngineCC > 10_000) {
		problems["engine_cc"] = "объём двигателя указан вне допустимого диапазона"
	}
	if form.PowerHP != nil && (*form.PowerHP < 0 || *form.PowerHP > 2_000) {
		problems["power_hp"] = "мощность указана вне допустимого диапазона"
	}
	if form.Seats != nil && (*form.Seats < 1 || *form.Seats > 30) {
		problems["seats"] = "число мест указано вне допустимого диапазона"
	}
	if form.DeliveryDays != nil && (*form.DeliveryDays < 0 || *form.DeliveryDays > 365) {
		problems["delivery_days"] = "срок доставки указан вне допустимого диапазона"
	}

	vin := strings.ToUpper(strings.TrimSpace(form.VIN))
	if vin != "" {
		if err := validateVIN(vin); err != nil {
			problems["vin"] = err.Error()
		}
	}

	if len(form.Equipment) > 100 {
		problems["equipment"] = "слишком много элементов комплектации"
	}
	equipment := make([]string, 0, len(form.Equipment))
	for _, item := range form.Equipment {
		cleaned := cleanText(item, 100)
		if cleaned != "" {
			equipment = append(equipment, cleaned)
		}
	}

	if len(problems) > 0 {
		return store.CarInput{}, apierr.Validation(problems)
	}

	priceRub, err := c.converter.ToRubMinor(form.PriceMinor, currency)
	if err != nil {
		return store.CarInput{}, apierr.Validation(map[string]string{"currency": err.Error()})
	}

	return store.CarInput{
		Origin:     origin,
		Brand:      brand,
		Model:      model,
		Generation: cleanText(form.Generation, 60),
		TrimLevel:  cleanText(form.TrimLevel, 60),
		Year:       form.Year,

		MileageKM: form.MileageKM,
		EngineCC:  form.EngineCC,
		PowerHP:   form.PowerHP,

		Fuel:          fuel,
		Gearbox:       gearbox,
		Drive:         drive,
		Body:          body,
		Color:         cleanText(form.Color, 40),
		Seats:         form.Seats,
		SteeringRight: form.SteeringRight,

		AuctionGrade:     cleanText(form.AuctionGrade, 10),
		InteriorGrade:    cleanText(form.InteriorGrade, 10),
		AuctionLotNumber: cleanText(form.AuctionLotNumber, 40),
		AuctionDate:      form.AuctionDate,

		VIN:        vin,
		VINVisible: form.VINVisible,

		PriceMinor:      form.PriceMinor,
		Currency:        currency,
		PriceRubMinor:   priceRub,
		TurnkeyRubMinor: form.TurnkeyRubMinor,
		CustomsRubMinor: form.CustomsRubMinor,
		DeliveryDays:    form.DeliveryDays,

		Title:       title,
		Description: description,
		Equipment:   equipment,
	}, nil
}

// validateVIN проверяет формат VIN.
//
// Буквы I, O и Q в VIN не используются по стандарту именно потому, что их
// путают с цифрами 1 и 0. Их наличие означает ошибку ввода, а не редкий
// автомобиль.
func validateVIN(vin string) error {
	if len(vin) < 11 || len(vin) > 17 {
		return errors.New("VIN должен содержать от 11 до 17 символов")
	}
	for _, r := range vin {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z' && r != 'I' && r != 'O' && r != 'Q':
		default:
			return fmt.Errorf("недопустимый символ %q в VIN", r)
		}
	}
	return nil
}

// cleanText приводит пользовательский текст к безопасному виду.
//
// Экранирование HTML здесь не выполняется намеренно: текст хранится и
// отдаётся как обычный текст, а фронтенд выводит его через React, который
// подставляет значения как текстовые узлы. Хранить экранированный HTML в
// базе — источник двойного экранирования и «&amp;amp;» в интерфейсе.
//
// Что действительно нужно убрать — управляющие символы: они не несут смысла,
// ломают вывод в консоли и позволяют подделывать строки в журналах.
func cleanText(raw string, maxRunes int) string {
	var b strings.Builder
	b.Grow(len(raw))

	for _, r := range raw {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r):
			// Пропускаем: возврат каретки, escape-последовательности и
			// прочие управляющие символы.
		case r == '\uFEFF' || r == '\u200B':
			// Невидимые пробелы, которые попадают при копировании из Word.
		default:
			b.WriteRune(r)
		}
	}

	result := strings.TrimSpace(b.String())
	runes := []rune(result)
	if len(runes) > maxRunes {
		result = strings.TrimSpace(string(runes[:maxRunes]))
	}
	return result
}

func (c *Catalog) recordAudit(ctx context.Context, entry store.AuditEntry) {
	if err := c.audit.RecordAudit(context.WithoutCancel(ctx), entry); err != nil {
		c.log.ErrorContext(ctx, "не удалось записать аудит",
			slog.String("action", entry.Action),
			slog.String("error", err.Error()))
	}
}
