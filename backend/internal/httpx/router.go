package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/metrics"
	"github.com/autoimport/crm/internal/pkg/ratelimit"
	"github.com/autoimport/crm/internal/pkg/storage"
)

// RouterDeps — зависимости маршрутизатора.
type RouterDeps struct {
	Config  *config.Config
	Log     *slog.Logger
	Metrics *metrics.Registry

	Guard *Guard
	Auth  *AuthMiddleware

	AuthHandler         *AuthHandler
	CarHandler          *CarHandler
	RequestHandler      *RequestHandler
	DealHandler         *DealHandler
	SellerHandler       *SellerHandler
	BannerHandler       *BannerHandler
	AdminHandler        *AdminHandler
	NotificationHandler *NotificationHandler
	HealthHandler       *HealthHandler
	UploadHandler       *UploadHandler
	DealerHandler       *DealerHandler
	SocialHandler       *SocialHandler
	UploadsDir          string
}

// Правила ограничения частоты для отдельных групп эндпоинтов.
//
// Значения различаются на порядки, и это главное: единый лимит на всё либо
// не защищает вход, либо ломает каталог. Дорогие и опасные операции
// (вход, регистрация, отправка кода) ограничены жёстко, чтение каталога —
// свободно.
func rateRules(cfg config.RateLimit) map[string]ratelimit.Rule {
	return map[string]ratelimit.Rule{
		"login": {
			Name: "login", Limit: cfg.LoginPerMinute, Window: time.Minute,
			// Небольшой всплеск разрешён: человек может ошибиться дважды
			// подряд, это не атака.
			Burst: cfg.LoginPerMinute + 2,
		},
		"register": {
			Name: "register", Limit: cfg.RegisterPerHour, Window: time.Hour,
			Burst: cfg.RegisterPerHour,
		},
		"verify_code": {
			Name: "verify_code", Limit: cfg.VerifyCodePer10Min, Window: 10 * time.Minute,
			Burst: cfg.VerifyCodePer10Min,
		},
		"search": {
			Name: "search", Limit: cfg.SearchPerMinute, Window: time.Minute,
			Burst: cfg.SearchPerMinute + 20,
		},
		"upload": {
			Name: "upload", Limit: cfg.UploadPerHour, Window: time.Hour,
			Burst: 5,
		},
		"global": {
			Name: "global", Limit: cfg.GlobalPerMinute, Window: time.Minute,
			Burst: cfg.GlobalPerMinute / 2,
		},
		"mutation": {
			// Общий лимит на изменяющие запросы авторизованного пользователя:
			// защищает от скриптов, создающих сотни объявлений или заявок.
			Name: "mutation", Limit: 120, Window: time.Minute, Burst: 40,
		},
	}
}

