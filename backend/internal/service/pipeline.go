package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/pkg/storage"
	"github.com/autoimport/crm/internal/store"
)

// Pipeline — сценарии воронки: сделки, этапы, переписка, задачи, документы.
type Pipeline struct {
	deals     *store.Deals
	requests  *store.Requests
	comms     *store.DealComms
	reviews   *store.Reviews
	notify    *store.Notifications
	converter *money.Converter
	audit     *store.SecurityLog
	log       *slog.Logger

	users   *store.Users
	dealers *store.Dealers
	cars    *store.Cars
	sellers *store.Sellers
	cipher  *security.Cipher
	disk    *storage.Disk
}

// PipelineExtras — источники для автозаполнения печатных форм.
type PipelineExtras struct {
	Users   *store.Users
	Dealers *store.Dealers
	Cars    *store.Cars
	Sellers *store.Sellers
	Cipher  *security.Cipher
	Disk    *storage.Disk
}

func NewPipeline(
	deals *store.Deals,
	requests *store.Requests,
	comms *store.DealComms,
	reviews *store.Reviews,
	notify *store.Notifications,
	converter *money.Converter,
	audit *store.SecurityLog,
	log *slog.Logger,
	extras PipelineExtras,
) *Pipeline {
	return &Pipeline{
		deals: deals, requests: requests, comms: comms, reviews: reviews,
		notify: notify, converter: converter, audit: audit, log: log,
		users: extras.Users, dealers: extras.Dealers, cars: extras.Cars,
		sellers: extras.Sellers, cipher: extras.Cipher, disk: extras.Disk,
	}
}

// --- Доступ -----------------------------------------------------------------

// DealAccess — права запрашивающего на сделку.
type DealAccess struct {
	Deal *domain.Deal

	IsDealer bool
	IsClient bool
	IsAdmin  bool
}

// CanManage сообщает, может ли запрашивающий менять сделку.
//
// Клиент видит сделку, но не управляет ею: этапы, суммы и документы —
// зона ответственности дилера.
func (a DealAccess) CanManage() bool { return a.IsDealer || a.IsAdmin }

// access загружает сделку и определяет права.
//
// Единая точка проверки: каждый обработчик воронки начинается с неё, и
// забыть проверку в отдельном методе невозможно — без DealAccess до сделки
// просто не добраться.
func (p *Pipeline) access(ctx context.Context, dealID uuid.UUID, viewer Viewer) (*DealAccess, error) {
	isAdmin := viewer.Role == domain.RoleAdmin

	deal, err := p.deals.ByIDForParticipant(ctx, dealID, viewer.UserID, isAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		return nil, apierr.Internal(err)
	}

	return &DealAccess{
		Deal:     deal,
		IsDealer: deal.DealerID == viewer.UserID,
		IsClient: deal.ClientID == viewer.UserID,
		IsAdmin:  isAdmin,
	}, nil
}

// requireManage загружает сделку и требует прав на изменение.
func (p *Pipeline) requireManage(ctx context.Context, dealID uuid.UUID, viewer Viewer) (*DealAccess, error) {
	access, err := p.access(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}
	if !access.CanManage() {
		return nil, apierr.Forbidden("Изменять сделку может только дилер")
	}
	return access, nil
}

// --- Создание и чтение ------------------------------------------------------

// CreateDealForm — данные новой сделки.
type CreateDealForm struct {
	RequestID string
	ClientID  string
	CarID     string
	SellerID  string

	ClientName  string
	ClientEmail string
	ClientPhone string

	Title       string
	AmountMinor *int64
	Currency    string
	Stage       string
}

