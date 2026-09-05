package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Notifications — уведомления в интерфейсе.
type Notifications struct {
	pool *Pool
}

func NewNotifications(pool *Pool) *Notifications { return &Notifications{pool: pool} }

// Виды уведомлений. Значения совпадают с enum notification_kind.
const (
	NotifyRequestCreated   = "request_created"
	NotifyRequestAnswered  = "request_answered"
	NotifyDealCreated      = "deal_created"
	NotifyDealStageChanged = "deal_stage_changed"
	NotifyDealMessage      = "deal_message"
	NotifyDealTaskDue      = "deal_task_due"
	NotifyReviewReceived   = "review_received"
	NotifyModerationResult = "moderation_result"
	NotifySecurityAlert    = "security_alert"
)

// Notification — уведомление.
type Notification struct {
	ID        int64          `json:"id"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Body      string         `json:"body,omitempty"`
	Link      string         `json:"link,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	ReadAt    *time.Time     `json:"read_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CreateNotificationParams — параметры создания уведомления.
type CreateNotificationParams struct {
	UserID  uuid.UUID
	Kind    string
	Title   string
	Body    string
	Link    string
	Payload map[string]any
}

// Create создаёт уведомление.
func (n *Notifications) Create(ctx context.Context, params CreateNotificationParams) error {
	payload := []byte("{}")
	if len(params.Payload) > 0 {
		encoded, err := json.Marshal(params.Payload)
		if err != nil {
			return fmt.Errorf("сериализация данных уведомления: %w", err)
		}
		payload = encoded
	}

	_, err := n.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, kind, title, body, link, payload)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)`,
		params.UserID, params.Kind, truncate(params.Title, 200),
		truncate(params.Body, 1000), params.Link, payload)
	if err != nil {
		return fmt.Errorf("создание уведомления: %w", err)
	}
	return nil
}

// CreateMany создаёт уведомления пакетом.
//
// Используется при событиях, затрагивающих обе стороны сделки: два
// отдельных запроса на каждое событие — лишний расход соединений пула.
func (n *Notifications) CreateMany(ctx context.Context, items []CreateNotificationParams) error {
	if len(items) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, item := range items {
		payload := []byte("{}")
		if len(item.Payload) > 0 {
			if encoded, err := json.Marshal(item.Payload); err == nil {
				payload = encoded
			}
		}
		batch.Queue(`
			INSERT INTO notifications (user_id, kind, title, body, link, payload)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)`,
			item.UserID, item.Kind, truncate(item.Title, 200),
			truncate(item.Body, 1000), item.Link, payload)
	}

	results := n.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for range items {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("создание уведомлений пакетом: %w", err)
		}
	}
	return nil
}

// List возвращает уведомления пользователя.
func (n *Notifications) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, beforeID int64, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	conditions := ""
	args := []any{userID, limit}
	if unreadOnly {
		conditions += " AND read_at IS NULL"
	}
	if beforeID > 0 {
		conditions += fmt.Sprintf(" AND id < $%d", len(args)+1)
		args = append(args, beforeID)
	}

	rows, err := n.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, kind, title, body, COALESCE(link, ''), payload, read_at, created_at
		FROM notifications
		WHERE user_id = $1%s
		ORDER BY id DESC
		LIMIT $2`, conditions), args...)
	if err != nil {
		return nil, fmt.Errorf("чтение уведомлений: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var item Notification
		var payload []byte
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Body,
			&item.Link, &payload, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор уведомления: %w", err)
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &item.Payload)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UnreadCount возвращает число непрочитанных уведомлений.
func (n *Notifications) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	// Подсчёт ограничен сверху: интерфейс всё равно показывает «99+»,
	// а точное число при тысяче уведомлений никому не нужно.
	err := n.pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT 1 FROM notifications
			WHERE user_id = $1 AND read_at IS NULL
			LIMIT 100
		) AS limited`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("подсчёт непрочитанных уведомлений: %w", err)
	}
	return count, nil
}

// MarkRead отмечает уведомления прочитанными.
func (n *Notifications) MarkRead(ctx context.Context, userID uuid.UUID, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := n.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND id = ANY($2) AND read_at IS NULL`, userID, ids)
	if err != nil {
		return fmt.Errorf("отметка уведомлений прочитанными: %w", err)
	}
	return nil
}

// MarkAllRead отмечает прочитанными все уведомления пользователя.
func (n *Notifications) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := n.pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("отметка всех уведомлений прочитанными: %w", err)
	}
	return nil
}

// DeleteOld удаляет прочитанные уведомления старше срока.
func (n *Notifications) DeleteOld(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := n.pool.Exec(ctx, `
		DELETE FROM notifications
		WHERE read_at IS NOT NULL AND created_at < now() - $1::interval`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("очистка уведомлений: %w", err)
	}
	return tag.RowsAffected(), nil
}
