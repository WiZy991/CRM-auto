package httpx

import (
	"context"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
)

type contextKey int

const (
	ctxKeyLogger contextKey = iota
	ctxKeyActor
	ctxKeyClientIP
	ctxKeyRequestID
	ctxKeyRoutePattern
)

// Actor — аутентифицированный участник запроса.
//
// Хранится в контексте, чтобы обработчики не разбирали токен повторно.
// Важно: наличие Actor означает только «токен валиден». Право на конкретное
// действие проверяется отдельно, ближе к данным.
type Actor struct {
	UserID        uuid.UUID
	SessionID     uuid.UUID
	Role          domain.Role
	EmailVerified bool
	PhoneVerified bool
}

// IsZero сообщает, что запрос анонимный.
func (a Actor) IsZero() bool { return a.UserID == uuid.Nil }

// WithLogger кладёт логгер запроса в контекст.
func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger, log)
}

// LoggerFrom достаёт логгер запроса. Всегда возвращает рабочий логгер.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}

// WithActor кладёт участника в контекст.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, ctxKeyActor, actor)
}

// ActorFrom достаёт участника из контекста.
func ActorFrom(ctx context.Context) Actor {
	if actor, ok := ctx.Value(ctxKeyActor).(Actor); ok {
		return actor
	}
	return Actor{}
}

// WithClientIP кладёт вычисленный адрес клиента в контекст.
func WithClientIP(ctx context.Context, ip netip.Addr) context.Context {
	return context.WithValue(ctx, ctxKeyClientIP, ip)
}

// ClientIPFrom достаёт адрес клиента.
func ClientIPFrom(ctx context.Context) netip.Addr {
	if ip, ok := ctx.Value(ctxKeyClientIP).(netip.Addr); ok {
		return ip
	}
	return netip.Addr{}
}

// ClientIPString отдаёт адрес строкой (для ключей Redis и записи в базу).
func ClientIPString(ctx context.Context) string {
	if ip := ClientIPFrom(ctx); ip.IsValid() {
		return ip.String()
	}
	return "unknown"
}

// WithRequestID кладёт идентификатор запроса в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFrom достаёт идентификатор запроса.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID).(string)
	return id
}

// WithRoutePattern сохраняет шаблон маршрута для метрик и логов.
func WithRoutePattern(ctx context.Context, pattern string) context.Context {
	return context.WithValue(ctx, ctxKeyRoutePattern, pattern)
}

// RoutePatternFrom достаёт шаблон маршрута.
func RoutePatternFrom(ctx context.Context) string {
	pattern, _ := ctx.Value(ctxKeyRoutePattern).(string)
	return pattern
}