// CreateDeal создаёт сделку от имени дилера.
func (p *Pipeline) CreateDeal(ctx context.Context, dealerID uuid.UUID, form CreateDealForm) (*domain.Deal, error) {
	params := store.CreateDealParams{
		DealerID: dealerID,
		Title:    normalizeShortText(form.Title, 200),
	}
	if params.Title == "" {
		return nil, apierr.Validation(map[string]string{"title": "укажите название сделки"})
	}

	switch {
	case form.RequestID != "":
		requestID, err := uuid.Parse(form.RequestID)
		if err != nil {
			return nil, apierr.BadRequest("Некорректный идентификатор заявки")
		}

		request, err := p.requests.ByIDForParticipant(ctx, requestID, dealerID, false, true)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, apierr.NotFound("Заявка")
			}
			return nil, apierr.Internal(err)
		}
		// Заявка обязана быть закреплена именно за этим дилером: иначе
		// сделку можно создать по чужой заявке из общего пула, минуя
		// закрепление и уведомление клиента.
		if request.DealerID == nil || *request.DealerID != dealerID {
			return nil, apierr.Forbidden("Сначала возьмите заявку в работу")
		}
		if err := request.CanConvertToDeal(); err != nil {
			return nil, apierr.Conflict(err.Error())
		}

		params.RequestID = &requestID
		params.ClientID = request.ClientID
		params.CarID = request.CarID

	default:
		clientID, err := p.resolveDealClient(ctx, dealerID, form)
		if err != nil {
			return nil, err
		}
		params.ClientID = clientID
	}

	if form.CarID != "" {
		carID, err := uuid.Parse(form.CarID)
		if err != nil {
			return nil, apierr.BadRequest("Некорректный идентификатор объявления")
		}
		params.CarID = &carID
	}
	if form.SellerID != "" {
		sellerID, err := uuid.Parse(form.SellerID)
		if err != nil {
			return nil, apierr.BadRequest("Некорректный идентификатор продавца")
		}
		params.SellerID = &sellerID
	}

	if form.Stage != "" {
		stage := domain.Stage(normalizeEnum(form.Stage))
		if !stage.Valid() {
			return nil, apierr.Validation(map[string]string{"stage": "неизвестный этап воронки"})
		}
		params.Stage = stage
	}

	currency := domain.Currency(normalizeEnum(form.Currency))
	if currency == "" {
		currency = domain.CurrencyRUB
	}
	if !currency.Valid() {
		return nil, apierr.BadRequest("Неизвестная валюта")
	}
	params.Currency = currency

	if form.AmountMinor != nil {
		if *form.AmountMinor < 0 {
			return nil, apierr.BadRequest("Сумма не может быть отрицательной")
		}
		params.AmountMinor = form.AmountMinor

		// Рублёвый эквивалент фиксируется на момент создания: курс меняется,
		// а аналитика воронки должна опираться на сумму, которую дилер
		// видел, когда договаривался с клиентом.
		rubMinor, err := p.converter.ToRubMinor(*form.AmountMinor, currency)
		if err != nil {
			return nil, apierr.BadRequest(err.Error())
		}
		params.AmountRubMinor = &rubMinor
	}

	deal, err := p.deals.Create(ctx, params)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	notifyTitle := "Дилер открыл сделку"
	if params.RequestID != nil {
		notifyTitle = "Открыта сделка по вашей заявке"
	}
	if err := p.notify.Create(ctx, store.CreateNotificationParams{
		UserID: deal.ClientID,
		Kind:   store.NotifyDealCreated,
		Title:  notifyTitle,
		Body:   deal.Title,
		Link:   fmt.Sprintf("/app/deals/%s", deal.ID),
	}); err != nil {
		p.log.ErrorContext(ctx, "не удалось уведомить клиента о сделке", "error", err)
	}

	p.recordAudit(ctx, dealerID, "deal.create", deal.ID.String(),
		map[string]any{"title": deal.Title, "number": deal.PublicNumber})
	return deal, nil
}

