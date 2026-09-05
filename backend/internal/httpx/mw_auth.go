package httpx

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/pkg/security"
)

// SessionRevoker сообщает, отозвана ли сессия.
//
// Access-токен живёт 15 минут и не проверяется по базе на каждый запрос —
// это был бы лишний запрос к PostgreSQL на каждое обращение. Но выход из
// аккаунта, блокировка пользователя и обнаружение кражи токена должны
// действовать немедленно, а не через 15 минут. Поэтому идентификаторы
// отозванных сессий лежат в Redis со сроком жизни, равным сроку жизни
// access-токена: одна проверка по ключу вместо запроса к базе.
type SessionRevoker interface {
	IsSessionRevoked(ctx context.Context, sessionID uuid.UUID) bool
}

// AuthMiddleware разбирает токен и наполняет контекст участником запроса.
type AuthMiddleware struct {
	Tokens  *security.TokenIssuer
	Revoker SessionRevoker
}

// Authenticate распознаёт пользователя, но не требует авторизации.
//
// Разделение с RequireAuth нужно для публичных страниц, которые ведут себя
// по-разному для гостя и вошедшего пользователя: каталог показывает
// признак «в избранном», а карточка дилера — кнопку заявки.
func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims, err := a.Tokens.ParseAccess(raw)
		if err != nil {
			// Истёкший токен — штатная ситуация: клиент обновит его через
			// refresh. Повреждённый токен разбирать дальше бессмысленно.
			if errors.Is(err, security.ErrTokenExpired) {
				Error(w, r, apierr.TokenExpired())
				return
			}
			Error(w, r, apierr.Unauthorized("Токен доступа недействителен"))
			return
		}

		if a.Revoker != nil && a.Revoker.IsSessionRevoked(r.Context(), claims.SessionID) {
			Error(w, r, apierr.Unauthorized("Сессия завершена, войдите заново"))
			return
		}

		role, err := domain.ParseRole(claims.Role)
		if err != nil {
			Error(w, r, apierr.Unauthorized("Токен доступа недействителен"))
			return
		}

		actor := Actor{
			UserID:        claims.UserID,
			SessionID:     claims.SessionID,
			Role:          role,
			EmailVerified: claims.EmailVerified,
			PhoneVerified: claims.PhoneVerified,
		}

		ctx := WithActor(r.Context(), actor)
		ctx = logging.WithUserID(ctx, actor.UserID.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth пропускает только аутентифицированные запросы.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ActorFrom(r.Context()).IsZero() {
			Error(w, r, apierr.Unauthorized(""))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole пропускает только перечисленные роли.
//
// Принцип запрета по умолчанию: доступ разрешён только тому, кто явно
// перечислен. Администратор не добавляется автоматически — там, где ему
// нужен доступ, он указывается явно.
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := make(map[domain.Role]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := ActorFrom(r.Context())
			if actor.IsZero() {
				Error(w, r, apierr.Unauthorized(""))
				return
			}
			if !allowed[actor.Role] {
				LoggerFrom(r.Context()).Warn("отказ по роли",
					slog.String("role", actor.Role.String()),
					slog.String("path", r.URL.Path))
				Error(w, r, apierr.Forbidden(""))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireVerified требует подтверждённой почты.
//
// Применяется к действиям, порождающим обязательства: создание заявки,
// публикация объявления, переписка. Просмотр каталога подтверждения не
// требует — иначе платформа теряет посетителей на пустом месте.
// SMS-подтверждение телефона отключено (платный провайдер не подключён).
func RequireVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFrom(r.Context())
		if actor.IsZero() {
			Error(w, r, apierr.Unauthorized(""))
			return
		}
		if !actor.EmailVerified {
			Error(w, r, &apierr.Error{
				Status:  http.StatusForbidden,
				Code:    apierr.CodeEmailNotVerified,
				Message: "Подтвердите адрес электронной почты, чтобы продолжить",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// --- Защита от подделки межсайтовых запросов --------------------------------

// CSRFCookieName — имя cookie с токеном двойной отправки.
const CSRFCookieName = "ai_csrf"

// CSRFHeaderName — заголовок, в котором клиент дублирует значение cookie.
const CSRFHeaderName = "X-CSRF-Token"

// CSRF реализует схему double submit cookie.
//
// Схема применяется к эндпоинтам, которые опираются на cookie: обновление
// и завершение сессии. Остальные вызовы API авторизуются заголовком
// Authorization, который сторонний сайт выставить не может, поэтому там
// подделка межсайтового запроса невозможна по построению.
//
// Значение cookie доступно скриптам того же origin, но недоступно чужому
// сайту из-за политики одного источника. Сравнение выполняется за
// постоянное время, чтобы не давать подсказок по времени ответа.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil || cookie.Value == "" {
			Error(w, r, apierr.Forbidden("Отсутствует токен защиты от подделки запроса"))
			return
		}

		header := r.Header.Get(CSRFHeaderName)
		if header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			Error(w, r, apierr.Forbidden("Токен защиты от подделки запроса не совпадает"))
			return
		}

		// Дополнительная проверка источника: браузеры всегда присылают
		// Origin для небезопасных методов.
		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r) {
			Error(w, r, apierr.Forbidden("Запрос отклонён: недопустимый источник"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func sameOrigin(origin string, r *http.Request) bool {
	trimmed := strings.TrimSuffix(origin, "/")
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return strings.EqualFold(trimmed, scheme+"://"+r.Host)
}
