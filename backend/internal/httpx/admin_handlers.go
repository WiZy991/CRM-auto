package httpx

import (
	"net/http"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// AdminHandler — обработчики административной панели.
type AdminHandler struct {
	admin   *service.Admin
	catalog *service.Catalog
}

func NewAdminHandler(admin *service.Admin, catalog *service.Catalog) *AdminHandler {
	return &AdminHandler{admin: admin, catalog: catalog}
}

var (
	allowedRoles        = []string{"client", "dealer", "seller", "admin"}
	allowedUserStatuses = []string{"pending", "active", "suspended", "deleted"}
	allowedAuditEntity  = []string{"user", "car", "request", "deal", "seller", "banner"}
)

// Overview — GET /api/v1/admin/overview
func (h *AdminHandler) Overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.admin.Overview(r.Context())
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"overview": overview})
}

// Users — GET /api/v1/admin/users
func (h *AdminHandler) Users(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)

	filter := store.UserFilter{
		Roles:    q.EnumList("role", 4, allowedRoles...),
		Statuses: q.EnumList("status", 4, allowedUserStatuses...),
		Search:   q.String("q", 200),
	}
	if unverified := q.Bool("unverified"); unverified != nil {
		filter.UnverifiedOnly = *unverified
	}
	filter.Limit, filter.Offset = readOffsetPage(q, 50, 200)

	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	rows, total, err := h.admin.Users(r.Context(), filter)
	if err != nil {
		Error(w, r, err)
		return
	}

	if rows == nil {
		rows = []store.UserRow{}
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows, "total": total})
}

type userStatusBody struct {
	Status string `json:"status"`
}

// SetUserStatus — PATCH /api/v1/admin/users/{id}/status
func (h *AdminHandler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body userStatusBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	if err := h.admin.SetUserStatus(r.Context(), userID, viewerFrom(r), body.Status); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"status": body.Status})
}

type userRoleBody struct {
	Role string `json:"role"`
}

// SetUserRole — PATCH /api/v1/admin/users/{id}/role
func (h *AdminHandler) SetUserRole(w http.ResponseWriter, r *http.Request) {
	userID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body userRoleBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	if err := h.admin.SetUserRole(r.Context(), userID, viewerFrom(r), body.Role); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"role": body.Role})
}

// Audit — GET /api/v1/admin/audit
func (h *AdminHandler) Audit(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)

	filter := store.AuditFilter{
		ActorID: q.UUID("actor_id"),
		Entity:  q.Enum("entity", allowedAuditEntity...),
		Action:  q.String("action", 60),
		Since:   q.Date("since"),
	}
	filter.Limit, filter.Offset = readOffsetPage(q, 100, 500)

	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	rows, err := h.admin.AuditLog(r.Context(), filter)
	if err != nil {
		Error(w, r, err)
		return
	}

	if rows == nil {
		rows = []store.AuditRow{}
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// SecurityEvents — GET /api/v1/admin/security-events
func (h *AdminHandler) SecurityEvents(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)

	kinds := q.StringList("kind", 10, 60)
	minSeverity := 1
	if value := q.Int("min_severity", 1, 5); value != nil {
		minSeverity = *value
	}
	limit, offset := readOffsetPage(q, 100, 500)

	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	rows, err := h.admin.SecurityEvents(r.Context(), kinds, minSeverity, limit, offset)
	if err != nil {
		Error(w, r, err)
		return
	}

	if rows == nil {
		rows = []store.SecurityEventRow{}
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// Cars — GET /api/v1/admin/cars
func (h *AdminHandler) Cars(w http.ResponseWriter, r *http.Request) {
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

	page, err := h.catalog.ListForAdmin(r.Context(), filter)
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

type moderateCarBody struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

// ModerateCar — POST /api/v1/admin/cars/{id}/moderate
func (h *AdminHandler) ModerateCar(w http.ResponseWriter, r *http.Request) {
	carID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body moderateCarBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	target := domain.CarDraft
	if body.Approve {
		target = domain.CarActive
	}

	if err := h.catalog.ChangeStatus(r.Context(), carID, viewerFrom(r), target, requestMeta(r)); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"status":  string(target),
		"approve": body.Approve,
	})
}
