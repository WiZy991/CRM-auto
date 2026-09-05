package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// Requests — сценарии работы с заявками клиентов.
type Requests struct {
	requests *store.Requests
	cars     *store.Cars
	notify   *store.Notifications
	audit    *store.SecurityLog
	log      *slog.Logger
}

func NewRequests(
	requests *store.Requests,
	cars *store.Cars,
	notify *store.Notifications,
	audit *store.SecurityLog,
	log *slog.Logger,
) *Requests {
	return &Requests{requests: requests, cars: cars, notify: notify, audit: audit, log: log}
}

// Прикладной предел на число заявок клиента.
//
// Ограничение частоты по адресу тут не помогает: сто заявок с разных
// адресов проходят его свободно, а для дилеров это тот же спам. Порог
// заведомо выше любого разумного поведения — за сутки человек не подбирает
// двадцать разных автомобилей.
const (
	requestsPerDayLimit = 20
	requestsWindow      = 24 * time.Hour
)

// RequestForm — данные заявки, приходящие от клиента.
type RequestForm struct {
	DealerID string
	CarID    string

	DesiredBrand string
	DesiredModel string
	YearFrom     *int
	YearTo       *int
	Origin       string

	BudgetFromRub *int64
	BudgetToRub   *int64

	Body    string
	Gearbox string

	Comment           string
	ContactPreference string
}

// Create создаёт заявку от имени клиента.
func (r *Requests) Create(ctx context.Context, clientID uuid.UUID, form RequestForm) (*domain.Request, error) {
	recent, err := r.requests.CountRecentByClient(ctx, clientID, requestsWindow)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if recent >= requestsPerDayLimit {
		return nil, apierr.TooManyRequests("Слишком много заявок за сутки. Дождитесь ответа по созданным заявкам.")
	}

	params, listingDealer, err := r.buildCreateParams(ctx, clientID, form)
	if err != nil {
		return nil, err
	}

	request, err := r.requests.Create(ctx, *params)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	r.notifyRequestCreated(ctx, request, listingDealer)
	return request, nil
}

func (r *Requests) buildCreateParams(ctx context.Context, clientID uuid.UUID, form RequestForm) (*store.CreateRequestParams, *uuid.UUID, error) {
	params := store.CreateRequestParams{
		ClientID:     clientID,
		DesiredBrand: normalizeShortText(form.DesiredBrand, 80),
		DesiredModel: normalizeShortText(form.DesiredModel, 80),
		Comment:      normalizeShortText(form.Comment, 2000),
	}

	preference, err := domain.ParseContactPreference(form.ContactPreference)
	if err != nil {
		return nil, nil, apierr.BadRequest(err.Error())
	}
	params.ContactPreference = string(preference)

	var listingDealer *uuid.UUID

	if form.CarID != "" {
		carID, err := uuid.Parse(form.CarID)
		if err != nil {
			return nil, nil, apierr.BadRequest("Некорректный идентификатор объявления")
		}

		// Объявление обязано быть доступно публично: иначе через заявку
		// можно подтвердить существование чужого черновика.
		car, err := r.cars.ByID(ctx, carID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, nil, apierr.NotFound("Объявление")
			}
			return nil, nil, apierr.Internal(err)
		}
		if !car.Status.PubliclyVisible() {
			return nil, nil, apierr.NotFound("Объявление")
		}
		if car.OwnedBy(clientID) {
			return nil, nil, apierr.BadRequest("Нельзя оставить заявку на собственное объявление")
		}

		params.CarID = &carID
		listingDealer = car.DealerID
		// Лота недостаточно, чтобы закрыть заявку от остальных дилеров:
		// покупатель ждёт, что любой импортёр увидит её в пуле и возьмёт.
		// Владельца объявления уведомляем отдельно.

		if params.DesiredBrand == "" {
			params.DesiredBrand = car.Brand
			params.DesiredModel = car.Model
		}
	} else if form.DealerID != "" {
		dealerID, err := uuid.Parse(form.DealerID)
		if err != nil {
			return nil, nil, apierr.BadRequest("Некорректный идентификатор дилера")
		}
		if dealerID == clientID {
			return nil, nil, apierr.BadRequest("Нельзя направить заявку самому себе")
		}
		params.DealerID = &dealerID
	}

	if form.Origin != "" {
		origin := domain.Origin(normalizeEnum(form.Origin))
		if !origin.Valid() {
			return nil, nil, apierr.BadRequest("Неизвестная страна происхождения")
		}
		params.Origin = &origin
	}
	if form.Body != "" {
		body := domain.BodyType(normalizeEnum(form.Body))
		if !body.Valid() {
			return nil, nil, apierr.BadRequest("Неизвестный тип кузова")
		}
		params.Body = &body
	}
	if form.Gearbox != "" {
		gearbox := domain.Transmission(normalizeEnum(form.Gearbox))
		if !gearbox.Valid() {
			return nil, nil, apierr.BadRequest("Неизвестный тип коробки передач")
		}
		params.Gearbox = &gearbox
	}

	if err := validateYearRange(form.YearFrom, form.YearTo); err != nil {
		return nil, nil, err
	}
	params.YearFrom = form.YearFrom
	params.YearTo = form.YearTo

	// Бюджет приходит в рублях, а хранится в копейках: единица измерения
	// на границе и внутри разная, и путать их нельзя.
	budgetFrom, budgetTo, err := normalizeBudget(form.BudgetFromRub, form.BudgetToRub)
	if err != nil {
		return nil, nil, err
	}
	params.BudgetFromRubMinor = budgetFrom
	params.BudgetToRubMinor = budgetTo

	// Заявка без единого содержательного поля бесполезна дилеру: он не
	// поймёт, что искать, и такая заявка только засоряет пул.
	if params.DesiredBrand == "" && params.CarID == nil &&
		params.Origin == nil && params.Comment == "" {
		return nil, nil, apierr.BadRequest("Укажите марку, страну или опишите пожеланиями в комментарии")
	}
	return &params, listingDealer, nil
}