// resolveDealClient находит существующего покупателя или заводит карточку
// по имени и телефону, как контакт в классической CRM.
func (p *Pipeline) resolveDealClient(ctx context.Context, dealerID uuid.UUID, form CreateDealForm) (uuid.UUID, error) {
	if p.users == nil {
		return uuid.Nil, apierr.Internal(fmt.Errorf("каталог пользователей недоступен"))
	}

	if form.ClientID != "" {
		clientID, err := uuid.Parse(form.ClientID)
		if err != nil {
			return uuid.Nil, apierr.Validation(map[string]string{"client_id": "некорректный идентификатор"})
		}
		if clientID == dealerID {
			return uuid.Nil, apierr.Validation(map[string]string{"client_id": "нельзя создать сделку с самим собой"})
		}
		rec, err := p.users.ByID(ctx, clientID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return uuid.Nil, apierr.NotFound("Клиент")
			}
			return uuid.Nil, apierr.Internal(err)
		}
		if rec.Role != domain.RoleClient {
			return uuid.Nil, apierr.Validation(map[string]string{"client_id": "сделку можно открыть только с покупателем"})
		}
		return rec.ID, nil
	}

	name := normalizeShortText(form.ClientName, 120)
	email := normalizeEmail(form.ClientEmail)
	phoneRaw := strings.TrimSpace(form.ClientPhone)

	problems := map[string]string{}
	if name == "" {
		problems["client_name"] = "укажите имя клиента"
	}
	var phone string
	if phoneRaw == "" {
		problems["client_phone"] = "укажите телефон клиента"
	} else {
		normalized, err := normalizePhone(phoneRaw)
		if err != nil {
			problems["client_phone"] = err.Error()
		} else {
			phone = normalized
		}
	}
	if len(problems) > 0 {
		return uuid.Nil, apierr.Validation(problems)
	}

	var byEmail, byPhone *store.UserRecord
	if email != "" {
		found, err := p.users.ByEmail(ctx, email)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return uuid.Nil, apierr.Internal(err)
		}
		if err == nil {
			byEmail = found
		}
	}
	foundPhone, err := p.users.ByPhone(ctx, phone)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return uuid.Nil, apierr.Internal(err)
	}
	if err == nil {
		byPhone = foundPhone
	}

	if byEmail != nil && byPhone != nil && byEmail.ID != byPhone.ID {
		return uuid.Nil, apierr.Validation(map[string]string{
			"client_email": "эта почта принадлежит другому человеку, чем телефон",
		})
	}

	existing := byEmail
	if existing == nil {
		existing = byPhone
	}
	if existing != nil {
		if existing.Role != domain.RoleClient {
			return uuid.Nil, apierr.Validation(map[string]string{
				"client_phone": "этот контакт уже занят другой ролью",
			})
		}
		if existing.ID == dealerID {
			return uuid.Nil, apierr.Validation(map[string]string{"client_phone": "нельзя создать сделку с самим собой"})
		}
		return existing.ID, nil
	}

	if email == "" {
		token := make([]byte, 6)
		if _, err := rand.Read(token); err != nil {
			return uuid.Nil, apierr.Internal(err)
		}
		email = fmt.Sprintf("walkin.%s@crm.local", hex.EncodeToString(token))
	}

	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return uuid.Nil, apierr.Internal(err)
	}
	hash, err := security.NewHasher(19*1024, 2, 1).Hash(hex.EncodeToString(secret))
	if err != nil {
		return uuid.Nil, apierr.Internal(err)
	}

	created, err := p.users.Create(ctx, store.CreateUserParams{
		Role:            domain.RoleClient,
		Email:           email,
		Phone:           phone,
		PasswordHash:    hash,
		FullName:        name,
		ImmediateActive: true,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrEmailTaken):
			return uuid.Nil, apierr.Validation(map[string]string{"client_email": "эта почта уже зарегистрирована"})
		case errors.Is(err, store.ErrPhoneTaken):
			return uuid.Nil, apierr.Validation(map[string]string{"client_phone": "этот телефон уже зарегистрирован"})
		default:
			return uuid.Nil, apierr.Internal(err)
		}
	}
	return created.ID, nil
}

