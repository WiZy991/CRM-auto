package httpx

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// RequestHandler — обработчики заявок клиентов.
type RequestHandler struct {
	requests *service.Requests
}

func NewRequestHandler(requests *service.Requests) *RequestHandler {
	return &RequestHandler{requests: requests}
}

var allowedRequestStatuses = []string{
	"new", "in_progress", "answered", "converted", "rejected", "closed",
}

// --- Представления ----------------------------------------------------------

type requestResponse struct {
	ID     uuid.UUID `json:"id"`
	Number int64     `json:"number"`
	Status string    `json:"status"`
	// StatusTitle приходит с сервера, чтобы подписи статусов не расходились
	// между веб-интерфейсом и любыми другими клиентами.
	StatusTitle string `json:"status_title"`
	Summary     string `json:"summary"`

	DealerID *uuid.UUID `json:"dealer_id,omitempty"`
	CarID    *uuid.UUID `json:"car_id,omitempty"`

	DesiredBrand string  `json:"desired_brand,omitempty"`
	DesiredModel string  `json:"desired_model,omitempty"`
	YearFrom     *int    `json:"year_from,omitempty"`
	YearTo       *int    `json:"year_to,omitempty"`
	Origin       *string `json:"origin,omitempty"`
	Body         *string `json:"body,omitempty"`
	Gearbox      *string `json:"gearbox,omitempty"`

	BudgetFromRubMinor *int64 `json:"budget_from_rub_minor,omitempty"`
	BudgetToRubMinor   *int64 `json:"budget_to_rub_minor,omitempty"`
	BudgetLabel        string `json:"budget_label,omitempty"`

	Comment           string `json:"comment,omitempty"`
	ContactPreference string `json:"contact_preference"`

	DealerReply    string     `json:"dealer_reply,omitempty"`
	RepliedAt      *time.Time `json:"replied_at,omitempty"`
	RejectedReason string     `json:"rejected_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type requestListItemResponse struct {
	requestResponse

	ClientName  string `json:"client_name,omitempty"`
	ClientPhone string `json:"client_phone,omitempty"`
	DealerName  string `json:"dealer_name,omitempty"`
	CarTitle    string `json:"car_title,omitempty"`
	HasDeal     bool   `json:"has_deal"`
}

func toRequestResponse(request *domain.Request) requestResponse {
	response := requestResponse{
		ID:           request.ID,
		Number:       request.PublicNumber,
		Status:       string(request.Status),
		StatusTitle:  request.Status.Title(),
		Summary:      request.Summary(),
		DealerID:     request.DealerID,
		CarID:        request.CarID,
		DesiredBrand: request.DesiredBrand,
		DesiredModel: request.DesiredModel,
		YearFrom:     request.YearFrom,
		YearTo:       request.YearTo,

		BudgetFromRubMinor: request.BudgetFromRubMinor,
		BudgetToRubMinor:   request.BudgetToRubMinor,

		Comment:           request.Comment,
		ContactPreference: string(request.ContactPreference),

		DealerReply:    request.DealerReply,
		RepliedAt:      request.RepliedAt,
		RejectedReason: request.RejectedReason,

		CreatedAt: request.CreatedAt,
		UpdatedAt: request.UpdatedAt,
	}

	if request.Origin != nil {
		value := string(*request.Origin)
		response.Origin = &value
	}
	if request.Body != nil {
		value := string(*request.Body)
		response.Body = &value
	}
	if request.Gearbox != nil {
		value := string(*request.Gearbox)
		response.Gearbox = &value
	}
	response.BudgetLabel = formatBudget(request.BudgetFromRubMinor, request.BudgetToRubMinor)
	return response
}

// formatBudget собирает подпись бюджета.
func formatBudget(fromMinor, toMinor *int64) string {
	switch {
	case fromMinor != nil && toMinor != nil:
		return money.FormatRub(*fromMinor) + " — " + money.FormatRub(*toMinor)
	case fromMinor != nil:
		return "от " + money.FormatRub(*fromMinor)
	case toMinor != nil:
		return "до " + money.FormatRub(*toMinor)
	default:
		return ""
	}
}

func toRequestListItem(item store.RequestListItem) requestListItemResponse {
	return requestListItemResponse{
		requestResponse: toRequestResponse(&item.Request),
		ClientName:      item.ClientName,
		ClientPhone:     item.ClientPhone,
		DealerName:      item.DealerName,
		CarTitle:        item.CarTitle,
		HasDeal:         item.HasDeal,
	}
}

func toRequestList(items []store.RequestListItem) []requestListItemResponse {
	out := make([]requestListItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toRequestListItem(item))
	}
	return out
}

// --- Обработчики ------------------------------------------------------------

type createRequestBody struct {
	DealerID string `json:"dealer_id"`
	CarID    string `json:"car_id"`

	DesiredBrand string `json:"desired_brand"`
	DesiredModel string `json:"desired_model"`
	YearFrom     *int   `json:"year_from"`
	YearTo       *int   `json:"year_to"`
	Origin       string `json:"origin"`

	BudgetFromRub *int64 `json:"budget_from_rub"`
	BudgetToRub   *int64 `json:"budget_to_rub"`

	Body    string `json:"body"`
	Gearbox string `json:"gearbox"`

	Comment           string `json:"comment"`
	ContactPreference string `json:"contact_preference"`
}

// Create — POST /api/v1/requests
func (h *RequestHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createRequestBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	request, err := h.requests.Create(r.Context(), actor.UserID, service.RequestForm{
		DealerID:      body.DealerID,
		CarID:         body.CarID,
		DesiredBrand:  body.DesiredBrand,
		DesiredModel:  body.DesiredModel,
		YearFrom:      body.YearFrom,
		YearTo:        body.YearTo,
		Origin:        body.Origin,
		BudgetFromRub: body.BudgetFromRub,
		BudgetToRub:   body.BudgetToRub,

		Body:              body.Body,
		Gearbox:           body.Gearbox,
		Comment:           body.Comment,
		ContactPreference: body.ContactPreference,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"request": toRequestResponse(request)})
}

// MyRequests — GET /api/v1/requests/my
func (h *RequestHandler) MyRequests(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	statuses := q.EnumList("status", 6, allowedRequestStatuses...)
	limit, offset := readOffsetPage(q, 30, 100)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	items, total, err := h.requests.ListForClient(r.Context(), actor.UserID, statuses, limit, offset)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"items": toRequestList(items),
		"total": total,
	})
}

// DealerRequests — GET /api/v1/dealer/requests
func (h *RequestHandler) DealerRequests(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	statuses := q.EnumList("status", 6, allowedRequestStatuses...)
	limit, offset := readOffsetPage(q, 30, 100)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	items, total, err := h.requests.ListForDealer(r.Context(), actor.UserID, statuses, limit, offset)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"items": toRequestList(items),
		"total": total,
	})
}

// OpenPool — GET /api/v1/dealer/requests/pool
func (h *RequestHandler) OpenPool(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	limit, offset := readOffsetPage(q, 30, 100)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	items, total, err := h.requests.ListOpenPool(r.Context(), limit, offset)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"items": toRequestList(items),
		"total": total,
	})
}

// Get — GET /api/v1/requests/{id}
func (h *RequestHandler) Get(w http.ResponseWriter, r *http.Request) {
	requestID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	request, err := h.requests.Get(r.Context(), requestID, service.Viewer{
		UserID: actor.UserID, Role: actor.Role,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"request": toRequestResponse(request)})
}

// Claim — POST /api/v1/dealer/requests/{id}/claim
func (h *RequestHandler) Claim(w http.ResponseWriter, r *http.Request) {
	requestID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	request, err := h.requests.Claim(r.Context(), requestID, actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"request": toRequestResponse(request)})
}

type requestReplyBody struct {
	Reply string `json:"reply"`
}

// Reply — POST /api/v1/dealer/requests/{id}/reply
func (h *RequestHandler) Reply(w http.ResponseWriter, r *http.Request) {
	requestID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body requestReplyBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	request, err := h.requests.Reply(r.Context(), requestID, actor.UserID, body.Reply)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"request": toRequestResponse(request)})
}

type requestRejectBody struct {
	Reason string `json:"reason"`
}

// Reject — POST /api/v1/dealer/requests/{id}/reject
func (h *RequestHandler) Reject(w http.ResponseWriter, r *http.Request) {
	requestID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body requestRejectBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	request, err := h.requests.Reject(r.Context(), requestID, actor.UserID, body.Reason)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"request": toRequestResponse(request)})
}

// Close — POST /api/v1/requests/{id}/close
func (h *RequestHandler) Close(w http.ResponseWriter, r *http.Request) {
	requestID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.requests.Close(r.Context(), requestID, actor.UserID); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"status": "closed"})
}

// readOffsetPage читает постраничность со смещением.
//
// Смещение здесь допустимо: списки заявок и сделок у одного участника
// короткие, а курсор усложнил бы переход на произвольную страницу в
// табличном интерфейсе кабинета.
func readOffsetPage(q *Query, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if value := q.Int("limit", 1, maxLimit); value != nil {
		limit = *value
	}
	if value := q.Int("offset", 0, 10_000); value != nil {
		offset = *value
	}
	return limit, offset
}