func validateYearRange(from, to *int) error {
	currentYear := time.Now().Year()

	for _, year := range []*int{from, to} {
		if year == nil {
			continue
		}
		if *year < 1980 || *year > currentYear+1 {
			return apierr.BadRequest(fmt.Sprintf("Год должен быть между 1980 и %d", currentYear+1))
		}
	}
	if from != nil && to != nil && *from > *to {
		return apierr.BadRequest("Начальный год больше конечного")
	}
	return nil
}

func normalizeBudget(fromRub, toRub *int64) (*int64, *int64, error) {
	const maxBudgetRub = 100_000_000

	convert := func(value *int64) (*int64, error) {
		if value == nil {
			return nil, nil
		}
		if *value < 0 || *value > maxBudgetRub {
			return nil, apierr.BadRequest("Бюджет указан вне допустимых границ")
		}
		minor := *value * 100
		return &minor, nil
	}

	from, err := convert(fromRub)
	if err != nil {
		return nil, nil, err
	}
	to, err := convert(toRub)
	if err != nil {
		return nil, nil, err
	}
	if from != nil && to != nil && *from > *to {
		return nil, nil, apierr.BadRequest("Нижняя граница бюджета больше верхней")
	}
	return from, to, nil
}

func normalizeEnum(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeShortText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) <= limit {
		return trimmed
	}
	return string([]rune(trimmed)[:limit])
}

