package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// Admin — сценарии административной панели.
type Admin struct {
	admin    *store.Admin
	sessions *store.Sessions
	audit    *store.SecurityLog
	revoker  *SessionRevoker
	log      *slog.Logger
}

func NewAdmin(
	admin *store.Admin,
	sessions *store.Sessions,
	audit *store.SecurityLog,
	revoker *SessionRevoker,
	log *slog.Logger,
) *Admin {
	return &Admin{admin: admin, sessions: sessions, audit: audit, revoker: revoker, log: log}
}

// Overview возвращает сводку для главной страницы панели.
func (a *Admin) Overview(ctx context.Context) (*store.Overview, error) {
	overview, err := a.admin.Overview(ctx)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return overview, nil
}

// Users возвращает список пользователей.
func (a *Admin) Users(ctx context.Context, filter store.UserFilter) ([]store.UserRow, int, error) {
	rows, total, err := a.admin.Users(ctx, filter)
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}
	return rows, total, nil
}

// AuditLog возвращает журнал действий.
func (a *Admin) AuditLog(ctx context.Context, filter store.AuditFilter) ([]store.AuditRow, error) {
	rows, err := a.admin.AuditLog(ctx, filter)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return rows, nil
}

// SecurityEvents возвращает события безопасности.
func (a *Admin) SecurityEvents(ctx context.Context, kinds []string, minSeverity, limit, offset int) ([]store.SecurityEventRow, error) {
	rows, err := a.admin.SecurityEvents(ctx, kinds, minSeverity, limit, offset)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return rows, nil
}

// SetUserStatus меняет состояние учётной записи.
//
// Блокировка немедленно обрывает действующие сессии: без этого выданный ранее
// access-токен продолжает действовать до истечения срока, и заблокированный
// пользователь ещё четверть часа работает как ни в чём не бывало.
func (a *Admin) SetUserStatus(ctx context.Context, targetID uuid.UUID, admin Viewer, rawStatus string) error {
	status := domain.UserStatus(normalizeEnum(rawStatus))
	if !status.Valid() {
		return apierr.BadRequest("Неизвестное состояние учётной записи")
	}

	// Администратор не может заблокировать сам себя: иначе одна опечатка
	// оставляет платформу без доступа к панели управления.
	if targetID == admin.UserID {
		return apierr.BadRequest("Нельзя изменить состояние собственной учётной записи")
	}

	if err := a.admin.SetUserStatus(ctx, targetID, status); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Пользователь")
		}
		return apierr.Internal(err)
	}

	if status != domain.StatusActive {
		a.dropSessions(ctx, targetID, "статус учётной записи изменён администратором")
	}

	a.recordAudit(ctx, admin, "user.status", targetID.String(),
		map[string]any{"status": string(status)})
	return nil
}

// SetUserRole меняет роль пользователя.
func (a *Admin) SetUserRole(ctx context.Context, targetID uuid.UUID, admin Viewer, rawRole string) error {
	role, err := domain.ParseRole(rawRole)
	if err != nil {
		return apierr.BadRequest(err.Error())
	}

	if targetID == admin.UserID {
		return apierr.BadRequest("Нельзя изменить собственную роль")
	}

	if err := a.admin.SetUserRole(ctx, targetID, role); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Пользователь")
		}
		return apierr.Internal(err)
	}

	// Роль зашита в access-токен, поэтому старые токены надо погасить:
	// иначе понижение прав вступит в силу только после их истечения.
	a.dropSessions(ctx, targetID, "роль изменена администратором")

	a.recordAudit(ctx, admin, "user.role", targetID.String(),
		map[string]any{"role": string(role)})
	return nil
}

// dropSessions завершает все сессии пользователя.
//
// Отзыв идёт в два места: refresh-сессии гасятся в базе, а их
// идентификаторы попадают в список отозванных в Redis. Только базы мало —
// уже выданный access-токен проверяется по подписи и продолжал бы
// действовать до истечения срока.
func (a *Admin) dropSessions(ctx context.Context, userID uuid.UUID, reason string) {
	sessionIDs, err := a.sessions.RevokeAllForUser(ctx, userID, reason)
	if err != nil {
		a.log.ErrorContext(ctx, "не удалось завершить сессии пользователя", "error", err)
		return
	}
	if err := a.revoker.RevokeMany(ctx, sessionIDs); err != nil {
		a.log.ErrorContext(ctx, "не удалось отметить сессии отозванными", "error", err)
	}
}

func (a *Admin) recordAudit(ctx context.Context, admin Viewer, action, entityID string, diff map[string]any) {
	actorID := admin.UserID
	if err := a.audit.RecordAudit(ctx, store.AuditEntry{
		ActorID:   &actorID,
		ActorRole: string(admin.Role),
		Action:    action,
		Entity:    "user",
		EntityID:  entityID,
		Diff:      diff,
	}); err != nil {
		a.log.ErrorContext(ctx, "не удалось записать аудит администратора", "error", err, "action", action)
	}
}
