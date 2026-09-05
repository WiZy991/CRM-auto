// Команда запуска HTTP API платформы.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/httpx"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/pkg/metrics"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/pkg/notify"
	"github.com/autoimport/crm/internal/pkg/ratelimit"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/pkg/storage"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// version подставляется при сборке: -ldflags "-X main.version=1.2.3"
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "критическая ошибка: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logging.New(cfg.App.LogLevel, cfg.App.LogFormat)
	slog.SetDefault(log)

	log.Info("запуск сервиса",
		slog.String("version", version),
		slog.String("env", string(cfg.App.Env)),
		slog.String("addr", cfg.HTTP.Addr()),
	)

	// Контекст завершается по SIGINT или SIGTERM: контейнер должен
	// останавливаться штатно, доводя начатые запросы до конца.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	disk, err := storage.NewDisk(cfg.Storage.LocalPath)
	if err != nil {
		return fmt.Errorf("хранилище файлов: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:            cfg.Redis.Addr,
		Password:        cfg.Redis.Password,
		DB:              cfg.Redis.DB,
		DialTimeout:     3 * time.Second,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		PoolSize:        20,
		MinIdleConns:    2,
		ConnMaxLifetime: 30 * time.Minute,
	})
	defer func() { _ = rdb.Close() }()

	pingCtx, cancelPing := context.WithTimeout(ctx, 3*time.Second)
	defer cancelPing()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		// Redis не является жёсткой зависимостью для запуска: без него
		// работают ограничения на уровне nginx, а приложение переходит в
		// режим «пропускать» с записью в лог. Падать при старте нельзя —
		// иначе перезапуск Redis положит всю платформу.
		log.Error("Redis недоступен при старте, ограничения частоты работают в ослабленном режиме",
			slog.String("addr", cfg.Redis.Addr),
			slog.String("error", err.Error()))
	} else {
		log.Info("подключение к Redis установлено", slog.String("addr", cfg.Redis.Addr))
	}

	registry := metrics.New(version)
	registry.SetDBPoolSource(pool.Stats)

	// --- Слои приложения ---
	users := store.NewUsers(pool)
	sessions := store.NewSessions(pool)
	verification := store.NewVerification(pool)
	securityLog := store.NewSecurityLog(pool)
	cars := store.NewCars(pool)
	requests := store.NewRequests(pool)
	deals := store.NewDeals(pool)
	dealComms := store.NewDealComms(pool)
	notifications := store.NewNotifications(pool)
	sellers := store.NewSellers(pool)
	banners := store.NewBanners(pool)
	adminStore := store.NewAdmin(pool)

	converter := money.NewConverter()

	hasher := security.NewHasher(cfg.Auth.Argon2MemoryKiB, cfg.Auth.Argon2Iterations, cfg.Auth.Argon2Parallelism)
	tokens := security.NewTokenIssuer(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer, cfg.Auth.AccessTokenTTL)
	revoker := service.NewSessionRevoker(rdb, cfg.Auth.AccessTokenTTL)

	piiCipher, err := security.NewCipher(cfg.Auth.PIIKey)
	if err != nil {
		return fmt.Errorf("ключ шифрования персональных данных: %w", err)
	}

	dealerStore := store.NewDealers(pool)

	authService := service.NewAuth(service.AuthDeps{
		Users:        users,
		Sessions:     sessions,
		Verification: verification,
		SecurityLog:  securityLog,
		Hasher:       hasher,
		Tokens:       tokens,
		Revoker:      revoker,
		Mailer:       notify.NewMailer(cfg.Mailer, log),
		SMS:          notify.NewSMS(cfg.SMS, log),
		Config:       cfg.Auth,
		Log:          log,
		AppName:      cfg.App.Name,
		PublicURL:    cfg.App.PublicURL,
		Cipher:       piiCipher,
		Dealers:      dealerStore,
		Sellers:      sellers,
		SkipVerify:   cfg.App.Env.IsDevelopment(),
	})

	catalogService := service.NewCatalog(cars, converter, securityLog, log)
	socialService := service.NewSocial(
		store.NewSocialAccounts(pool),
		cars,
		notifications,
		piiCipher,
		cfg.Social,
		cfg.App.PublicURL,
		cfg.Storage.PublicBaseURL,
		rdb,
		log,
	)
	catalogService.SetSocial(socialService)
	dealerService := service.NewPublicDealers(dealerStore, cars, store.NewReviews(pool))
	requestService := service.NewRequests(requests, cars, notifications, securityLog, log)
	pipelineService := service.NewPipeline(
		deals, requests, dealComms, store.NewReviews(pool), notifications, converter, securityLog, log,
		service.PipelineExtras{
			Users: users, Dealers: dealerStore, Cars: cars, Sellers: sellers,
			Cipher: piiCipher, Disk: disk,
		})
	sellerService := service.NewSellers(sellers, securityLog, log)
	bannerCounters := service.NewBannerCounters(rdb, log)
	bannerService := service.NewBanners(banners, bannerCounters, notifications, securityLog, log)
	adminService := service.NewAdmin(adminStore, sessions, securityLog, revoker, log)

	guard := &httpx.Guard{
		Limiter: ratelimit.New(rdb, "rl"),
		Blocklist: ratelimit.NewBlocklist(rdb, "ban", ratelimit.BlocklistConfig{
			Threshold:    cfg.RateLimit.SuspiciousLimit,
			Window:       cfg.RateLimit.AutobanWindow,
			BaseDuration: cfg.RateLimit.AutobanBase,
			MaxDuration:  cfg.RateLimit.AutobanMax,
		}),
		Metrics: registry,
		Enabled: cfg.RateLimit.Enabled && !cfg.App.Env.IsDevelopment(),
		Log:     log,
		Redis:   rdb,
	}

	router := httpx.NewRouter(httpx.RouterDeps{
		Config:  cfg,
		Log:     log,
		Metrics: registry,
		Guard:   guard,
		Auth: &httpx.AuthMiddleware{
			Tokens:  tokens,
			Revoker: revoker,
		},
		AuthHandler:         httpx.NewAuthHandler(authService, httpx.ShouldSecureCookies(cfg.App.PublicURL)),
		CarHandler:          httpx.NewCarHandler(catalogService, socialService),
		RequestHandler:      httpx.NewRequestHandler(requestService),
		DealHandler:         httpx.NewDealHandler(pipelineService, disk),
		SellerHandler:       httpx.NewSellerHandler(sellerService),
		BannerHandler:       httpx.NewBannerHandler(bannerService),
		AdminHandler:        httpx.NewAdminHandler(adminService, catalogService),
		NotificationHandler: httpx.NewNotificationHandler(notifications),
		HealthHandler:       httpx.NewHealthHandler(version, pool, redisPinger{rdb}),
		UploadHandler:       httpx.NewUploadHandler(disk, cfg.Limits.MaxUploadBytes, cfg.Limits.MaxImagePixels),
		DealerHandler:       httpx.NewDealerHandler(dealerService),
		SocialHandler:       httpx.NewSocialHandler(socialService),
		UploadsDir:          disk.Dir(),
	})

	server := &http.Server{
		Addr:    cfg.HTTP.Addr(),
		Handler: router,

		// Таймауты обязательны. Значения по умолчанию в http.Server
		// отсутствуют, и сервер без них удерживает соединение сколько
		// угодно — это готовая уязвимость к медленным клиентам.
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,

		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	worker := service.NewWorker(bannerService, dealComms, notifications, sessions, log, socialService)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(ctx)
	}()

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("HTTP-сервер слушает", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("сбой HTTP-сервера: %w", err)

	case <-ctx.Done():
		log.Info("получен сигнал завершения, останавливаем приём запросов")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			// Принудительное закрытие: лучше обрубить остатки, чем повиснуть.
			_ = server.Close()
			return fmt.Errorf("не удалось штатно остановить сервер: %w", err)
		}

		// Счётчики баннеров живут в Redis и переносятся в базу фоновой
		// задачей, поэтому перед выходом ждём её завершения — иначе
		// накопленная с последнего тика статистика теряется при каждом
		// развёртывании.
		select {
		case <-workerDone:
		case <-shutdownCtx.Done():
			log.Warn("фоновые задачи не завершились в отведённое время")
		}

		flushCtx, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFlush()
		if err := bannerService.FlushCounters(flushCtx); err != nil {
			log.Error("не удалось перенести счётчики баннеров при остановке",
				slog.String("error", err.Error()))
		}

		log.Info("сервис остановлен штатно")
		return nil
	}
}

// redisPinger приводит клиент Redis к интерфейсу проверки готовности.
type redisPinger struct {
	client redis.UniversalClient
}

func (p redisPinger) Ping(ctx context.Context) error {
	return p.client.Ping(ctx).Err()
}
