// Package logging настраивает структурное логирование.
//
// Логи проходят через обработчик, который маскирует персональные данные.
// Это не косметика: журналы уезжают в системы сбора логов, копируются в
// бэкапы и попадают в тикеты поддержки, а email, телефон и токен в открытом
// виде превращают любую утечку логов в утечку персональных данных.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Ключи контекста для сквозных полей запроса.
type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyUserID
	ctxKeyIP
)

// WithRequestID кладёт идентификатор запроса в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// WithUserID кладёт идентификатор пользователя в контекст.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, id)
}

// WithIP кладёт адрес клиента в контекст.
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ctxKeyIP, ip)
}

// RequestID возвращает идентификатор запроса из контекста.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// sensitiveKeys — поля, значение которых никогда не попадает в лог целиком.
var sensitiveKeys = map[string]bool{
	"password":           true,
	"password_hash":      true,
	"old_password":       true,
	"new_password":       true,
	"token":              true,
	"access_token":       true,
	"refresh_token":      true,
	"code":               true,
	"code_hash":          true,
	"secret":             true,
	"authorization":      true,
	"cookie":             true,
	"set-cookie":         true,
	"jwt":                true,
	"passport":           true,
	"passport_encrypted": true,
	"pii":                true,
	"card":               true,
	"vin":                true,
	"captcha_token":      true,
}

// partialKeys — поля, которые логируются в усечённом виде: для разбора
// инцидентов достаточно узнать, о каком адресе речь, без полного значения.
var partialKeys = map[string]bool{
	"email":       true,
	"phone":       true,
	"destination": true,
	"address":     true,
	"full_name":   true,
}

// New создаёт логгер с маскированием чувствительных полей.
func New(level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:       parseLevel(level),
		ReplaceAttr: redact,
	}

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(&contextHandler{Handler: handler})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func redact(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)

	if sensitiveKeys[key] {
		return slog.String(a.Key, "[REDACTED]")
	}
	if partialKeys[key] && a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, Mask(a.Value.String()))
	}
	return a
}

// Mask оставляет от значения ровно столько, чтобы человек мог сопоставить
// запись с обращением пользователя, но не восстановить сам контакт.
//
//	ivanov@mail.ru  -> iv***@mail.ru
//	+79141234567    -> +7914***4567
func Mask(v string) string {
	if v == "" {
		return ""
	}
	if at := strings.LastIndex(v, "@"); at > 0 {
		local := v[:at]
		domain := v[at:]
		if len(local) <= 2 {
			return "***" + domain
		}
		return local[:2] + "***" + domain
	}
	runes := []rune(v)
	if len(runes) <= 4 {
		return "***"
	}
	if len(runes) <= 8 {
		return string(runes[:2]) + "***"
	}
	return string(runes[:4]) + "***" + string(runes[len(runes)-4:])
}

// contextHandler автоматически добавляет к каждой записи request_id, user_id
// и адрес клиента, если они есть в контексте. Без этого каждый вызов
// логгера пришлось бы вручную снабжать одними и теми же полями.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok && v != "" {
		r.AddAttrs(slog.String("request_id", v))
	}
	if v, ok := ctx.Value(ctxKeyUserID).(string); ok && v != "" {
		r.AddAttrs(slog.String("user_id", v))
	}
	if v, ok := ctx.Value(ctxKeyIP).(string); ok && v != "" {
		r.AddAttrs(slog.String("ip", v))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}
