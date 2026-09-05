package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/pkg/storage"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// DealHandler — обработчики воронки сделок.
type DealHandler struct {
	pipeline *service.Pipeline
	disk     *storage.Disk
}

func NewDealHandler(pipeline *service.Pipeline, disk *storage.Disk) *DealHandler {
	return &DealHandler{pipeline: pipeline, disk: disk}
}

// Допустимые значения фильтров собираются из домена, а не перечисляются
// здесь руками: список, продублированный вручную, расходится с enum в базе
// при первом же переименовании этапа, и фильтр начинает молча отбрасывать
// корректные значения.
var (
	allowedStages   = stageValues()
	allowedOutcomes = []string{"open", "won", "lost"}
)

func stageValues() []string {
	out := make([]string, 0, len(domain.StageOrder))
	for _, stage := range domain.StageOrder {
		out = append(out, string(stage))
	}
	return out
}

// --- Представления ----------------------------------------------------------

type dealResponse struct {
	ID     uuid.UUID `json:"id"`
	Number int64     `json:"number"`

	ClientID uuid.UUID  `json:"client_id"`
	DealerID uuid.UUID  `json:"dealer_id"`
	CarID    *uuid.UUID `json:"car_id,omitempty"`
	SellerID *uuid.UUID `json:"seller_id,omitempty"`

	Stage      string `json:"stage"`
	StageTitle string `json:"stage_title"`
	// StagePosition нужен интерфейсу для полосы прогресса, чтобы не хранить
	// порядок этапов отдельной копией на клиенте.
	StagePosition int    `json:"stage_position"`
	Outcome       string `json:"outcome"`

	Title          string  `json:"title"`
	AmountMinor    *int64  `json:"amount_minor,omitempty"`
	Currency       string  `json:"currency"`
	AmountRubMinor *int64  `json:"amount_rub_minor,omitempty"`
	AmountLabel    string  `json:"amount_label,omitempty"`
	PaidRubMinor   int64   `json:"paid_rub_minor"`
	PaidShare      float64 `json:"paid_share"`

	StageChangedAt     time.Time  `json:"stage_changed_at"`
	DaysOnStage        int        `json:"days_on_stage"`
	IsStale            bool       `json:"is_stale"`
	ExpectedHandoverAt *time.Time `json:"expected_handover_at,omitempty"`

	LostReason  string `json:"lost_reason,omitempty"`
	ManagerNote string `json:"manager_note,omitempty"`

	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func toDealResponse(deal *domain.Deal) dealResponse {
	now := time.Now()

	response := dealResponse{
		ID:       deal.ID,
		Number:   deal.PublicNumber,
		ClientID: deal.ClientID,
		DealerID: deal.DealerID,
		CarID:    deal.CarID,
		SellerID: deal.SellerID,

		Stage:         string(deal.Stage),
		StageTitle:    deal.Stage.Title(),
		StagePosition: deal.Stage.Position(),
		Outcome:       string(deal.Outcome),

		Title:          deal.Title,
		AmountMinor:    deal.AmountMinor,
		Currency:       string(deal.Currency),
		AmountRubMinor: deal.AmountRubMinor,
		PaidRubMinor:   deal.PaidRubMinor,

		StageChangedAt:     deal.StageChangedAt,
		DaysOnStage:        deal.DaysOnStage(now),
		IsStale:            deal.IsStale(now),
		ExpectedHandoverAt: deal.ExpectedHandoverAt,

		LostReason:  deal.LostReason,
		ManagerNote: deal.ManagerNote,

		ClosedAt:  deal.ClosedAt,
		CreatedAt: deal.CreatedAt,
		UpdatedAt: deal.UpdatedAt,
	}

	if deal.AmountRubMinor != nil {
		response.AmountLabel = money.FormatRub(*deal.AmountRubMinor)
		if *deal.AmountRubMinor > 0 {
			response.PaidShare = float64(deal.PaidRubMinor) / float64(*deal.AmountRubMinor)
		}
	}
	return response
}

type dealListItemResponse struct {
	dealResponse

	ClientName     string `json:"client_name"`
	DealerName     string `json:"dealer_name"`
	CarTitle       string `json:"car_title,omitempty"`
	OpenTasks      int    `json:"open_tasks"`
	UnreadMessages int    `json:"unread_messages"`
}

type stageHistoryResponse struct {
	FromStage  *string `json:"from_stage,omitempty"`
	ToStage    string  `json:"to_stage"`
	StageTitle string  `json:"stage_title"`
	Outcome    string  `json:"outcome"`

	ChangedBy string `json:"changed_by"`
	Comment   string `json:"comment,omitempty"`
	// DurationDays — сколько сделка провела на предыдущем этапе.
	DurationDays *float64  `json:"duration_days,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type taskResponse struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Stage       *string   `json:"stage,omitempty"`

	AssigneeName string     `json:"assignee_name,omitempty"`
	DueAt        *time.Time `json:"due_at,omitempty"`
	DoneAt       *time.Time `json:"done_at,omitempty"`
	Overdue      bool       `json:"overdue"`
	CreatedAt    time.Time  `json:"created_at"`
}

type documentResponse struct {
	ID    uuid.UUID `json:"id"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
	// URL отдаётся как ссылка на защищённую выдачу, а не путь в хранилище.
	URL             string    `json:"url"`
	MimeType        string    `json:"mime_type,omitempty"`
	Bytes           int       `json:"bytes"`
	SizeLabel       string    `json:"size_label"`
	UploaderName    string    `json:"uploader_name,omitempty"`
	VisibleToClient bool      `json:"visible_to_client"`
	CreatedAt       time.Time `json:"created_at"`
}

type messageResponse struct {
	ID            int64      `json:"id"`
	AuthorID      uuid.UUID  `json:"author_id"`
	AuthorName    string     `json:"author_name"`
	AuthorRole    string     `json:"author_role"`
	Body          string     `json:"body"`
	IsSystem      bool       `json:"is_system"`
	AttachmentURL string     `json:"attachment_url,omitempty"`
	ReadAt        *time.Time `json:"read_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toStageHistory(entries []store.StageHistoryEntry) []stageHistoryResponse {
	out := make([]stageHistoryResponse, 0, len(entries))
	for _, entry := range entries {
		item := stageHistoryResponse{
			ToStage:    string(entry.ToStage),
			StageTitle: entry.ToStage.Title(),
			Outcome:    string(entry.Outcome),
			ChangedBy:  entry.ChangedByName,
			Comment:    entry.Comment,
			CreatedAt:  entry.CreatedAt,
		}
		if entry.FromStage != nil {
			value := string(*entry.FromStage)
			item.FromStage = &value
		}
		if entry.DurationSeconds != nil {
			days := float64(*entry.DurationSeconds) / 86400.0
			item.DurationDays = &days
		}
		out = append(out, item)
	}
	return out
}

func toTasks(tasks []store.Task) []taskResponse {
	now := time.Now()
	out := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		item := taskResponse{
			ID:           task.ID,
			Title:        task.Title,
			Description:  task.Description,
			AssigneeName: task.AssigneeName,
			DueAt:        task.DueAt,
			DoneAt:       task.DoneAt,
			Overdue:      task.Overdue(now),
			CreatedAt:    task.CreatedAt,
		}
		if task.Stage != nil {
			value := string(*task.Stage)
			item.Stage = &value
		}
		out = append(out, item)
	}
	return out
}

func toDocuments(documents []store.Document) []documentResponse {
	out := make([]documentResponse, 0, len(documents))
	for _, doc := range documents {
		out = append(out, documentResponse{
			ID:    doc.ID,
			Kind:  string(doc.Kind),
			Title: doc.Title,
			// Прямой путь в хранилище наружу не отдаётся: скачивание идёт
			// через обработчик, который проверяет права на сделку.
			URL:             "/api/v1/documents/" + doc.ID.String(),
			MimeType:        doc.MimeType,
			Bytes:           doc.Bytes,
			SizeLabel:       formatBytes(doc.Bytes),
			UploaderName:    doc.UploaderName,
			VisibleToClient: doc.VisibleToClient,
			CreatedAt:       doc.CreatedAt,
		})
	}
	return out
}

func toMessages(messages []store.Message) []messageResponse {
	out := make([]messageResponse, 0, len(messages))
	for _, msg := range messages {
		out = append(out, messageResponse{
			ID:            msg.ID,
			AuthorID:      msg.AuthorID,
			AuthorName:    msg.AuthorName,
			AuthorRole:    string(msg.AuthorRole),
			Body:          msg.Body,
			IsSystem:      msg.IsSystem,
			AttachmentURL: msg.AttachmentURL,
			ReadAt:        msg.ReadAt,
			CreatedAt:     msg.CreatedAt,
		})
	}
	return out
}

// --- Обработчики ------------------------------------------------------------

func viewerFrom(r *http.Request) service.Viewer {
	actor := ActorFrom(r.Context())
	return service.Viewer{UserID: actor.UserID, Role: actor.Role}
}

type createDealBody struct {
	RequestID string `json:"request_id"`
	ClientID  string `json:"client_id"`
	CarID     string `json:"car_id"`
	SellerID  string `json:"seller_id"`

	ClientName  string `json:"client_name"`
	ClientEmail string `json:"client_email"`
	ClientPhone string `json:"client_phone"`

	Title       string `json:"title"`
	AmountMinor *int64 `json:"amount_minor"`
	Currency    string `json:"currency"`
	Stage       string `json:"stage"`
}

// SearchClients — GET /api/v1/deals/clients
func (h *DealHandler) SearchClients(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	query := q.String("q", 120)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	items, err := h.pipeline.SearchClients(r.Context(), viewerFrom(r), query)
	if err != nil {
		Error(w, r, err)
		return
	}
	if items == nil {
		items = []store.ClientLookup{}
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// Create — POST /api/v1/deals
func (h *DealHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createDealBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	deal, err := h.pipeline.CreateDeal(r.Context(), actor.UserID, service.CreateDealForm{
		RequestID:   body.RequestID,
		ClientID:    body.ClientID,
		CarID:       body.CarID,
		SellerID:    body.SellerID,
		ClientName:  body.ClientName,
		ClientEmail: body.ClientEmail,
		ClientPhone: body.ClientPhone,
		Title:       body.Title,
		AmountMinor: body.AmountMinor,
		Currency:    body.Currency,
		Stage:       body.Stage,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"deal": toDealResponse(deal)})
}

// List — GET /api/v1/deals
func (h *DealHandler) List(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	form := service.DealFilterForm{
		Stages:   q.EnumList("stage", 7, allowedStages...),
		Outcomes: q.EnumList("outcome", 3, allowedOutcomes...),
		Search:   q.String("q", 100),
	}
	if stale := q.Bool("stale"); stale != nil {
		form.StaleOnly = *stale
	}
	form.Limit, form.Offset = readOffsetPage(q, 50, 200)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	items, total, err := h.pipeline.ListDeals(r.Context(), viewerFrom(r), form)
	if err != nil {
		Error(w, r, err)
		return
	}

	response := make([]dealListItemResponse, 0, len(items))
	for _, item := range items {
		deal := item.Deal
		response = append(response, dealListItemResponse{
			dealResponse:   toDealResponse(&deal),
			ClientName:     item.ClientName,
			DealerName:     item.DealerName,
			CarTitle:       item.CarTitle,
			OpenTasks:      item.OpenTasks,
			UnreadMessages: item.UnreadMessages,
		})
	}

	JSON(w, http.StatusOK, map[string]any{"items": response, "total": total})
}

// Get — GET /api/v1/deals/{id}
func (h *DealHandler) Get(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	details, err := h.pipeline.DealDetails(r.Context(), dealID, viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	nextStages := make([]map[string]any, 0, len(details.NextStages))
	for _, stage := range details.NextStages {
		nextStages = append(nextStages, map[string]any{
			"value": string(stage),
			"title": stage.Title(),
		})
	}

	JSON(w, http.StatusOK, map[string]any{
		"deal":        toDealResponse(details.Deal),
		"history":     toStageHistory(details.History),
		"tasks":       toTasks(details.Tasks),
		"documents":   toDocuments(details.Documents),
		"next_stages": nextStages,
		"can_manage":  details.Access.CanManage(),
		"review":      toReview(details.Review),
		"can_review": details.Access.IsClient && details.Review == nil &&
			(details.Deal.Outcome == domain.OutcomeWon || details.Deal.Stage == domain.StageHandover),
	})
}

// Stages — GET /api/v1/deals/stages
func (h *DealHandler) Stages(w http.ResponseWriter, r *http.Request) {
	catalog := domain.StagesCatalog()
	stages := make([]map[string]any, 0, len(catalog))
	for _, meta := range catalog {
		stages = append(stages, map[string]any{
			"value":          string(meta.Stage),
			"title":          meta.Title,
			"position":       meta.Position,
			"normative_days": meta.NormativeDays,
			"description":    meta.Description,
		})
	}

	JSON(w, http.StatusOK, map[string]any{
		"stages":         stages,
		"document_kinds": store.DocumentKinds(),
	})
}

type changeStageBody struct {
	Stage   string `json:"stage"`
	Comment string `json:"comment"`
}

// ChangeStage — POST /api/v1/deals/{id}/stage
func (h *DealHandler) ChangeStage(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body changeStageBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	deal, err := h.pipeline.ChangeStage(r.Context(), dealID, viewerFrom(r), body.Stage, body.Comment)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"deal": toDealResponse(deal)})
}

type closeDealBody struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

// Close — POST /api/v1/deals/{id}/close
func (h *DealHandler) Close(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body closeDealBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	deal, err := h.pipeline.CloseDeal(r.Context(), dealID, viewerFrom(r), body.Outcome, body.Reason)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"deal": toDealResponse(deal)})
}

type updateDealBody struct {
	Title              string     `json:"title"`
	AmountMinor        *int64     `json:"amount_minor"`
	Currency           string     `json:"currency"`
	PaidRubMinor       *int64     `json:"paid_rub_minor"`
	ExpectedHandoverAt *time.Time `json:"expected_handover_at"`
	ManagerNote        *string    `json:"manager_note"`
	SellerID           string     `json:"seller_id"`
}

// Update — PATCH /api/v1/deals/{id}
func (h *DealHandler) Update(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body updateDealBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	deal, err := h.pipeline.UpdateDeal(r.Context(), dealID, viewerFrom(r), service.UpdateDealForm{
		Title:              body.Title,
		AmountMinor:        body.AmountMinor,
		Currency:           body.Currency,
		PaidRubMinor:       body.PaidRubMinor,
		ExpectedHandoverAt: body.ExpectedHandoverAt,
		ManagerNote:        body.ManagerNote,
		SellerID:           body.SellerID,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"deal": toDealResponse(deal)})
}

// Summary — GET /api/v1/deals/summary
func (h *DealHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.pipeline.Summary(r.Context(), viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"summary": summary})
}

// Messages — GET /api/v1/deals/{id}/messages
func (h *DealHandler) Messages(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	q := NewQuery(r)
	limit := 50
	if value := q.Int("limit", 1, 200); value != nil {
		limit = *value
	}
	beforeID := readInt64Query(q, "before_id")
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	messages, err := h.pipeline.Messages(r.Context(), dealID, viewerFrom(r), beforeID, limit)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"items": toMessages(messages)})
}

type sendMessageBody struct {
	Body          string `json:"body"`
	AttachmentURL string `json:"attachment_url"`
}

// SendMessage — POST /api/v1/deals/{id}/messages
func (h *DealHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body sendMessageBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	message, err := h.pipeline.SendMessage(r.Context(), dealID, viewerFrom(r), body.Body, body.AttachmentURL)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"message": toMessages([]store.Message{*message})[0]})
}

type createTaskBody struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Stage       string     `json:"stage"`
	DueAt       *time.Time `json:"due_at"`
	AssigneeID  string     `json:"assignee_id"`
}

// CreateTask — POST /api/v1/deals/{id}/tasks
func (h *DealHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body createTaskBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	task, err := h.pipeline.CreateTask(r.Context(), dealID, viewerFrom(r), service.TaskForm{
		Title:       body.Title,
		Description: body.Description,
		Stage:       body.Stage,
		DueAt:       body.DueAt,
		AssigneeID:  body.AssigneeID,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"task": toTasks([]store.Task{*task})[0]})
}

type toggleTaskBody struct {
	Done bool `json:"done"`
}

// ToggleTask — POST /api/v1/deals/{id}/tasks/{taskID}/toggle
func (h *DealHandler) ToggleTask(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	taskID, err := UUIDParam(r, "taskID")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body toggleTaskBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	if err := h.pipeline.ToggleTask(r.Context(), dealID, taskID, viewerFrom(r), body.Done); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"done": body.Done})
}

// DeleteTask — DELETE /api/v1/deals/{id}/tasks/{taskID}
func (h *DealHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	taskID, err := UUIDParam(r, "taskID")
	if err != nil {
		Error(w, r, err)
		return
	}

	if err := h.pipeline.DeleteTask(r.Context(), dealID, taskID, viewerFrom(r)); err != nil {
		Error(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type addDocumentBody struct {
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	FileURL         string `json:"file_url"`
	MimeType        string `json:"mime_type"`
	Bytes           int    `json:"bytes"`
	VisibleToClient bool   `json:"visible_to_client"`
}

// AddDocument — POST /api/v1/deals/{id}/documents
func (h *DealHandler) AddDocument(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body addDocumentBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}
	if strings.TrimSpace(body.FileURL) == "" {
		Error(w, r, apierr.Validation(map[string]string{"file_url": "загрузите файл"}))
		return
	}

	doc, err := h.pipeline.AddDocument(r.Context(), dealID, viewerFrom(r), service.DocumentForm{
		Kind:            body.Kind,
		Title:           body.Title,
		FileURL:         body.FileURL,
		MimeType:        body.MimeType,
		Bytes:           body.Bytes,
		VisibleToClient: body.VisibleToClient,
	})
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"document": toDocuments([]store.Document{*doc})[0]})
}

type generateDocumentsBody struct {
	Kind            string   `json:"kind"`
	Kinds           []string `json:"kinds"`
	VisibleToClient *bool    `json:"visible_to_client"`
}

// PreviewDocument — GET /api/v1/deals/{id}/documents/preview?kind=
func (h *DealHandler) PreviewDocument(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	html, _, title, err := h.pipeline.PreviewDocument(r.Context(), dealID, viewerFrom(r), kind)
	if err != nil {
		Error(w, r, err)
		return
	}
	writePrintableHTML(w, html, title)
}

// GenerateDocuments — POST /api/v1/deals/{id}/documents/generate
func (h *DealHandler) GenerateDocuments(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	var body generateDocumentsBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}
	kinds := body.Kinds
	if strings.TrimSpace(body.Kind) != "" {
		kinds = []string{body.Kind}
	}
	visible := true
	if body.VisibleToClient != nil {
		visible = *body.VisibleToClient
	}
	docs, warnings, err := h.pipeline.GenerateDocuments(r.Context(), dealID, viewerFrom(r), kinds, visible)
	if err != nil {
		Error(w, r, err)
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	JSON(w, http.StatusCreated, map[string]any{
		"documents": toDocuments(docs),
		"warnings":  warnings,
	})
}

// DownloadDocument — GET /api/v1/documents/{id}
func (h *DealHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	doc, err := h.pipeline.DocumentForDownload(r.Context(), docID, viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}
	if doc.FileURL == "" {
		Error(w, r, apierr.NotFound("Документ"))
		return
	}

	if h.disk != nil {
		if local, ok := h.disk.Resolve(doc.FileURL); ok {
			name := printableFilename(doc.Title, doc.MimeType)
			if doc.MimeType != "" {
				w.Header().Set("Content-Type", doc.MimeType)
			}
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
			http.ServeFile(w, r, local)
			return
		}
	}

	http.Redirect(w, r, doc.FileURL, http.StatusFound)
}

type leaveReviewBody struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

// LeaveReview — POST /api/v1/deals/{id}/review
func (h *DealHandler) LeaveReview(w http.ResponseWriter, r *http.Request) {
	dealID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	var body leaveReviewBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}
	rev, err := h.pipeline.LeaveReview(r.Context(), dealID, viewerFrom(r), body.Rating, strings.TrimSpace(body.Text))
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"review": toReview(rev)})
}

func toReview(rev *store.Review) any {
	if rev == nil {
		return nil
	}
	return map[string]any{
		"id":           rev.ID,
		"rating":       rev.Rating,
		"text":         rev.Text,
		"author_name":  rev.AuthorName,
		"dealer_reply": strings.TrimSpace(rev.DealerReply),
		"created_at":   rev.CreatedAt,
	}
}

// readInt64Query читает 64-битное целое из строки запроса.
func readInt64Query(q *Query, key string) int64 {
	value := q.Int64(key, 0, 1<<62)
	if value == nil {
		return 0
	}
	return *value
}

func writePrintableHTML(w http.ResponseWriter, body []byte, title string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `inline; filename="`+printableFilename(title, "text/html")+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func printableFilename(title, mime string) string {
	name := strings.TrimSpace(title)
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "/", "-")
	if name == "" {
		name = "document"
	}
	if strings.Contains(mime, "html") && !strings.HasSuffix(strings.ToLower(name), ".html") {
		name += ".html"
	}
	return name
}

// formatBytes возвращает размер файла в понятном виде.
func formatBytes(size int) string {
	switch {
	case size >= 1<<20:
		return strconv.FormatFloat(float64(size)/(1<<20), 'f', 1, 64) + " МБ"
	case size >= 1<<10:
		return strconv.Itoa(size/(1<<10)) + " КБ"
	default:
		return strconv.Itoa(size) + " Б"
	}
}
