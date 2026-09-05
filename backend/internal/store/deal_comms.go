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

// DealComms — переписка, задачи и документы сделки.
//
// Объединены в один тип осознанно: все три сущности живут внутри сделки,
// имеют одинаковую проверку доступа и почти всегда запрашиваются вместе при
// открытии карточки.
type DealComms struct {
	pool *Pool
}

func NewDealComms(pool *Pool) *DealComms { return &DealComms{pool: pool} }

// --- Переписка --------------------------------------------------------------

// Message — сообщение в ленте сделки.
type Message struct {
	ID       int64
	DealID   uuid.UUID
	AuthorID uuid.UUID

	AuthorName string
	AuthorRole domain.Role

	Body          string
	IsSystem      bool
	AttachmentURL string

	ReadAt    *time.Time
	CreatedAt time.Time
}

// AddMessage добавляет сообщение в ленту.
//
// Вставка только если автор — участник сделки (или администратор). Иначе
// нулевая выборка: чужой UUID не создаёт строку и не отличает «нет сделки»
// от «чужая сделка».
func (c *DealComms) AddMessage(ctx context.Context, dealID, authorID uuid.UUID, body, attachmentURL string, isAdmin bool) (*Message, error) {
	row := c.pool.QueryRow(ctx, `
		INSERT INTO deal_messages (deal_id, author_id, body, attachment_url)
		SELECT $1, $2, $3, NULLIF($4, '')
		WHERE EXISTS (
			SELECT 1 FROM deals
			WHERE deals.id = $1
			  AND ($5 OR deals.client_id = $2 OR deals.dealer_id = $2)
		)
		RETURNING id, deal_id, author_id, body, is_system, COALESCE(attachment_url, ''), read_at, created_at`,
		dealID, authorID, body, attachmentURL, isAdmin)

	var msg Message
	if err := row.Scan(&msg.ID, &msg.DealID, &msg.AuthorID, &msg.Body,
		&msg.IsSystem, &msg.AttachmentURL, &msg.ReadAt, &msg.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("добавление сообщения: %w", err)
	}
	return &msg, nil
}

