package httpx

// Базовые middleware.
//
// Порядок подключения важен и зафиксирован в NewRouter: сначала то, что
// должно отработать для любого запроса, включая заведомо вредоносный
// (идентификатор запроса, определение адреса, проверка блокировки), затем
// разбор и лимиты, и только в конце аутентификация.

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/pkg/metrics"
)

// RequestIDMiddleware присваивает запросу идентификатор и возвращает его
// клиенту.
//
// Значение из заголовка принимается только от доверенного шлюза и жёстко
// ограничивается по длине и алфавиту: этот идентификатор попадает в логи,
// а лог с управляющими символами внутри — готовый вектор для подделки
// журнальных записей.
func RequestIDMiddleware(trustProxyHeader bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if trustProxyHeader {
				id = sanitizeRequestID(r.Header.Get("X-Request-Id"))
			}
			if id == "" {
				id = uuid.NewString()
			}

			w.Header().Set("X-Request-Id", id)

			ctx := WithRequestID(r.Context(), id)
			ctx = logging.WithRequestID(ctx, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sanitizeRequestID(raw string) string {
	if len(raw) == 0 || len(raw) > 64 {
		return ""
	}
	for _, r := range raw {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !isAllowed {
			return ""
		}
	}
	return raw
}

// RealIP определяет адрес клиента.
//
// Это не удобство, а основа всей защиты по адресам. X-Forwarded-For
// подделывается одной строкой в curl, поэтому заголовок читается только
// если само соединение пришло из доверенной подсети (шлюз nginx). Иначе
// берётся адрес TCP-соединения. Без этого правила ограничение частоты и
// блокировки обходятся тривиально.
func RealIP(trustedProxies []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteAddr(r)

			if len(trustedProxies) > 0 && isTrusted(ip, trustedProxies) {
				if forwarded := clientFromForwardedFor(r.Header.Get("X-Forwarded-For"), trustedProxies); forwarded.IsValid() {
					ip = forwarded
				} else if realIP, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-Ip"))); err == nil {
					ip = realIP.Unmap()
				}
			}

			ctx := WithClientIP(r.Context(), ip)
			ctx = logging.WithIP(ctx, ip.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func isTrusted(ip netip.Addr, trusted []*net.IPNet) bool {
	if !ip.IsValid() {
		return false
	}
	legacy := net.IP(ip.AsSlice())
	for _, network := range trusted {
		if network.Contains(legacy) {
			return true
		}
	}
	return false
}

// clientFromForwardedFor выбирает правый крайний недоверенный адрес.
//
// Цепочка X-Forwarded-For выглядит как «клиент, прокси1, прокси2», причём
// левую часть заполняет сам клиент и подделать её может как угодно.
// Доверять можно только тем звеньям справа, которые дописали наши прокси,
// поэтому идём с конца и берём первый адрес вне доверенных подсетей.
func clientFromForwardedFor(header string, trusted []*net.IPNet) netip.Addr {
	if header == "" {
		return netip.Addr{}
	}
	parts := strings.Split(header, ",")
	if len(parts) > 20 {
		// Аномально длинная цепочка — признак попытки запутать разбор.
		return netip.Addr{}
	}

	for i := len(parts) - 1; i >= 0; i-- {
		candidate, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(parts[i]), "[]"))
		if err != nil {
			continue
		}
		candidate = candidate.Unmap()
		if !isTrusted(candidate, trusted) {
			return candidate
		}
	}
	return netip.Addr{}
}

// Recovery перехватывает панику, не давая уронить весь процесс.
//
// Паника в одном обработчике не должна обрывать соединения остальных
// пользователей, а стек не должен попадать клиенту: это карта внутреннего
// устройства приложения.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// Разрыв соединения клиентом приходит как паника
				// http.ErrAbortHandler — это не ошибка приложения.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				LoggerFrom(r.Context()).Error("паника при обработке запроса",
					slog.Any("panic", rec),
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
					slog.String("stack", string(debug.Stack())),
				)
				Error(w, r, apierr.Internal(nil))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// WithBaseLogger кладёт в контекст логгер запроса.
func WithBaseLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithLogger(r.Context(), base)))
		})
	}
}

// responseRecorder запоминает код ответа и объём тела для логов и метрик.
type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap нужен, чтобы http.ResponseController мог добраться до исходного
// writer'а (например, для Flush в потоковой отдаче).
func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// AccessLog пишет одну строку на запрос и снимает метрики.
func AccessLog(registry *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			duration := time.Since(start)
			pattern := routePattern(r)

			if registry != nil {
				registry.ObserveRequest(r.Method, pattern, recorder.status, duration)
			}

			// Служебные проверки не засоряют журнал: они идут раз в несколько
			// секунд и не несут информации.
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				return
			}

			log := LoggerFrom(r.Context())
			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", pattern),
				slog.Int("status", recorder.status),
				slog.Int("bytes", recorder.bytes),
				slog.Duration("took", duration),
			}

			switch {
			case recorder.status >= 500:
				log.ErrorContext(r.Context(), "запрос завершён с ошибкой", attrs...)
			case recorder.status >= 400:
				log.WarnContext(r.Context(), "запрос отклонён", attrs...)
			default:
				log.InfoContext(r.Context(), "запрос обработан", attrs...)
			}
		})
	}
}

func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}

// SecurityHeaders выставляет заголовки безопасности на уровне приложения.
//
// Дублирование с nginx сделано намеренно: приложение может работать без
// шлюза (локальная разработка, отладочный стенд), и терять защиту в этом
// случае нельзя. Дублирующиеся заголовки браузер не смущают.
func SecurityHeaders(isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")

			// API отдаёт только JSON, поэтому политика максимально узкая:
			// исполнять и загружать что-либо с этих ответов запрещено полностью.
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")

			// Ответы API не должны попадать в кеш браузера или прокси:
			// они содержат персональные данные.
			h.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			h.Set("Pragma", "no-cache")

			if isProduction {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS разрешает запросы только с перечисленных источников.
//
// Подстановочный * не используется никогда: вместе с cookie-сессией это
// означало бы, что любой сайт может выполнять запросы от имени
// авторизованного пользователя.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimRight(origin, "/"))] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[strings.ToLower(strings.TrimRight(origin, "/"))] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers",
					"Content-Type, Authorization, X-CSRF-Token, X-Requested-With, Idempotency-Key")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After, ETag, Idempotent-Replay")
				h.Set("Access-Control-Max-Age", "600")
				h.Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimit ограничивает размер тела запроса.
//
// Без этого ограничения один клиент может занять память процесса,
// отправляя гигабайтный JSON: сервер честно попытается его прочитать.
func BodyLimit(maxBytes, uploadBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := maxBytes
			if strings.HasPrefix(r.URL.Path, "/api/v1/uploads") {
				limit = uploadBytes
			}
			if r.ContentLength > 0 && r.ContentLength > limit {
				Error(w, r, apierr.PayloadTooLarge(""))
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