// SearchClients подсказывает покупателей при создании сделки вручную.
func (p *Pipeline) SearchClients(ctx context.Context, viewer Viewer, query string) ([]store.ClientLookup, error) {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Поиск клиентов доступен дилеру")
	}
	if p.users == nil {
		return nil, apierr.Internal(fmt.Errorf("каталог пользователей недоступен"))
	}

	query = strings.TrimSpace(query)
	if query == "" {
		items, err := p.users.RecentClientsForDealer(ctx, viewer.UserID, 15)
		if err != nil {
			return nil, apierr.Internal(err)
		}
		return items, nil
	}

	items, err := p.users.SearchClients(ctx, query, 15)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

// DealFilterForm — фильтр списка сделок.
type DealFilterForm struct {
	Stages    []string
	Outcomes  []string
	Search    string
	StaleOnly bool
	Limit     int
	Offset    int
}

// ListDeals возвращает сделки в зависимости от роли запрашивающего.
func (p *Pipeline) ListDeals(ctx context.Context, viewer Viewer, form DealFilterForm) ([]store.DealListItem, int, error) {
	filter := store.DealFilter{
		Stages:    form.Stages,
		Outcomes:  form.Outcomes,
		Search:    normalizeShortText(form.Search, 100),
		StaleOnly: form.StaleOnly,
		Limit:     form.Limit,
		Offset:    form.Offset,
	}

	// Ограничение по участнику ставится по роли, а не по параметру запроса:
	// подставить чужой идентификатор в фильтр невозможно.
	switch viewer.Role {
	case domain.RoleDealer:
		filter.DealerID = &viewer.UserID
	case domain.RoleClient:
		filter.ClientID = &viewer.UserID
	case domain.RoleAdmin:
		filter.AdminAll = true
	default:
		return nil, 0, apierr.Forbidden("Роль не имеет доступа к сделкам")
	}

	items, total, err := p.deals.List(ctx, filter, viewer.UserID)
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}
	return items, total, nil
}

// DealDetails — карточка сделки со всем содержимым.
type DealDetails struct {
	Deal      *domain.Deal
	Access    DealAccess
	History   []store.StageHistoryEntry
	Tasks     []store.Task
	Documents []store.Document
	Review    *store.Review
	// NextStages — переходы, доступные из текущего этапа.
	NextStages []domain.Stage
}

// DealDetails собирает карточку сделки.
func (p *Pipeline) DealDetails(ctx context.Context, dealID uuid.UUID, viewer Viewer) (*DealDetails, error) {
	access, err := p.access(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}

	history, err := p.deals.StageHistory(ctx, dealID)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	tasks, err := p.comms.Tasks(ctx, dealID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	// Задачи — внутренний инструмент дилера, клиенту они не показываются.
	if !access.CanManage() {
		tasks = nil
	}

	documents, err := p.comms.Documents(ctx, dealID, !access.CanManage())
	if err != nil {
		return nil, apierr.Internal(err)
	}

	var review *store.Review
	if p.reviews != nil {
		loaded, loadErr := p.reviews.ByDeal(ctx, dealID)
		if loadErr != nil {
			return nil, apierr.Internal(loadErr)
		}
		review = loaded
	}

	details := &DealDetails{
		Deal:      access.Deal,
		Access:    *access,
		History:   history,
		Tasks:     tasks,
		Documents: documents,
		Review:    review,
	}
	if access.CanManage() {
		details.NextStages = domain.AllowedNextStages(access.Deal.Stage, access.Deal.Outcome)
	}
	return details, nil
}

// --- Этапы ------------------------------------------------------------------

// ChangeStage переводит сделку на другой этап.
func (p *Pipeline) ChangeStage(ctx context.Context, dealID uuid.UUID, viewer Viewer, toStage, comment string) (*domain.Deal, error) {
	access, err := p.requireManage(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}

	stage := domain.Stage(normalizeEnum(toStage))
	if !stage.Valid() {
		return nil, apierr.BadRequest("Неизвестный этап сделки")
	}

	fromStage := access.Deal.Stage

	deal, err := p.deals.ChangeStage(ctx, store.ChangeStageParams{
		DealID:    dealID,
		DealerID:  viewer.UserID,
		IsAdmin:   viewer.Role == domain.RoleAdmin,
		ToStage:   stage,
		Comment:   normalizeShortText(comment, 1000),
		ChangedBy: viewer.UserID,
	})
	if err != nil {
		return nil, p.mapStageError(err)
	}

	p.announceStageChange(ctx, deal, fromStage, viewer.UserID)
	p.recordAudit(ctx, viewer.UserID, "deal.stage", dealID.String(),
		map[string]any{"from": string(fromStage), "to": string(stage)})
	return deal, nil
}

// mapStageError переводит ошибку смены этапа в ответ API.
//
// Нарушение правил перехода — это ошибка пользователя, а не сбой сервера:
// отвечать на неё пятисоткой значит прятать понятную причину.
func (p *Pipeline) mapStageError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return apierr.NotFound("Сделка")
	case errors.As(err, new(*domain.StageTransitionError)):
		return apierr.Conflict(err.Error())
	default:
		return apierr.Internal(err)
	}
}

