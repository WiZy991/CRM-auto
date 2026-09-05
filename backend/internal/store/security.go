package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SecurityLog — журнал событий безопасности и аудита.
//
// Записи пишутся вне транзакции основного действия: если аудит не удался,
// само действие пользователя отменять нельзя, но потерять след нежелательно,
// поэтому ошибка записи логируется как ошибка приложения.
type SecurityLog struct {
	pool *Pool
}

func NewSecurityLog(pool *Pool) *SecurityLog { return &SecurityLog{pool: pool} }

// Виды событий безопасности.
const (
	EventLoginSuccess     = "login_success"
	EventLoginFailed      = "login_failed"
	EventAccountLocked    = "account_locked"
	EventRegistration     = "registration"
	EventPasswordChanged  = "password_changed"
	EventTokenReuse       = "refresh_token_reuse"
	EventLogout           = "logout"
	EventLogoutAll        = "logout_all_devices"
	EventVerifyCodeSent   = "verify_code_sent"
	EventVerifyCodeFailed = "verify_code_failed"
	EventVerifySuccess    = "verify_success"
	EventRateLimited      = "rate_limited"
	EventIPBanned         = "ip_banned"
	EventSuspiciousPath   = "suspicious_path"
	EventForbiddenAccess  = "forbidden_access"
	EventProfileUpdated   = "profile_updated"
	EventAccountDeleted   = "account_deleted"
)

// SecurityEvent — одно событие безопасности.
type SecurityEvent struct {
	Kind      string
	Severity  int
	UserID    *uuid.UUID
	IP        string
	UserAgent string
	Route     string
	Details   map[string]any
}

// RecordEvent пишет событие безопасности.
func (s *SecurityLog) RecordEvent(ctx context.Context, event SecurityEvent) error {
	if event.Severity == 0 {
		event.Severity = 1
	}

	details := []byte("{}")
	if len(event.Details) > 0 {
		encoded, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("сериализация подробностей события: %w", err)
		}
		details = encoded
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO security_events (kind, severity, user_id, ip, user_agent, route, details)
		VALUES ($1, $2, $3, NULLIF($4, '')::inet, $5, $6, $7)`,
		event.Kind, event.Severity, event.UserID, event.IP,
		truncate(event.UserAgent, 400), truncate(event.Route, 200), details)
	if err != nil {
		return fmt.Errorf("запись события безопасности: %w", err)
	}
	return nil
}

// AuditEntry — запись аудита изменений.
type AuditEntry struct {
	ActorID   *uuid.UUID
	ActorRole string
	Action    string
	Entity    string
	EntityID  string
	Diff      map[string]any
	IP        string
	UserAgent string
	RequestID string
}

// RecordAudit пишет запись аудита.
func (s *SecurityLog) RecordAudit(ctx context.Context, entry AuditEntry) error {
	diff := []byte("{}")
	if len(entry.Diff) > 0 {
		encoded, err := json.Marshal(entry.Diff)
		if err != nil {
			return fmt.Errorf("сериализация изменений: %w", err)
		}
		diff = encoded
	}

	var role any
	if entry.ActorRole != "" {
		role = entry.ActorRole
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log
		    (actor_id, actor_role, action, entity, entity_id, diff, ip, user_agent, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::inet, $8, $9)`,
		entry.ActorID, role, entry.Action, entry.Entity, nullIfEmpty(entry.EntityID),
		diff, entry.IP, truncate(entry.UserAgent, 400), nullIfEmpty(entry.RequestID))
	if err != nil {
		return fmt.Errorf("запись аудита: %w", err)
	}
	return nil
}

// CountRecentEvents считает события одного вида с адреса за период.
// Используется, чтобы подтвердить или опровергнуть подозрение перед
// постоянной блокировкой.
func (s *SecurityLog) CountRecentEvents(ctx context.Context, ip, kind string, window time.Duration) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM security_events
		WHERE ip = $1::inet AND kind = $2 AND created_at > now() - $3::interval`,
		ip, kind, window).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("подсчёт событий безопасности: %w", err)
	}
	return count, nil
}

// PersistIPBlock сохраняет блокировку адреса в базу.
//
// Основной механизм блокировки живёт в Redis, но постоянные и длительные
// баны дублируются в базу: они должны переживать перезапуск и очистку кеша
// и быть видны администратору в отчёте.
func (s *SecurityLog) PersistIPBlock(ctx context.Context, ip, reason string, expiresAt *time.Time, permanent bool, createdBy *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ip_blocks (ip, reason, expires_at, permanent, created_by)
		VALUES ($1::inet, $2, $3, $4, $5)
		ON CONFLICT (ip) DO UPDATE
		SET reason = excluded.reason,
		    hits = ip_blocks.hits + 1,
		    blocked_at = now(),
		    expires_at = excluded.expires_at,
		    permanent = ip_blocks.permanent OR excluded.permanent`,
		ip, reason, expiresAt, permanent, createdBy)
	if err != nil {
		return fmt.Errorf("сохранение блокировки адреса: %w", err)
	}
	return nil
}

// ActiveIPBlocks возвращает действующие блокировки для восстановления
// состояния в Redis после перезапуска.
func (s *SecurityLog) ActiveIPBlocks(ctx context.Context) ([]IPBlock, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ip::text, reason, expires_at, permanent
		FROM ip_blocks
		WHERE permanent OR (expires_at IS NOT NULL AND expires_at > now())
		ORDER BY blocked_at DESC
		LIMIT 10000`)
	if err != nil {
		return nil, fmt.Errorf("чтение блокировок адресов: %w", err)
	}
	defer rows.Close()

	var out []IPBlock
	for rows.Next() {
		var block IPBlock
		if err := rows.Scan(&block.IP, &block.Reason, &block.ExpiresAt, &block.Permanent); err != nil {
			return nil, fmt.Errorf("разбор блокировки: %w", err)
		}
		out = append(out, block)
	}
	return out, rows.Err()
}

// IPBlock — постоянная запись о блокировке адреса.
type IPBlock struct {
	IP        string
	Reason    string
	ExpiresAt *time.Time
	Permanent bool
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