// NewRouter собирает маршрутизатор.
//
// Порядок middleware — часть защиты, а не стилистика:
//
//  1. идентификатор запроса и логгер — чтобы у любой записи был след;
//  2. RealIP — до всех проверок по адресу;
//  3. Recovery — снаружи прикладной логики;
//  4. AccessLog и метрики — считают в том числе отклонённые запросы;
//  5. BanCheck — заблокированный адрес не должен идти дальше;
//  6. WAF — отбрасывает сканеры до разбора тела;
//  7. SecurityHeaders, CORS, BodyLimit — подготовка ответа и границы ввода;
//  8. глобальный лимит, затем аутентификация и точечные лимиты.
func NewRouter(deps RouterDeps) http.Handler {
	cfg := deps.Config
	rules := rateRules(cfg.RateLimit)

	r := chi.NewRouter()

	r.Use(RequestIDMiddleware(len(cfg.HTTP.TrustedProxies) > 0))
	r.Use(WithBaseLogger(deps.Log))
	r.Use(RealIP(cfg.HTTP.TrustedProxies))
	r.Use(Recovery(deps.Log))
	r.Use(AccessLog(deps.Metrics))
	r.Use(deps.Guard.BanCheck())
	r.Use(deps.Guard.PunishStatus())
	r.Use(deps.Guard.WAF())
	r.Use(SecurityHeaders(cfg.App.Env.IsProduction()))
	r.Use(CORS(cfg.HTTP.AllowedOrigins))
	r.Use(Gzip)
	r.Use(BodyLimit(cfg.Limits.MaxRequestBodyBytes, cfg.Limits.MaxUploadBytes))

	// Служебные эндпоинты вне общих лимитов: система мониторинга не должна
	// получать 429 и считать сервис упавшим.
	r.Get("/healthz", deps.HealthHandler.Live)
	r.Get("/readyz", deps.HealthHandler.Ready)
	r.Get("/metrics", deps.Metrics.Handler())

	if deps.UploadsDir != "" {
		r.Handle("/uploads/*", http.StripPrefix("/uploads/", storage.FileServer(deps.UploadsDir)))
	}

	// Пока SMTP/SMS не подключены, коды никуда не уходят.
	// В development подтверждение контакта не режет заявки и объявления.
	requireVerified := RequireVerified
	if cfg.App.Env.IsDevelopment() {
		requireVerified = func(next http.Handler) http.Handler { return next }
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(deps.Guard.RateLimit(rules["global"]))
		api.Use(deps.Auth.Authenticate)
		api.Use(ETagOnGET)
		api.Use(deps.Guard.Idempotency())

		api.Get("/meta/stages", stagesHandler)

		api.Route("/dealers", func(dealers chi.Router) {
			dealers.Use(deps.Guard.RateLimit(rules["search"]))

			dealers.Get("/", deps.DealerHandler.List)
			dealers.With(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin)).
				Get("/me", deps.DealerHandler.Mine)
			dealers.With(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin), requireVerified).
				Put("/me", deps.DealerHandler.SaveMine)
			dealers.Get("/{slug}/cars", deps.DealerHandler.Cars)
			dealers.Get("/{slug}/reviews", deps.DealerHandler.Reviews)
			dealers.Get("/{slug}", deps.DealerHandler.Get)
		})

		api.With(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin)).
			Get("/analytics", deps.DealHandler.Summary)

		api.Route("/auth", func(auth chi.Router) {
			auth.Group(func(public chi.Router) {
				public.With(deps.Guard.RateLimit(rules["register"])).
					Post("/register", deps.AuthHandler.Register)
				public.With(deps.Guard.RateLimit(rules["login"])).
					Post("/login", deps.AuthHandler.Login)
				public.With(deps.Guard.RateLimit(rules["login"])).
					Get("/precheck", deps.AuthHandler.LoginPrecheck)

				// Обновление и выход опираются на cookie, поэтому требуют
				// защиты от подделки межсайтового запроса.
				public.With(CSRF).Post("/refresh", deps.AuthHandler.Refresh)
				public.With(CSRF).Post("/logout", deps.AuthHandler.Logout)
			})

			auth.Group(func(private chi.Router) {
				private.Use(RequireAuth)

				private.Get("/me", deps.AuthHandler.Me)
				private.Patch("/profile", deps.AuthHandler.UpdateProfile)
				private.Get("/sessions", deps.AuthHandler.Sessions)
				private.Delete("/sessions/{id}", deps.AuthHandler.RevokeSession)

				private.With(deps.Guard.RateLimitByUser(rules["verify_code"])).
					Post("/verify/resend", deps.AuthHandler.SendCode)
				private.With(deps.Guard.RateLimitByUser(rules["verify_code"])).
					Post("/verify", deps.AuthHandler.Verify)

				private.With(CSRF).Post("/logout-all", deps.AuthHandler.LogoutAll)
				private.With(deps.Guard.RateLimitByUser(rules["login"])).
					Post("/password", deps.AuthHandler.ChangePassword)
				private.With(CSRF).Post("/delete", deps.AuthHandler.DeleteAccount)
			})
		})

		api.Route("/cars", func(cars chi.Router) {
			cars.Use(deps.Guard.RateLimit(rules["search"]))

			// Статические сегменты объявлены до параметрических, иначе
			// /cars/my попал бы в обработчик /cars/{id} как идентификатор.
			cars.Get("/", deps.CarHandler.List)
			cars.Get("/dictionaries", deps.CarHandler.Dictionaries)

			cars.With(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin)).
				Get("/my", deps.CarHandler.MyCars)

			cars.Get("/{id}", deps.CarHandler.Get)

			cars.Group(func(dealer chi.Router) {
				dealer.Use(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin))
				dealer.Use(requireVerified)
				dealer.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				dealer.Post("/", deps.CarHandler.Create)
				dealer.Put("/{id}", deps.CarHandler.Update)
				dealer.Patch("/{id}/status", deps.CarHandler.ChangeStatus)
				dealer.Post("/{id}/publish-social", deps.CarHandler.PublishSocial)
				dealer.Delete("/{id}", deps.CarHandler.Delete)
			})

			cars.Group(func(client chi.Router) {
				client.Use(RequireAuth)
				client.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				client.Put("/{id}/favorite", deps.CarHandler.SetFavorite(true))
				client.Delete("/{id}/favorite", deps.CarHandler.SetFavorite(false))
			})
		})

		api.Route("/requests", func(requests chi.Router) {
			requests.Use(RequireAuth)

			requests.Get("/my", deps.RequestHandler.MyRequests)
			requests.Get("/{id}", deps.RequestHandler.Get)

			// Заявку создаёт только клиент с подтверждённым контактом:
			// дилеру нужен способ связи, а неподтверждённый телефон
			// превращает заявку в мусор в пуле.
			requests.With(
				RequireRole(domain.RoleClient),
				requireVerified,
				deps.Guard.RateLimitByUser(rules["mutation"]),
			).Post("/", deps.RequestHandler.Create)

			requests.With(
				RequireRole(domain.RoleClient),
				deps.Guard.RateLimitByUser(rules["mutation"]),
			).Post("/{id}/close", deps.RequestHandler.Close)
		})

		api.Get("/integrations/oauth/callback", deps.SocialHandler.OAuthCallback)

		api.Route("/dealer/channels", func(channels chi.Router) {
			channels.Use(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin))
			channels.Use(requireVerified)

			channels.Get("/", deps.SocialHandler.List)
			channels.Get("/{network}/oauth/start", deps.SocialHandler.OAuthStart)

			channels.Group(func(mutate chi.Router) {
				mutate.Use(CSRF)
				mutate.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				mutate.Put("/", deps.SocialHandler.Save)
				mutate.Post("/{network}/test", deps.SocialHandler.Test)
			})
		})

		api.Route("/dealer/reviews", func(reviews chi.Router) {
			reviews.Use(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin))
			reviews.Get("/", deps.DealerHandler.MineReviews)
			reviews.Post("/{id}/reply", deps.DealerHandler.ReplyReview)
		})

		api.Route("/dealer/requests", func(dealer chi.Router) {
			dealer.Use(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin))
			dealer.Use(requireVerified)

			dealer.Get("/", deps.RequestHandler.DealerRequests)
			dealer.Get("/pool", deps.RequestHandler.OpenPool)

			dealer.Group(func(mutate chi.Router) {
				mutate.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				mutate.Post("/{id}/claim", deps.RequestHandler.Claim)
				mutate.Post("/{id}/reply", deps.RequestHandler.Reply)
				mutate.Post("/{id}/reject", deps.RequestHandler.Reject)
			})
		})

		api.Route("/deals", func(deals chi.Router) {
			deals.Use(RequireAuth)

			// Статические сегменты идут первыми: иначе /deals/summary
			// уйдёт в обработчик /deals/{id} и упадёт на разборе UUID.
			deals.Get("/stages", deps.DealHandler.Stages)
			deals.Get("/summary", deps.DealHandler.Summary)
			deals.With(RequireRole(domain.RoleDealer, domain.RoleAdmin)).
				Get("/clients", deps.DealHandler.SearchClients)
			deals.Get("/", deps.DealHandler.List)
			deals.Get("/{id}", deps.DealHandler.Get)

			// Переписка доступна обеим сторонам сделки: право проверяется
			// по участию, а не по роли.
			deals.Get("/{id}/messages", deps.DealHandler.Messages)
			deals.With(deps.Guard.RateLimitByUser(rules["mutation"])).
				Post("/{id}/messages", deps.DealHandler.SendMessage)
			deals.With(deps.Guard.RateLimitByUser(rules["mutation"])).
				Post("/{id}/review", deps.DealHandler.LeaveReview)

			deals.Group(func(manage chi.Router) {
				manage.Use(RequireRole(domain.RoleDealer, domain.RoleAdmin))
				manage.Use(requireVerified)
				manage.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				manage.Post("/", deps.DealHandler.Create)
				manage.Patch("/{id}", deps.DealHandler.Update)
				manage.Post("/{id}/stage", deps.DealHandler.ChangeStage)
				manage.Post("/{id}/close", deps.DealHandler.Close)

				manage.Post("/{id}/tasks", deps.DealHandler.CreateTask)
				manage.Post("/{id}/tasks/{taskID}/toggle", deps.DealHandler.ToggleTask)
				manage.Delete("/{id}/tasks/{taskID}", deps.DealHandler.DeleteTask)

				manage.Post("/{id}/documents", deps.DealHandler.AddDocument)
				manage.Get("/{id}/documents/preview", deps.DealHandler.PreviewDocument)
				manage.Post("/{id}/documents/generate", deps.DealHandler.GenerateDocuments)
			})
		})

		api.With(RequireAuth).Get("/documents/{id}", deps.DealHandler.DownloadDocument)

		api.Route("/notifications", func(notifications chi.Router) {
			notifications.Use(RequireAuth)

			notifications.Get("/", deps.NotificationHandler.List)
			notifications.Get("/unread", deps.NotificationHandler.UnreadCount)
			notifications.Post("/read", deps.NotificationHandler.MarkRead)
		})

		// Справочник поставщиков без контактов: гостю видны страна, тип и
		// бренды, телефоны остаются за кабинетом дилера.
		api.Route("/directory/sellers", func(dir chi.Router) {
			dir.Use(deps.Guard.RateLimit(rules["search"]))
			dir.Get("/", deps.SellerHandler.List)
			dir.Get("/facets", deps.SellerHandler.Facets)
			dir.Get("/{id}", deps.SellerHandler.Get)
		})

		// База продавцов закрыта от неавторизованных: это рабочий
		// инструмент импортёра, а открытый справочник контактов зарубежных
		// поставщиков — готовая база для рассылок и парсинга.
		api.Route("/sellers", func(sellers chi.Router) {
			sellers.Use(RequireAuth)
			sellers.Use(deps.Guard.RateLimit(rules["search"]))

			sellers.Get("/facets", deps.SellerHandler.Facets)
			sellers.With(RequireRole(domain.RoleSeller, domain.RoleAdmin)).
				Get("/my", deps.SellerHandler.Mine)

			sellers.Get("/", deps.SellerHandler.List)
			sellers.Get("/{id}", deps.SellerHandler.Get)

			sellers.Group(func(edit chi.Router) {
				edit.Use(RequireRole(domain.RoleDealer, domain.RoleSeller, domain.RoleAdmin))
				edit.Use(requireVerified)
				edit.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				edit.Post("/", deps.SellerHandler.Create)
				edit.Put("/{id}", deps.SellerHandler.Update)
				edit.Patch("/{id}/active", deps.SellerHandler.SetActive)
			})

			// Приватные заметки о поставщиках ведёт только дилер.
			sellers.Group(func(notes chi.Router) {
				notes.Use(RequireRole(domain.RoleDealer))
				notes.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				notes.Put("/{id}/note", deps.SellerHandler.SaveNote)
				notes.Delete("/{id}/note", deps.SellerHandler.DeleteNote)
			})
		})

		api.Route("/banners", func(banners chi.Router) {
			banners.Get("/", deps.BannerHandler.Active)
			banners.Get("/{id}/click", deps.BannerHandler.Click)
		})

		api.Route("/dealer/banners", func(banners chi.Router) {
			banners.Use(RequireAuth, RequireRole(domain.RoleDealer, domain.RoleAdmin))
			banners.Use(requireVerified)

			banners.Get("/", deps.BannerHandler.Mine)

			banners.Group(func(mutate chi.Router) {
				mutate.Use(deps.Guard.RateLimitByUser(rules["mutation"]))

				mutate.Post("/", deps.BannerHandler.Create)
				mutate.Put("/{id}", deps.BannerHandler.Update)
				mutate.Post("/{id}/submit", deps.BannerHandler.Submit)
				mutate.Patch("/{id}/pause", deps.BannerHandler.SetPaused)
				mutate.Delete("/{id}", deps.BannerHandler.Delete)
			})
		})

		api.Route("/uploads", func(uploads chi.Router) {
			uploads.Use(RequireAuth, requireVerified)
			uploads.Use(deps.Guard.RateLimitByUser(rules["upload"]))
			uploads.Post("/images", deps.UploadHandler.Images)
		})

		api.Route("/admin", func(admin chi.Router) {
			admin.Use(RequireAuth, RequireRole(domain.RoleAdmin))

			admin.Get("/overview", deps.AdminHandler.Overview)
			admin.Get("/users", deps.AdminHandler.Users)
			admin.Get("/audit", deps.AdminHandler.Audit)
			admin.Get("/security-events", deps.AdminHandler.SecurityEvents)
			admin.Get("/cars", deps.AdminHandler.Cars)

			admin.Get("/banners", deps.BannerHandler.PendingModeration)

			// Изменение прав и модерация проходят проверку от подделки
			// межсайтового запроса: это самые чувствительные действия на
			// платформе, и одного заголовка авторизации для них мало.
			admin.Group(func(mutate chi.Router) {
				mutate.Use(CSRF)

				mutate.Patch("/users/{id}/status", deps.AdminHandler.SetUserStatus)
				mutate.Patch("/users/{id}/role", deps.AdminHandler.SetUserRole)
				mutate.Post("/banners/{id}/moderate", deps.BannerHandler.Moderate)
				mutate.Patch("/sellers/{id}/verify", deps.SellerHandler.SetVerified)
				mutate.Post("/cars/{id}/moderate", deps.AdminHandler.ModerateCar)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Error(w, r, notFoundError())
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		Error(w, r, methodNotAllowedError())
	})

	return r
}

// stagesHandler отдаёт описание этапов воронки.
//
// Справочник приходит с сервера, а не дублируется во фронтенде: названия,
// подсказки и нормативные сроки должны быть в одном месте, иначе интерфейс
// и логика расходятся.
func stagesHandler(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, map[string]any{"items": domain.StagesCatalog()})
}
