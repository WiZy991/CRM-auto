package httpx

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/metrics"
	"github.com/autoimport/crm/internal/pkg/ratelimit"
)

// Guard объединяет проверки, которым нужен доступ к Redis и метрикам.
type Guard struct {
	Limiter   *ratelimit.Limiter
	Blocklist *ratelimit.Blocklist
	Metrics   *metrics.Registry
	Enabled   bool
	Log       *slog.Logger
	Redis     redis.UniversalClient
}

// --- Проверка блокировки ----------------------------------------------------

// BanCheck отклоняет запросы с заблокированных адресов.
//
// Стоит первым в цепочке после определения адреса: заблокированный клиент
// не должен доходить ни до разбора тела, ни до базы. Именно это делает
// блокировку дешёвой для нас и дорогой для атакующего.
func (g *Guard) BanCheck() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.Enabled || g.Blocklist == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := ClientIPString(r.Context())
			banned, ttl := g.Blocklist.IsBanned(r.Context(), ip)
			if !banned {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Retry-After", strconv.Itoa(int(ttl.Seconds())+1))
			Error(w, r, apierr.TooManyRequests(
				"Доступ временно ограничен из-за подозрительной активности"))
		})
	}
}

// --- WAF-lite ---------------------------------------------------------------

// scannerPaths — пути, которые запрашивают только автоматические сканеры.
// Живой пользователь платформы по ним не ходит, поэтому любое обращение
// считается подозрительным с большим весом.
var scannerPaths = []string{
	"/.env", "/.git", "/.svn", "/.aws", "/.ssh",
	"/wp-admin", "/wp-login", "/wordpress", "/xmlrpc.php",
	"/phpmyadmin", "/pma", "/adminer", "/phpinfo",
	"/vendor/phpunit", "/cgi-bin", "/actuator", "/solr",
	"/config.json", "/backup.sql", "/dump.sql", "/.DS_Store",
	"/server-status", "/druid", "/jenkins", "/manager/html",
}

// suspiciousPatterns — признаки попыток обхода в пути или строке запроса.
var suspiciousPatterns = []string{
	"../", "..\\", "%2e%2e", "%252e",
	"<script", "%3cscript", "javascript:",
	"union select", "union+select", "' or '1'='1", "\" or \"1\"=\"1",
	"/etc/passwd", "c:\\windows", "cmd.exe", "/bin/sh",
	"${jndi:", "{{", "<%=",
}