func (p *Pipeline) announceStageChange(ctx context.Context, deal *domain.Deal, from domain.Stage, actorID uuid.UUID) {
	body := fmt.Sprintf("Сделка «%s» перешла на этап «%s»", deal.Title, deal.Stage.Title())

	if err := p.comms.AddSystemMessage(ctx, deal.ID, actorID, body); err != nil {
		p.log.ErrorContext(ctx, "не удалось записать системное сообщение", "error", err)
	}

	if err := p.notify.Create(ctx, store.CreateNotificationParams{
		UserID: deal.ClientID,
		Kind:   store.NotifyDealStageChanged,
		Title:  "Изменился этап сделки",
		Body:   body,
		Link:   fmt.Sprintf("/cabinet/deals/%s", deal.ID),
		Payload: map[string]any{
			"deal_id": deal.ID.String(),
			"from":    string(from),
			"to":      string(deal.Stage),
		},
	}); err != nil {
		p.log.ErrorContext(ctx, "не удалось уведомить об этапе", "error", err)
	}
}

// CloseDeal закрывает сделку с указанным исходом.
func (p *Pipeline) CloseDeal(ctx context.Context, dealID uuid.UUID, viewer Viewer, outcome, reason string) (*domain.Deal, error) {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return nil, err
	}

	result := domain.Outcome(normalizeEnum(outcome))
	if result != domain.OutcomeWon && result != domain.OutcomeLost {
		return nil, apierr.BadRequest("Исход сделки должен быть «успешно» или «отказ»")
	}

	deal, err := p.deals.Close(ctx, store.CloseParams{
		DealID:     dealID,
		DealerID:   viewer.UserID,
		IsAdmin:    viewer.Role == domain.RoleAdmin,
		Outcome:    result,
		LostReason: normalizeShortText(reason, 1000),
		ChangedBy:  viewer.UserID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		// Правила закрытия проверяет хранилище внутри транзакции: только
		// там состояние сделки заблокировано и гарантированно актуально.
		return nil, apierr.Conflict(err.Error())
	}

	body := "Сделка завершена"
	if result == domain.OutcomeLost {
		body = "Сделка закрыта без покупки"
	}
	if err := p.notify.Create(ctx, store.CreateNotificationParams{
		UserID: deal.ClientID,
		Kind:   store.NotifyDealStageChanged,
		Title:  body,
		Body:   deal.Title,
		Link:   fmt.Sprintf("/cabinet/deals/%s", deal.ID),
	}); err != nil {
		p.log.ErrorContext(ctx, "не удалось уведомить о закрытии сделки", "error", err)
	}

	p.recordAudit(ctx, viewer.UserID, "deal.close", dealID.String(),
		map[string]any{"outcome": string(result)})
	return deal, nil
}

