package httpx

import (
	"net/http"

	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// NotificationHandler — обработчики уведомлений в интерфейсе.
//
// Работают напрямую с хранилищем: прикладной логики здесь нет — только
// чтение своих уведомлений и отметка прочитанными, а идентификатор
// пользователя всегда берётся из токена.
type NotificationHandler struct {
	notify *store.Notifications
}

func NewNotificationHandler(notify *store.Notifications) *NotificationHandler {
	return &NotificationHandler{notify: notify}
}

// List — GET /api/v1/notifications
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)

	unreadOnly := false
	if value := q.Bool("unread"); value != nil {
		unreadOnly = *value
	}
	limit := 30
	if value := q.Int("limit", 1, 100); value != nil {
		limit = *value
	}
	beforeID := readInt64Query(q, "before_id")
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	items, err := h.notify.List(r.Context(), actor.UserID, unreadOnly, beforeID, limit)
	if err != nil {
		Error(w, r, apierr.Internal(err))
		return
	}

	unread, err := h.notify.UnreadCount(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, apierr.Internal(err))
		return
	}

	if items == nil {
		items = []store.Notification{}
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "unread": unread})
}

// UnreadCount — GET /api/v1/notifications/unread
//
// Отдельная точка для счётчика в шапке: интерфейс опрашивает её часто, и
// тянуть вместе с ней тридцать уведомлений с текстами незачем.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())

	unread, err := h.notify.UnreadCount(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, apierr.Internal(err))
		return
	}

	JSON(w, http.StatusOK, map[string]any{"unread": unread})
}

type markReadBody struct {
	IDs []int64 `json:"ids"`
	All bool    `json:"all"`
}

// MarkRead — POST /api/v1/notifications/read
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	var body markReadBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())

	if body.All {
		if err := h.notify.MarkAllRead(r.Context(), actor.UserID); err != nil {
			Error(w, r, apierr.Internal(err))
			return
		}
		JSON(w, http.StatusOK, map[string]any{"unread": 0})
		return
	}

	// Ограничение на размер списка: запрос с сотней тысяч идентификаторов
	// заставил бы базу строить огромный массив в условии.
	if len(body.IDs) > 200 {
		Error(w, r, apierr.BadRequest("Слишком много идентификаторов в запросе"))
		return
	}

	// Условие по user_id стоит внутри UPDATE, поэтому подставленный чужой
	// идентификатор уведомления просто не изменит ни одной строки.
	if err := h.notify.MarkRead(r.Context(), actor.UserID, body.IDs); err != nil {
		Error(w, r, apierr.Internal(err))
		return
	}

	unread, err := h.notify.UnreadCount(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, apierr.Internal(err))
		return
	}

	JSON(w, http.StatusOK, map[string]any{"unread": unread})
}