// ListForClient возвращает заявки клиента.
func (r *Requests) ListForClient(ctx context.Context, clientID uuid.UUID, statuses []string, limit, offset int) ([]store.RequestListItem, int, error) {
	items, total, err := r.requests.List(ctx, store.RequestFilter{
		ClientID: &clientID,
		Statuses: statuses,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}
	return items, total, nil
}

// ListForDealer возвращает заявки, закреплённые за дилером.
func (r *Requests) ListForDealer(ctx context.Context, dealerID uuid.UUID, statuses []string, limit, offset int) ([]store.RequestListItem, int, error) {
	items, total, err := r.requests.List(ctx, store.RequestFilter{
		DealerID: &dealerID,
		Statuses: statuses,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}
	return items, total, nil
}

// ListOpenPool возвращает нераспределённые заявки.
//
// Контакты клиента из этого списка вычищаются: пул видят все проверенные
// дилеры, и телефон должен становиться доступен только после того, как
// дилер взял заявку в работу и принял на себя ответственность за неё.
func (r *Requests) ListOpenPool(ctx context.Context, limit, offset int) ([]store.RequestListItem, int, error) {
	items, total, err := r.requests.List(ctx, store.RequestFilter{
		OpenPool: true,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}

	for index := range items {
		items[index].ClientPhone = ""
		items[index].ClientName = maskPersonalName(items[index].ClientName)
	}
	return items, total, nil
}

// maskPersonalName оставляет имя и первую букву фамилии.
func maskPersonalName(fullName string) string {
	parts := strings.Fields(fullName)
	switch len(parts) {
	case 0:
		return "Клиент"
	case 1:
		return parts[0]
	default:
		return parts[0] + " " + string([]rune(parts[1])[:1]) + "."
	}
}

// Get возвращает заявку участнику.
func (r *Requests) Get(ctx context.Context, requestID uuid.UUID, viewer Viewer) (*domain.Request, error) {
	request, err := r.requests.ByIDForParticipant(
		ctx, requestID, viewer.UserID,
		viewer.Role == domain.RoleAdmin,
		viewer.Role == domain.RoleDealer)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Заявка")
		}
		return nil, apierr.Internal(err)
	}
	return request, nil
}

// Claim закрепляет заявку из общего пула за дилером.
func (r *Requests) Claim(ctx context.Context, requestID, dealerID uuid.UUID) (*domain.Request, error) {
	request, err := r.requests.Claim(ctx, requestID, dealerID)
	if err != nil {
		if errors.Is(err, store.ErrAlreadyClaimed) {
			return nil, apierr.Conflict("Заявку уже взял другой дилер")
		}
		return nil, apierr.Internal(err)
	}

	r.recordAudit(ctx, dealerID, "request.claim", request.ID.String(), nil)
	return request, nil
}

// Reply сохраняет ответ дилера на заявку.
func (r *Requests) Reply(ctx context.Context, requestID, dealerID uuid.UUID, reply string) (*domain.Request, error) {
	text := normalizeShortText(reply, 4000)
	if text == "" {
		return nil, apierr.BadRequest("Текст ответа не может быть пустым")
	}

	request, err := r.requests.Reply(ctx, requestID, dealerID, text)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Заявка")
		}
		return nil, apierr.Internal(err)
	}

	if err := r.notify.Create(ctx, store.CreateNotificationParams{
		UserID: request.ClientID,
		Kind:   store.NotifyRequestAnswered,
		Title:  "Дилер ответил на вашу заявку",
		Body:   normalizeShortText(text, 200),
		Link:   fmt.Sprintf("/cabinet/requests/%s", request.ID),
	}); err != nil {
		r.log.ErrorContext(ctx, "не удалось создать уведомление об ответе", "error", err)
	}
	return request, nil
}

// Reject отклоняет заявку дилером.
func (r *Requests) Reject(ctx context.Context, requestID, dealerID uuid.UUID, reason string) (*domain.Request, error) {
	text := normalizeShortText(reason, 500)
	if text == "" {
		return nil, apierr.BadRequest("Укажите причину отказа")
	}

	request, err := r.requests.Reject(ctx, requestID, dealerID, text)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Заявка")
		}
		return nil, apierr.Internal(err)
	}

	if err := r.notify.Create(ctx, store.CreateNotificationParams{
		UserID: request.ClientID,
		Kind:   store.NotifyRequestAnswered,
		Title:  "Заявка отклонена",
		Body:   text,
		Link:   fmt.Sprintf("/cabinet/requests/%s", request.ID),
	}); err != nil {
		r.log.ErrorContext(ctx, "не удалось создать уведомление об отказе", "error", err)
	}

	r.recordAudit(ctx, dealerID, "request.reject", request.ID.String(),
		map[string]any{"reason": text})
	return request, nil
}

// Close закрывает заявку по инициативе клиента.
func (r *Requests) Close(ctx context.Context, requestID, clientID uuid.UUID) error {
	if err := r.requests.Close(ctx, requestID, clientID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Заявка")
		}
		return apierr.Internal(err)
	}
	return nil
}

func (r *Requests) notifyRequestCreated(ctx context.Context, request *domain.Request, listingDealer *uuid.UUID) {
	target := request.DealerID
	if target == nil {
		target = listingDealer
	}
	if target == nil {
		return
	}

	if err := r.notify.Create(ctx, store.CreateNotificationParams{
		UserID: *request.DealerID,
		Kind:   store.NotifyRequestCreated,
		Title:  "Новая заявка",
		Body:   request.Summary(),
		Link:   fmt.Sprintf("/dealer/requests/%s", request.ID),
		Payload: map[string]any{
			"request_id": request.ID.String(),
			"number":     request.PublicNumber,
		},
	}); err != nil {
		// Заявка уже создана, и откатывать её из-за уведомления нельзя:
		// клиент своё действие выполнил, а дилер увидит заявку в списке.
		r.log.ErrorContext(ctx, "не удалось создать уведомление о заявке", "error", err)
	}
}

func (r *Requests) recordAudit(ctx context.Context, actorID uuid.UUID, action, entityID string, diff map[string]any) {
	if err := r.audit.RecordAudit(ctx, store.AuditEntry{
		ActorID:  &actorID,
		Action:   action,
		Entity:   "request",
		EntityID: entityID,
		Diff:     diff,
	}); err != nil {
		r.log.ErrorContext(ctx, "не удалось записать аудит заявки", "error", err, "action", action)
	}
}