// UpdateDealForm — изменяемые реквизиты сделки.
type UpdateDealForm struct {
	Title              string
	AmountMinor        *int64
	Currency           string
	PaidRubMinor       *int64
	ExpectedHandoverAt *time.Time
	ManagerNote        *string
	SellerID           string
}

// UpdateDeal меняет реквизиты сделки.
func (p *Pipeline) UpdateDeal(ctx context.Context, dealID uuid.UUID, viewer Viewer, form UpdateDealForm) (*domain.Deal, error) {
	access, err := p.requireManage(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}

	params := store.UpdateDealParams{
		DealID:             dealID,
		DealerID:           viewer.UserID,
		IsAdmin:            viewer.Role == domain.RoleAdmin,
		Title:              normalizeShortText(form.Title, 200),
		AmountMinor:        form.AmountMinor,
		PaidRubMinor:       form.PaidRubMinor,
		ExpectedHandoverAt: form.ExpectedHandoverAt,
	}

	if form.ManagerNote != nil {
		note := normalizeShortText(*form.ManagerNote, 4000)
		params.ManagerNote = &note
	}
	if form.SellerID != "" {
		sellerID, err := uuid.Parse(form.SellerID)
		if err != nil {
			return nil, apierr.BadRequest("Некорректный идентификатор продавца")
		}
		params.SellerID = &sellerID
	}

	if form.Currency != "" {
		currency := domain.Currency(normalizeEnum(form.Currency))
		if !currency.Valid() {
			return nil, apierr.BadRequest("Неизвестная валюта")
		}
		params.Currency = currency
	}

	// Рублёвый эквивалент пересчитывается при любом изменении суммы или
	// валюты: иначе в базе останется рублёвая сумма от прежней цены, и
	// отчёт по обороту будет расходиться с суммами в карточках.
	if form.AmountMinor != nil {
		currency := params.Currency
		if currency == "" {
			currency = access.Deal.Currency
		}
		rubMinor, err := p.converter.ToRubMinor(*form.AmountMinor, currency)
		if err != nil {
			return nil, apierr.BadRequest(err.Error())
		}
		params.AmountRubMinor = &rubMinor
	}

	deal, err := p.deals.Update(ctx, params)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		return nil, apierr.Internal(err)
	}

	p.recordAudit(ctx, viewer.UserID, "deal.update", dealID.String(), nil)
	return deal, nil
}

// Summary возвращает сводку воронки дилера.
func (p *Pipeline) Summary(ctx context.Context, viewer Viewer) (*store.PipelineSummary, error) {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Аналитика воронки доступна дилерам")
	}

	summary, err := p.deals.Summary(ctx, viewer.UserID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return summary, nil
}

// --- Переписка --------------------------------------------------------------

// Messages возвращает ленту сообщений сделки.
func (p *Pipeline) Messages(ctx context.Context, dealID uuid.UUID, viewer Viewer, beforeID int64, limit int) ([]store.Message, error) {
	if _, err := p.access(ctx, dealID, viewer); err != nil {
		return nil, err
	}

	messages, err := p.comms.Messages(ctx, dealID, beforeID, limit)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	if _, err := p.comms.MarkMessagesRead(ctx, dealID, viewer.UserID); err != nil {
		p.log.ErrorContext(ctx, "не удалось отметить сообщения прочитанными", "error", err)
	}
	return messages, nil
}