// AddSystemMessage пишет системное сообщение от имени участника сделки.
//
// Системные записи идут в ту же ленту, что и переписка: клиент видит единую
// хронологию, а не отдельный «журнал событий», который никто не открывает.
func (c *DealComms) AddSystemMessage(ctx context.Context, dealID, authorID uuid.UUID, body string) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO deal_messages (deal_id, author_id, body, is_system)
		VALUES ($1, $2, $3, true)`, dealID, authorID, body)
	if err != nil {
		return fmt.Errorf("добавление системного сообщения: %w", err)
	}
	return nil
}

// Messages возвращает ленту сообщений.
//
// Постраничность по возрастающему id от конца: лента читается «свежие
// внизу», а догрузка идёт вверх, как в любом мессенджере.
func (c *DealComms) Messages(ctx context.Context, dealID uuid.UUID, beforeID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	condition := ""
	args := []any{dealID, limit}
	if beforeID > 0 {
		condition = " AND m.id < $3"
		args = append(args, beforeID)
	}

	rows, err := c.pool.Query(ctx, fmt.Sprintf(`
		SELECT m.id, m.deal_id, m.author_id, u.full_name, u.role,
		       m.body, m.is_system, COALESCE(m.attachment_url, ''), m.read_at, m.created_at
		FROM deal_messages m
		JOIN users u ON u.id = m.author_id
		WHERE m.deal_id = $1%s
		ORDER BY m.id DESC
		LIMIT $2`, condition), args...)
	if err != nil {
		return nil, fmt.Errorf("чтение переписки: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.DealID, &msg.AuthorID, &msg.AuthorName, &msg.AuthorRole,
			&msg.Body, &msg.IsSystem, &msg.AttachmentURL, &msg.ReadAt, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор сообщения: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Разворачиваем в хронологический порядок: выборка шла от новых к старым
	// ради индекса, а интерфейсу нужен обратный порядок.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// MarkMessagesRead отмечает прочитанными сообщения собеседника.
func (c *DealComms) MarkMessagesRead(ctx context.Context, dealID, readerID uuid.UUID) (int64, error) {
	tag, err := c.pool.Exec(ctx, `
		UPDATE deal_messages SET read_at = now()
		WHERE deal_id = $1 AND author_id <> $2 AND read_at IS NULL`, dealID, readerID)
	if err != nil {
		return 0, fmt.Errorf("отметка сообщений прочитанными: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- Задачи -----------------------------------------------------------------

// Task — задача по сделке.
type Task struct {
	ID          uuid.UUID
	DealID      uuid.UUID
	Title       string
	Description string
	Stage       *domain.Stage

	AssigneeID   *uuid.UUID
	AssigneeName string

	DueAt     *time.Time
	DoneAt    *time.Time
	CreatedAt time.Time
}

// Overdue сообщает, что срок задачи прошёл.
func (t *Task) Overdue(now time.Time) bool {
	return t.DoneAt == nil && t.DueAt != nil && t.DueAt.Before(now)
}

// CreateTaskParams — параметры создания задачи.
type CreateTaskParams struct {
	DealID      uuid.UUID
	Title       string
	Description string
	Stage       *domain.Stage
	AssigneeID  *uuid.UUID
	CreatedBy   uuid.UUID
	DueAt       *time.Time
	IsAdmin     bool
}

// CreateTask создаёт задачу.
func (c *DealComms) CreateTask(ctx context.Context, params CreateTaskParams) (*Task, error) {
	row := c.pool.QueryRow(ctx, `
		INSERT INTO deal_tasks (deal_id, title, description, stage, assignee_id, created_by, due_at)
		SELECT $1, $2, $3, $4, $5, $6, $7
		WHERE EXISTS (
			SELECT 1 FROM deals
			WHERE deals.id = $1
			  AND ($8 OR deals.dealer_id = $6)
		)
		RETURNING id, deal_id, title, description, stage, assignee_id, due_at, done_at, created_at`,
		params.DealID, params.Title, params.Description, params.Stage,
		params.AssigneeID, params.CreatedBy, params.DueAt, params.IsAdmin)

	var task Task
	if err := row.Scan(&task.ID, &task.DealID, &task.Title, &task.Description, &task.Stage,
		&task.AssigneeID, &task.DueAt, &task.DoneAt, &task.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("создание задачи: %w", err)
	}
	return &task, nil
}

// Tasks возвращает задачи сделки.
func (c *DealComms) Tasks(ctx context.Context, dealID uuid.UUID) ([]Task, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT t.id, t.deal_id, t.title, t.description, t.stage,
		       t.assignee_id, COALESCE(u.full_name, ''), t.due_at, t.done_at, t.created_at
		FROM deal_tasks t
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE t.deal_id = $1
		ORDER BY t.done_at NULLS FIRST, t.due_at NULLS LAST, t.created_at`, dealID)
	if err != nil {
		return nil, fmt.Errorf("чтение задач сделки: %w", err)
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.DealID, &task.Title, &task.Description, &task.Stage,
			&task.AssigneeID, &task.AssigneeName, &task.DueAt, &task.DoneAt, &task.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор задачи: %w", err)
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// ToggleTask отмечает задачу выполненной или снимает отметку.
//
// Условие по deal_id обязательно: идентификатор задачи сам по себе не
// подтверждает право её менять.
func (c *DealComms) ToggleTask(ctx context.Context, taskID, dealID, actorID uuid.UUID, isAdmin, done bool) error {
	var doneAt any
	if done {
		doneAt = time.Now()
	}

	tag, err := c.pool.Exec(ctx, `
		UPDATE deal_tasks t
		SET done_at = $3
		FROM deals
		WHERE t.id = $1 AND t.deal_id = $2 AND deals.id = t.deal_id
		  AND ($4 OR deals.dealer_id = $5)`,
		taskID, dealID, doneAt, isAdmin, actorID)
	if err != nil {
		return fmt.Errorf("изменение задачи: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTask удаляет задачу.
func (c *DealComms) DeleteTask(ctx context.Context, taskID, dealID, actorID uuid.UUID, isAdmin bool) error {
	tag, err := c.pool.Exec(ctx, `
		DELETE FROM deal_tasks t
		USING deals
		WHERE t.id = $1 AND t.deal_id = $2 AND deals.id = t.deal_id
		  AND ($3 OR deals.dealer_id = $4)`,
		taskID, dealID, isAdmin, actorID)
	if err != nil {
		return fmt.Errorf("удаление задачи: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DueTasks возвращает задачи с подходящим сроком для напоминаний.
func (c *DealComms) DueTasks(ctx context.Context, within time.Duration, limit int) ([]Task, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	rows, err := c.pool.Query(ctx, `
		SELECT t.id, t.deal_id, t.title, t.description, t.stage,
		       t.assignee_id, COALESCE(u.full_name, ''), t.due_at, t.done_at, t.created_at
		FROM deal_tasks t
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE t.done_at IS NULL
		  AND t.due_at IS NOT NULL
		  AND t.due_at <= now() + $1::interval
		ORDER BY t.due_at
		LIMIT $2`, within, limit)
	if err != nil {
		return nil, fmt.Errorf("выборка задач с подходящим сроком: %w", err)
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.DealID, &task.Title, &task.Description, &task.Stage,
			&task.AssigneeID, &task.AssigneeName, &task.DueAt, &task.DoneAt, &task.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор задачи: %w", err)
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

// --- Документы --------------------------------------------------------------

// DocumentKind — вид документа сделки.
type DocumentKind string

const (
	DocContract    DocumentKind = "contract"
	DocInvoice     DocumentKind = "invoice"
	DocPayment     DocumentKind = "payment_order"
	DocCustoms     DocumentKind = "customs_declaration"
	DocPassport    DocumentKind = "passport"
	DocCertificate DocumentKind = "vehicle_certificate"
	DocAcceptance  DocumentKind = "acceptance_act"
	DocOther       DocumentKind = "other"
)

var documentTitles = map[DocumentKind]string{
	DocContract:    "Договор",
	DocInvoice:     "Инвойс",
	DocPayment:     "Платёжное поручение",
	DocCustoms:     "Таможенная декларация",
	DocPassport:    "Паспорт",
	DocCertificate: "Документ на автомобиль",
	DocAcceptance:  "Акт приёма-передачи",
	DocOther:       "Прочее",
}

func (k DocumentKind) Valid() bool { _, ok := documentTitles[k]; return ok }

func (k DocumentKind) Title() string {
	if title, ok := documentTitles[k]; ok {
		return title
	}
	return string(k)
}

// DocumentKinds возвращает справочник видов документов.
func DocumentKinds() []domain.DictionaryEntry {
	order := []DocumentKind{
		DocContract, DocInvoice, DocPayment, DocCustoms,
		DocCertificate, DocAcceptance, DocPassport, DocOther,
	}
	out := make([]domain.DictionaryEntry, 0, len(order))
	for _, kind := range order {
		out = append(out, domain.DictionaryEntry{Value: string(kind), Title: kind.Title()})
	}
	return out
}

// Document — документ сделки.
type Document struct {
	ID     uuid.UUID
	DealID uuid.UUID

	Kind     DocumentKind
	Title    string
	FileURL  string
	MimeType string
	Bytes    int

	UploadedBy      *uuid.UUID
	UploaderName    string
	VisibleToClient bool

	CreatedAt time.Time
}

// AddDocumentParams — параметры добавления документа.
type AddDocumentParams struct {
	DealID          uuid.UUID
	Kind            DocumentKind
	Title           string
	FileURL         string
	MimeType        string
	Bytes           int
	UploadedBy      uuid.UUID
	VisibleToClient bool
	IsAdmin         bool
}

// AddDocument добавляет документ к сделке.
func (c *DealComms) AddDocument(ctx context.Context, params AddDocumentParams) (*Document, error) {
	row := c.pool.QueryRow(ctx, `
		INSERT INTO deal_documents
		    (deal_id, kind, title, file_url, mime_type, bytes, uploaded_by, visible_to_client)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8
		WHERE EXISTS (
			SELECT 1 FROM deals
			WHERE deals.id = $1
			  AND ($9 OR deals.dealer_id = $7)
		)
		RETURNING id, deal_id, kind, title, file_url, mime_type, bytes,
		          uploaded_by, visible_to_client, created_at`,
		params.DealID, params.Kind, params.Title, params.FileURL,
		params.MimeType, params.Bytes, params.UploadedBy, params.VisibleToClient, params.IsAdmin)

	var doc Document
	if err := row.Scan(&doc.ID, &doc.DealID, &doc.Kind, &doc.Title, &doc.FileURL,
		&doc.MimeType, &doc.Bytes, &doc.UploadedBy, &doc.VisibleToClient, &doc.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("добавление документа: %w", err)
	}
	return &doc, nil
}

// Documents возвращает документы сделки.
//
// Флаг onlyClientVisible ставится для клиента: часть документов дилер
// готовит для внутреннего употребления, и показывать их клиенту нельзя.
func (c *DealComms) Documents(ctx context.Context, dealID uuid.UUID, onlyClientVisible bool) ([]Document, error) {
	condition := ""
	if onlyClientVisible {
		condition = " AND d.visible_to_client"
	}

	rows, err := c.pool.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.deal_id, d.kind, d.title, d.file_url, d.mime_type, d.bytes,
		       d.uploaded_by, COALESCE(u.full_name, ''), d.visible_to_client, d.created_at
		FROM deal_documents d
		LEFT JOIN users u ON u.id = d.uploaded_by
		WHERE d.deal_id = $1%s
		ORDER BY d.created_at DESC`, condition), dealID)
	if err != nil {
		return nil, fmt.Errorf("чтение документов сделки: %w", err)
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		var doc Document
		if err := rows.Scan(&doc.ID, &doc.DealID, &doc.Kind, &doc.Title, &doc.FileURL,
			&doc.MimeType, &doc.Bytes, &doc.UploadedBy, &doc.UploaderName,
			&doc.VisibleToClient, &doc.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор документа: %w", err)
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

// DocumentForDownload возвращает документ с проверкой доступа.
//
// Проверка встроена в запрос: документ отдаётся только участнику сделки, а
// клиенту — только если он помечен видимым. Прямая ссылка на файл без этой
// проверки означала бы утечку договоров и паспортов по угадываемому адресу.
func (c *DealComms) DocumentForDownload(ctx context.Context, docID, userID uuid.UUID, isAdmin bool) (*Document, error) {
	condition := `
		AND (
			deals.dealer_id = $2
			OR (deals.client_id = $2 AND d.visible_to_client)
		)`
	if isAdmin {
		condition = ""
	}

	row := c.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT d.id, d.deal_id, d.kind, d.title, d.file_url, d.mime_type, d.bytes,
		       d.uploaded_by, '', d.visible_to_client, d.created_at
		FROM deal_documents d
		JOIN deals ON deals.id = d.deal_id
		WHERE d.id = $1%s`, condition), docID, userID)

	var doc Document
	if err := row.Scan(&doc.ID, &doc.DealID, &doc.Kind, &doc.Title, &doc.FileURL,
		&doc.MimeType, &doc.Bytes, &doc.UploadedBy, &doc.UploaderName,
		&doc.VisibleToClient, &doc.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение документа: %w", err)
	}
	return &doc, nil
}

// DeleteDocument удаляет документ сделки.
func (c *DealComms) DeleteDocument(ctx context.Context, docID, dealID, actorID uuid.UUID, isAdmin bool) error {
	tag, err := c.pool.Exec(ctx, `
		DELETE FROM deal_documents d
		USING deals
		WHERE d.id = $1 AND d.deal_id = $2 AND deals.id = d.deal_id
		  AND ($3 OR deals.dealer_id = $4)`,
		docID, dealID, isAdmin, actorID)
	if err != nil {
		return fmt.Errorf("удаление документа: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