// WAF отсекает очевидно вредоносные запросы до попадания в приложение.
//
// Это намеренно не полноценный WAF: сигнатурный фильтр не защищает от
// целевой атаки и не заменяет параметризованные запросы, санитизацию и
// проверку прав. Его задача — дёшево отбросить массовый автоматический шум
// и быстро набрать вес для блокировки такого адреса.
func (g *Guard) WAF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.ToLower(r.URL.Path)
			query := strings.ToLower(r.URL.RawQuery)

			// Аномально длинный URL: обычные маршруты платформы короткие,
			// а разбор гигантских строк — это работа впустую.
			if len(r.URL.Path) > 512 || len(r.URL.RawQuery) > 2048 {
				g.punish(r, "oversized_url", 5)
				Error(w, r, apierr.BadRequest("Некорректный запрос"))
				return
			}

			for _, scanner := range scannerPaths {
				if strings.HasPrefix(path, scanner) || strings.Contains(path, scanner+"/") {
					g.punish(r, "scanner_path", 10)
					// Ответ 404 без подробностей: сканеру не сообщается,
					// что его распознали.
					Error(w, r, apierr.NotFound("Ресурс"))
					return
				}
			}

			combined := path + "?" + query
			for _, pattern := range suspiciousPatterns {
				if strings.Contains(combined, pattern) {
					g.punish(r, "injection_attempt", 8)
					Error(w, r, apierr.BadRequest("Некорректный запрос"))
					return
				}
			}

			// Заголовок Host обязателен и должен быть разумной длины:
			// пустой или гигантский Host встречается только у ботов.
			if r.Host == "" || len(r.Host) > 253 {
				g.punish(r, "bad_host", 5)
				Error(w, r, apierr.BadRequest("Некорректный запрос"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// punish регистрирует подозрительное событие и, при превышении порога,
// блокирует адрес.
func (g *Guard) punish(r *http.Request, kind string, weight int) {
	if g.Metrics != nil {
		g.Metrics.ObserveSuspicious(kind)
	}
	if !g.Enabled || g.Blocklist == nil {
		return
	}

	ctx := r.Context()
	ip := ClientIPString(ctx)

	verdict, err := g.Blocklist.RegisterSuspicious(ctx, ip, weight)
	if err != nil {
		LoggerFrom(ctx).Error("не удалось зарегистрировать подозрительное событие",
			slog.String("error", err.Error()))
		return
	}

	log := LoggerFrom(ctx)
	if verdict.Banned {
		if g.Metrics != nil {
			g.Metrics.ObserveBan()
		}
		log.WarnContext(ctx, "адрес заблокирован",
			slog.String("kind", kind),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", verdict.Duration),
		)
		return
	}
	log.InfoContext(ctx, "подозрительный запрос",
		slog.String("kind", kind),
		slog.String("path", r.URL.Path),
		slog.Int("score", verdict.Score),
	)
}

// PunishStatus регистрирует подозрительным сам факт ответа 401/403/404.
//
// Смысл в накоплении: одна ошибка авторизации ничего не значит, а сто
// ошибок с одного адреса за пять минут — это перебор паролей или обход
// идентификаторов. Порог и вес разделяют эти два случая.
func (g *Guard) PunishStatus() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)

			switch recorder.status {
			case http.StatusUnauthorized, http.StatusForbidden:
				g.punish(r, "auth_rejected", 2)
			case http.StatusNotFound:
				g.punish(r, "not_found", 1)
			}
		})
	}
}

// --- Ограничение частоты ----------------------------------------------------

// RateLimit применяет правило к запросу, определяя клиента по адресу.
func (g *Guard) RateLimit(rule ratelimit.Rule) func(http.Handler) http.Handler {
	return g.rateLimitBy(rule, func(r *http.Request) string {
		return ClientIPString(r.Context())
	})
}

// RateLimitByUser применяет правило к авторизованному пользователю.
//
// Отдельный лимит на пользователя нужен потому, что адрес — плохой
// идентификатор: за одним IP может сидеть весь офис дилера, а один
// злоумышленник легко меняет адреса. Поэтому лимиты работают в паре.
func (g *Guard) RateLimitByUser(rule ratelimit.Rule) func(http.Handler) http.Handler {
	return g.rateLimitBy(rule, func(r *http.Request) string {
		actor := ActorFrom(r.Context())
		if actor.IsZero() {
			return "anon:" + ClientIPString(r.Context())
		}
		return "user:" + actor.UserID.String()
	})
}

func (g *Guard) rateLimitBy(rule ratelimit.Rule, identity func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.Enabled || g.Limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			decision, err := g.Limiter.Allow(ctx, rule, identity(r))
			if err != nil {
				// Redis недоступен: запрос пропускается, но факт фиксируется
				// как ошибка — на это должен быть настроен алерт.
				LoggerFrom(ctx).Error("ограничитель частоты недоступен",
					slog.String("rule", rule.Name),
					slog.String("error", err.Error()))
			}

			if decision.Allowed {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rule.Limit))
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
				next.ServeHTTP(w, r)
				return
			}

			if g.Metrics != nil {
				g.Metrics.ObserveRateLimit(rule.Name)
			}
			// Исчерпание лимита само является подозрительным событием:
			// так методичный перебор доходит до блокировки адреса.
			g.punish(r, "rate_limited", 3)

			w.Header().Set("Retry-After", strconv.Itoa(int(decision.RetryAfter.Seconds())+1))
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rule.Limit))
			w.Header().Set("X-RateLimit-Remaining", "0")
			Error(w, r, apierr.TooManyRequests(
				"Слишком много запросов. Повторите через "+humanDuration(decision.RetryAfter)))
		})
	}
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())+1) + " с"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())+1) + " мин"
	default:
		return strconv.Itoa(int(d.Hours())+1) + " ч"
	}
}