// SendMessage добавляет сообщение в ленту сделки.
func (p *Pipeline) SendMessage(ctx context.Context, dealID uuid.UUID, viewer Viewer, body, attachmentURL string) (*store.Message, error) {
	access, err := p.access(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}

	text := normalizeShortText(body, 5000)
	if text == "" {
		return nil, apierr.BadRequest("Сообщение не может быть пустым")
	}

	// Переписка по закрытой сделке запрещена: иначе через закрытые сделки
	// остаётся вечный канал связи, за которым никто не следит.
	if access.Deal.Outcome != domain.OutcomeOpen {
		return nil, apierr.Conflict("Сделка закрыта, переписка недоступна")
	}

	message, err := p.comms.AddMessage(ctx, dealID, viewer.UserID, text, attachmentURL, viewer.Role == domain.RoleAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		return nil, apierr.Internal(err)
	}

	recipient := access.Deal.ClientID
	if access.IsClient {
		recipient = access.Deal.DealerID
	}
	if err := p.notify.Create(ctx, store.CreateNotificationParams{
		UserID: recipient,
		Kind:   store.NotifyDealMessage,
		Title:  "Новое сообщение по сделке",
		Body:   normalizeShortText(text, 200),
		Link:   fmt.Sprintf("/cabinet/deals/%s", dealID),
	}); err != nil {
		p.log.ErrorContext(ctx, "не удалось уведомить о сообщении", "error", err)
	}
	return message, nil
}

// --- Задачи -----------------------------------------------------------------

// TaskForm — данные новой задачи.
type TaskForm struct {
	Title       string
	Description string
	Stage       string
	DueAt       *time.Time
	AssigneeID  string
}

// CreateTask создаёт задачу по сделке.
func (p *Pipeline) CreateTask(ctx context.Context, dealID uuid.UUID, viewer Viewer, form TaskForm) (*store.Task, error) {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return nil, err
	}

	params := store.CreateTaskParams{
		DealID:      dealID,
		Title:       normalizeShortText(form.Title, 200),
		Description: normalizeShortText(form.Description, 2000),
		CreatedBy:   viewer.UserID,
		DueAt:       form.DueAt,
		IsAdmin:     viewer.Role == domain.RoleAdmin,
	}
	if params.Title == "" {
		return nil, apierr.BadRequest("Укажите название задачи")
	}

	if form.Stage != "" {
		stage := domain.Stage(normalizeEnum(form.Stage))
		if !stage.Valid() {
			return nil, apierr.BadRequest("Неизвестный этап сделки")
		}
		params.Stage = &stage
	}

	// Исполнителем по умолчанию становится сам дилер: задача без
	// исполнителя не попадает ни в один список дел и теряется.
	assignee := viewer.UserID
	if form.AssigneeID != "" {
		parsed, err := uuid.Parse(form.AssigneeID)
		if err != nil {
			return nil, apierr.BadRequest("Некорректный идентификатор исполнителя")
		}
		assignee = parsed
	}
	params.AssigneeID = &assignee

	task, err := p.comms.CreateTask(ctx, params)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		return nil, apierr.Internal(err)
	}
	return task, nil
}

// ToggleTask меняет отметку выполнения задачи.
func (p *Pipeline) ToggleTask(ctx context.Context, dealID, taskID uuid.UUID, viewer Viewer, done bool) error {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return err
	}

	if err := p.comms.ToggleTask(ctx, taskID, dealID, viewer.UserID, viewer.Role == domain.RoleAdmin, done); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Задача")
		}
		return apierr.Internal(err)
	}
	return nil
}

// DeleteTask удаляет задачу.
func (p *Pipeline) DeleteTask(ctx context.Context, dealID, taskID uuid.UUID, viewer Viewer) error {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return err
	}

	if err := p.comms.DeleteTask(ctx, taskID, dealID, viewer.UserID, viewer.Role == domain.RoleAdmin); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Задача")
		}
		return apierr.Internal(err)
	}
	return nil
}

// --- Документы --------------------------------------------------------------

// DocumentForm — метаданные загружаемого документа.
type DocumentForm struct {
	Kind            string
	Title           string
	FileURL         string
	MimeType        string
	Bytes           int
	VisibleToClient bool
}

