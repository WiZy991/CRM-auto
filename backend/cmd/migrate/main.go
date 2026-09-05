// Команда управления миграциями базы данных.
//
//	go run ./cmd/migrate up        — применить все непримененные миграции
//	go run ./cmd/migrate down 1    — откатить указанное число миграций
//	go run ./cmd/migrate status    — показать состояние
//
// Подключение выполняется под ролью миграций (POSTGRES_MIGRATOR_USER),
// у которой есть права на изменение структуры базы. Роль приложения таких
// прав не имеет — это осознанное разделение, ограничивающее ущерб при
// компрометации API.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/store"
	"github.com/autoimport/crm/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nОшибка: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logging.New(cfg.App.LogLevel, cfg.App.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, err := pgx.Connect(connectCtx, cfg.Postgres.MigratorDSN())
	if err != nil {
		return fmt.Errorf("подключение к базе под ролью миграций: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer closeCancel()
		_ = conn.Close(closeCtx)
	}()

	migrator := store.NewMigrator(migrations.FS, ".", log)

	switch command {
	case "up":
		return migrator.Up(ctx, conn)

	case "down":
		steps := 1
		if len(os.Args) > 2 {
			steps, err = strconv.Atoi(os.Args[2])
			if err != nil {
				return fmt.Errorf("число миграций для откага должно быть целым, получено %q", os.Args[2])
			}
		}
		if cfg.App.Env.IsProduction() {
			return errors.New("откат миграций в production через эту команду запрещён: " +
				"используйте восстановление из резервной копии")
		}
		return migrator.Down(ctx, conn, steps)

	case "status":
		applied, pending, err := migrator.Status(ctx, conn)
		if err != nil {
			return err
		}
		fmt.Printf("Применено (%d):\n", len(applied))
		for _, item := range applied {
			fmt.Printf("  + %s\n", item)
		}
		fmt.Printf("\nОжидает применения (%d):\n", len(pending))
		for _, item := range pending {
			fmt.Printf("  - %s\n", item)
		}
		return nil

	default:
		return fmt.Errorf("неизвестная команда %q, доступны: up, down, status", command)
	}
}
