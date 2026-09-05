package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
)

// Admin — выборки для административной панели.
type Admin struct {
	pool *Pool
}

func NewAdmin(pool *Pool) *Admin { return &Admin{pool: pool} }

// UserFilter — параметры поиска пользователей.
type UserFilter struct {
	Roles    []string
	Statuses []string
	Search   string
	// UnverifiedOnly показывает тех, кто не подтвердил контакты.
	UnverifiedOnly bool

	Limit  int
	Offset int
}

// UserRow — строка списка пользователей.
type UserRow struct {
	ID       uuid.UUID   `json:"id"`
	Email    string      `json:"email"`
	Phone    string      `json:"phone"`
	FullName string      `json:"full_name"`
	Role     domain.Role `json:"role"`
	Status   string      `json:"status"`

	EmailVerified bool `json:"email_verified"`
	PhoneVerified bool `json:"phone_verified"`

	DealsCount    int `json:"deals_count"`
	RequestsCount int `json:"requests_count"`
	CarsCount     int `json:"cars_count"`

	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Users возвращает страницу списка пользователей.
func (a *Admin) Users(ctx context.Context, filter UserFilter) ([]UserRow, int, error) {
	builder := &argBuilder{}
	where := make([]string, 0, 5)

	if len(filter.Roles) > 0 {
		where = append(where, fmt.Sprintf("users.role = ANY(%s::user_role[])", builder.add(filter.Roles)))
	}
	if len(filter.Statuses) > 0 {
		where = append(where, fmt.Sprintf("users.status = ANY(%s::user_status[])", builder.add(filter.Statuses)))
	}
	if filter.UnverifiedOnly {
		where = append(where, "users.email_verified_at IS NULL")
	}
	if filter.Search != "" {
		// Поиск идёт по имени, почте и телефону сразу: администратор
		// приходит с одной строкой из обращения и не должен угадывать,
		// в какое поле её вставить.
		pattern := builder.add("%" + filter.Search + "%")
		where = append(where, fmt.Sprintf(
			"(users.full_name ILIKE %s OR users.email::text ILIKE %s OR users.phone ILIKE %s)",
			pattern, pattern, pattern))
	}

	whereSQL := "true"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var total int
	if err := a.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE `+whereSQL, builder.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт пользователей: %w", err)
	}

	rows, err := a.pool.Query(ctx, fmt.Sprintf(`
		SELECT users.id, users.email::text, users.phone, users.full_name,
		       users.role, users.status,
		       users.email_verified_at IS NOT NULL,
		       users.phone_verified_at IS NOT NULL,
		       COALESCE(stats.deals, 0), COALESCE(stats.requests, 0), COALESCE(stats.cars, 0),
		       users.last_login_at, users.created_at
		FROM users
		LEFT JOIN LATERAL (
			SELECT
				(SELECT count(*) FROM deals
				  WHERE deals.client_id = users.id OR deals.dealer_id = users.id) AS deals,
				(SELECT count(*) FROM requests
				  WHERE requests.client_id = users.id OR requests.dealer_id = users.id) AS requests,
				(SELECT count(*) FROM cars WHERE cars.dealer_id = users.id) AS cars
		) AS stats ON true
		WHERE %s
		ORDER BY users.created_at DESC, users.id DESC
		LIMIT %s OFFSET %s`,
		whereSQL, builder.add(limit), builder.add(filter.Offset)), builder.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("выборка пользователей: %w", err)
	}
	defer rows.Close()

	var out []UserRow
	for rows.Next() {
		var row UserRow
		if err := rows.Scan(&row.ID, &row.Email, &row.Phone, &row.FullName,
			&row.Role, &row.Status, &row.EmailVerified, &row.PhoneVerified,
			&row.DealsCount, &row.RequestsCount, &row.CarsCount,
			&row.LastLoginAt, &row.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("разбор пользователя: %w", err)
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// AuditFilter — параметры выборки журнала действий.
type AuditFilter struct {
	ActorID *uuid.UUID
	Entity  string
	Action  string
	Since   *time.Time

	Limit  int
	Offset int
}

// AuditRow — запись журнала действий.
type AuditRow struct {
	ID        int64          `json:"id"`
	ActorID   *uuid.UUID     `json:"actor_id,omitempty"`
	ActorName string         `json:"actor_name,omitempty"`
	ActorRole string         `json:"actor_role,omitempty"`
	Action    string         `json:"action"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entity_id,omitempty"`
	Diff      map[string]any `json:"diff,omitempty"`
	IP        string         `json:"ip,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// AuditLog возвращает журнал действий.
func (a *Admin) AuditLog(ctx context.Context, filter AuditFilter) ([]AuditRow, error) {
	builder := &argBuilder{}
	where := make([]string, 0, 4)

	if filter.ActorID != nil {
		where = append(where, "audit_log.actor_id = "+builder.add(*filter.ActorID))
	}
	if filter.Entity != "" {
		where = append(where, "audit_log.entity = "+builder.add(filter.Entity))
	}
	if filter.Action != "" {
		where = append(where, "audit_log.action = "+builder.add(filter.Action))
	}
	if filter.Since != nil {
		where = append(where, "audit_log.created_at >= "+builder.add(*filter.Since))
	}

	whereSQL := "true"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := a.pool.Query(ctx, fmt.Sprintf(`
		SELECT audit_log.id, audit_log.actor_id, COALESCE(users.full_name, ''),
		       COALESCE(audit_log.actor_role::text, ''),
		       audit_log.action, audit_log.entity, COALESCE(audit_log.entity_id, ''),
		       audit_log.diff, COALESCE(audit_log.ip::text, ''),
		       COALESCE(audit_log.request_id, ''), audit_log.created_at
		FROM audit_log
		LEFT JOIN users ON users.id = audit_log.actor_id
		WHERE %s
		ORDER BY audit_log.id DESC
		LIMIT %s OFFSET %s`,
		whereSQL, builder.add(limit), builder.add(filter.Offset)), builder.args...)
	if err != nil {
		return nil, fmt.Errorf("выборка журнала действий: %w", err)
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var (
			row  AuditRow
			diff []byte
		)
		if err := rows.Scan(&row.ID, &row.ActorID, &row.ActorName, &row.ActorRole,
			&row.Action, &row.Entity, &row.EntityID, &diff, &row.IP,
			&row.RequestID, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор записи журнала: %w", err)
		}
		if len(diff) > 0 {
			_ = json.Unmarshal(diff, &row.Diff)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SecurityEventRow — событие безопасности.
type SecurityEventRow struct {
	ID        int64          `json:"id"`
	Kind      string         `json:"kind"`
	Severity  int            `json:"severity"`
	UserID    *uuid.UUID     `json:"user_id,omitempty"`
	UserName  string         `json:"user_name,omitempty"`
	IP        string         `json:"ip,omitempty"`
	Route     string         `json:"route,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// SecurityEvents возвращает события безопасности.
func (a *Admin) SecurityEvents(ctx context.Context, kinds []string, minSeverity, limit, offset int) ([]SecurityEventRow, error) {
	builder := &argBuilder{}
	where := []string{"security_events.severity >= " + builder.add(minSeverity)}

	if len(kinds) > 0 {
		where = append(where, "security_events.kind = ANY("+builder.add(kinds)+")")
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := a.pool.Query(ctx, fmt.Sprintf(`
		SELECT security_events.id, security_events.kind, security_events.severity,
		       security_events.user_id, COALESCE(users.full_name, ''),
		       COALESCE(security_events.ip::text, ''), COALESCE(security_events.route, ''),
		       security_events.details, security_events.created_at
		FROM security_events
		LEFT JOIN users ON users.id = security_events.user_id
		WHERE %s
		ORDER BY security_events.id DESC
		LIMIT %s OFFSET %s`,
		strings.Join(where, " AND "), builder.add(limit), builder.add(offset)), builder.args...)
	if err != nil {
		return nil, fmt.Errorf("выборка событий безопасности: %w", err)
	}
	defer rows.Close()

	var out []SecurityEventRow
	for rows.Next() {
		var (
			row     SecurityEventRow
			details []byte
		)
		if err := rows.Scan(&row.ID, &row.Kind, &row.Severity, &row.UserID, &row.UserName,
			&row.IP, &row.Route, &details, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор события безопасности: %w", err)
		}
		if len(details) > 0 {
			_ = json.Unmarshal(details, &row.Details)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Overview — сводка для главной страницы админки.
type Overview struct {
	UsersTotal   int `json:"users_total"`
	UsersNew7d   int `json:"users_new_7d"`
	DealersTotal int `json:"dealers_total"`

	CarsActive     int `json:"cars_active"`
	CarsModeration int `json:"cars_moderation"`

	RequestsOpen int `json:"requests_open"`
	DealsOpen    int `json:"deals_open"`
	DealsWon30d  int `json:"deals_won_30d"`

	BannersModeration int `json:"banners_moderation"`
	SellersPending    int `json:"sellers_pending"`

	SecurityEvents24h int `json:"security_events_24h"`
	BlockedIPs        int `json:"blocked_ips"`
}

// Overview собирает сводку одним запросом.
//
// Все показатели считаются в одном обращении подзапросами: дюжина
// отдельных запросов на открытие панели заняла бы дюжину соединений пула
// ради данных, которые всё равно показываются вместе.
func (a *Admin) Overview(ctx context.Context) (*Overview, error) {
	var overview Overview

	err := a.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM users WHERE created_at > now() - interval '7 days'),
			(SELECT count(*) FROM users WHERE role = 'dealer'),
			(SELECT count(*) FROM cars WHERE status = 'active'),
			(SELECT count(*) FROM cars WHERE status = 'moderation'),
			(SELECT count(*) FROM requests WHERE status IN ('new', 'in_progress', 'answered')),
			(SELECT count(*) FROM deals WHERE outcome = 'open'),
			(SELECT count(*) FROM deals WHERE outcome = 'won' AND closed_at > now() - interval '30 days'),
			(SELECT count(*) FROM banners WHERE status = 'moderation'),
			(SELECT count(*) FROM sellers WHERE verified_at IS NULL AND is_active),
			(SELECT count(*) FROM security_events WHERE created_at > now() - interval '24 hours' AND severity >= 2),
			(SELECT count(*) FROM ip_blocks WHERE permanent OR expires_at > now())`,
	).Scan(
		&overview.UsersTotal, &overview.UsersNew7d, &overview.DealersTotal,
		&overview.CarsActive, &overview.CarsModeration,
		&overview.RequestsOpen, &overview.DealsOpen, &overview.DealsWon30d,
		&overview.BannersModeration, &overview.SellersPending,
		&overview.SecurityEvents24h, &overview.BlockedIPs,
	)
	if err != nil {
		return nil, fmt.Errorf("сводка администратора: %w", err)
	}
	return &overview, nil
}

// SetUserStatus меняет состояние учётной записи.
func (a *Admin) SetUserStatus(ctx context.Context, userID uuid.UUID, status domain.UserStatus) error {
	tag, err := a.pool.Exec(ctx, `
		UPDATE users SET
			status = $2,
			email_verified_at = CASE
				WHEN $2 = 'active' THEN COALESCE(email_verified_at, now())
				ELSE email_verified_at
			END,
			phone_verified_at = CASE
				WHEN $2 = 'active' THEN COALESCE(phone_verified_at, now())
				ELSE phone_verified_at
			END
		WHERE id = $1`, userID, status)
	if err != nil {
		return fmt.Errorf("изменение состояния пользователя: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserRole меняет роль пользователя.
func (a *Admin) SetUserRole(ctx context.Context, userID uuid.UUID, role domain.Role) error {
	tag, err := a.pool.Exec(ctx,
		`UPDATE users SET role = $2 WHERE id = $1`, userID, role)
	if err != nil {
		return fmt.Errorf("изменение роли пользователя: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