// AddDocument прикрепляет документ к сделке.
func (p *Pipeline) AddDocument(ctx context.Context, dealID uuid.UUID, viewer Viewer, form DocumentForm) (*store.Document, error) {
	access, err := p.requireManage(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}

	kind := store.DocumentKind(normalizeEnum(form.Kind))
	if !kind.Valid() {
		return nil, apierr.BadRequest("Неизвестный вид документа")
	}

	title := normalizeShortText(form.Title, 200)
	if title == "" {
		title = kind.Title()
	}

	doc, err := p.comms.AddDocument(ctx, store.AddDocumentParams{
		DealID:          dealID,
		Kind:            kind,
		Title:           title,
		FileURL:         form.FileURL,
		MimeType:        form.MimeType,
		Bytes:           form.Bytes,
		UploadedBy:      viewer.UserID,
		VisibleToClient: form.VisibleToClient,
		IsAdmin:         viewer.Role == domain.RoleAdmin,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Сделка")
		}
		return nil, apierr.Internal(err)
	}

	if form.VisibleToClient {
		if err := p.notify.Create(ctx, store.CreateNotificationParams{
			UserID: access.Deal.ClientID,
			Kind:   store.NotifyDealStageChanged,
			Title:  "Добавлен документ по сделке",
			Body:   title,
			Link:   fmt.Sprintf("/cabinet/deals/%s", dealID),
		}); err != nil {
			p.log.ErrorContext(ctx, "не удалось уведомить о документе", "error", err)
		}
	}

	p.recordAudit(ctx, viewer.UserID, "deal.document.add", dealID.String(),
		map[string]any{"kind": string(kind)})
	return doc, nil
}

// DocumentForDownload отдаёт документ после проверки участия в сделке.
func (p *Pipeline) DocumentForDownload(ctx context.Context, docID uuid.UUID, viewer Viewer) (*store.Document, error) {
	doc, err := p.comms.DocumentForDownload(ctx, docID, viewer.UserID, viewer.Role == domain.RoleAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Документ")
		}
		return nil, apierr.Internal(err)
	}
	return doc, nil
}

// DeleteDocument удаляет документ сделки.
func (p *Pipeline) DeleteDocument(ctx context.Context, dealID, docID uuid.UUID, viewer Viewer) error {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return err
	}

	if err := p.comms.DeleteDocument(ctx, docID, dealID, viewer.UserID, viewer.Role == domain.RoleAdmin); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Документ")
		}
		return apierr.Internal(err)
	}
	return nil
}

func (p *Pipeline) LeaveReview(ctx context.Context, dealID uuid.UUID, viewer Viewer, rating int, text string) (*store.Review, error) {
	access, err := p.access(ctx, dealID, viewer)
	if err != nil {
		return nil, err
	}
	if !access.IsClient {
		return nil, apierr.Forbidden("Отзыв оставляет покупатель")
	}
	if access.Deal.Outcome != domain.OutcomeWon && access.Deal.Stage != domain.StageHandover {
		return nil, apierr.Conflict("Отзыв можно оставить после выдачи автомобиля")
	}
	if rating < 1 || rating > 5 {
		return nil, apierr.Validation(map[string]string{"rating": "оценка от 1 до 5"})
	}
	if len([]rune(text)) > 4000 {
		return nil, apierr.Validation(map[string]string{"text": "слишком длинный текст"})
	}

	existing, err := p.reviews.ByDeal(ctx, dealID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if existing != nil {
		return nil, apierr.Conflict("Отзыв по этой сделке уже оставлен")
	}

	rev, err := p.reviews.Create(ctx, store.CreateReviewParams{
		DealID:   dealID,
		DealerID: access.Deal.DealerID,
		AuthorID: viewer.UserID,
		Rating:   rating,
		Text:     text,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return rev, nil
}

func (p *Pipeline) recordAudit(ctx context.Context, actorID uuid.UUID, action, entityID string, diff map[string]any) {
	if err := p.audit.RecordAudit(ctx, store.AuditEntry{
		ActorID:  &actorID,
		Action:   action,
		Entity:   "deal",
		EntityID: entityID,
		Diff:     diff,
	}); err != nil {
		p.log.ErrorContext(ctx, "не удалось записать аудит сделки", "error", err, "action", action)
	}
}
