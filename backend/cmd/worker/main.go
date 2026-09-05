// Отдельный процесс фоновых задач.
//
// В development задачи крутятся внутри API. Этот бинарник нужен на стенде,
// где HTTP и фоновая работа разносятся по разным процессам, чтобы перенос
// счётчиков баннеров не делил пул соединений с пользовательскими запросами.
//
//	go run ./cmd/worker
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

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
	log.Info("запуск обработчика фоновых задач")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     8,
	})
	defer func() { _ = rdb.Close() }()

	dealComms := store.NewDealComms(pool)
	notifications := store.NewNotifications(pool)
	sessions := store.NewSessions(pool)
	bannerCounters := service.NewBannerCounters(rdb, log)
	bannerService := service.NewBanners(store.NewBanners(pool), bannerCounters, notifications, store.NewSecurityLog(pool), log)

	piiCipher, err := security.NewCipher(cfg.Auth.PIIKey)
	if err != nil {
		return fmt.Errorf("ключ шифрования персональных данных: %w", err)
	}
	cars := store.NewCars(pool)
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

	worker := service.NewWorker(bannerService, dealComms, notifications, sessions, log, socialService)
	log.Info("фоновые задачи запущены")
	worker.Run(ctx)
	return nil
}
