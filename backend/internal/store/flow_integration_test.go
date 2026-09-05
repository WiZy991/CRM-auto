package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/autoimport/crm/internal/domain"
)

func TestDealerVisibleAndRequestBecomesDeal(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	users := NewUsers(pool)
	dealers := NewDealers(pool)
	requests := NewRequests(pool)
	deals := NewDeals(pool)

	suffix := fmt.Sprintf("%07d", time.Now().UnixNano()%10_000_000)
	dealer, err := users.Create(ctx, CreateUserParams{
		Role:            domain.RoleDealer,
		Email:           "itest-dealer-" + suffix + "@local.test",
		Phone:           "+7999" + suffix,
		PasswordHash:    "argon2-placeholder-not-used",
		FullName:        "Интеграционный дилер",
		ImmediateActive: true,
	})
	if err != nil {
		t.Fatalf("создание дилера: %v", err)
	}

	client, err := users.Create(ctx, CreateUserParams{
		Role:            domain.RoleClient,
		Email:           "itest-client-" + suffix + "@local.test",
		Phone:           "+7988" + suffix,
		PasswordHash:    "argon2-placeholder-not-used",
		FullName:        "Интеграционный клиент",
		ImmediateActive: true,
	})
	if err != nil {
		t.Fatalf("создание клиента: %v", err)
	}

	stranger, err := users.Create(ctx, CreateUserParams{
		Role:            domain.RoleDealer,
		Email:           "itest-other-" + suffix + "@local.test",
		Phone:           "+7977" + suffix,
		PasswordHash:    "argon2-placeholder-not-used",
		FullName:        "Чужой дилер",
		ImmediateActive: true,
	})
	if err != nil {
		t.Fatalf("создание чужого дилера: %v", err)
	}

	t.Cleanup(func() {
		clean := context.Background()
		_, _ = pool.Exec(clean, `DELETE FROM deals WHERE dealer_id = ANY($1) OR client_id = ANY($1)`, []uuid.UUID{dealer.ID, client.ID, stranger.ID})
		_, _ = pool.Exec(clean, `DELETE FROM requests WHERE client_id = ANY($1) OR dealer_id = ANY($1)`, []uuid.UUID{dealer.ID, client.ID, stranger.ID})
		_, _ = pool.Exec(clean, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{dealer.ID, client.ID, stranger.ID})
	})

	if dealer.Status != domain.StatusActive {
		t.Fatalf("статус дилера %s, ожидалось active", dealer.Status)
	}
	if dealer.EmailVerifiedAt == nil || dealer.PhoneVerifiedAt == nil {
		t.Fatal("в development контакты должны быть сразу подтверждены")
	}

	if err := dealers.EnsureDefault(ctx, dealer.ID, dealer.FullName); err != nil {
		t.Fatalf("профиль дилера: %v", err)
	}

	items, _, err := dealers.List(ctx, DealerFilter{Limit: 50})
	if err != nil {
		t.Fatalf("список дилеров: %v", err)
	}
	found := false
	for _, item := range items {
		if item.UserID == dealer.ID {
			found = true
			if item.Verified {
				t.Error("новый дилер не должен требовать verified_at, чтобы попасть в каталог")
			}
			break
		}
	}
	if !found {
		t.Fatal("созданный дилер не попал в публичный список")
	}

	req, err := requests.Create(ctx, CreateRequestParams{
		ClientID:          client.ID,
		DesiredBrand:      "Haval",
		DesiredModel:      "H6",
		Comment:           "интеграционный сценарий",
		ContactPreference: string(domain.ContactPhone),
	})
	if err != nil {
		t.Fatalf("создание заявки: %v", err)
	}

	claimed, err := requests.Claim(ctx, req.ID, dealer.ID)
	if err != nil {
		t.Fatalf("закрепление заявки: %v", err)
	}
	if claimed.DealerID == nil || *claimed.DealerID != dealer.ID {
		t.Fatal("заявка не закрепилась за дилером")
	}

	deal, err := deals.Create(ctx, CreateDealParams{
		RequestID: &req.ID,
		ClientID:  client.ID,
		DealerID:  dealer.ID,
		Title:     claimed.Summary(),
		Currency:  domain.CurrencyRUB,
	})
	if err != nil {
		t.Fatalf("создание сделки: %v", err)
	}
	if deal.DealerID != dealer.ID || deal.ClientID != client.ID {
		t.Fatal("сделка создана с неверными участниками")
	}

	listed, _, err := deals.List(ctx, DealFilter{DealerID: &dealer.ID, Limit: 20}, dealer.ID)
	if err != nil {
		t.Fatalf("список сделок: %v", err)
	}
	seen := false
	for _, item := range listed {
		if item.Deal.ID == deal.ID {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatal("сделка не появилась в списке дилера")
	}

	_, err = deals.ByIDForParticipant(ctx, deal.ID, stranger.ID, false)
	if err == nil {
		t.Fatal("чужой дилер прочитал сделку")
	}
	if err != ErrNotFound {
		t.Fatalf("чужой UUID должен давать ErrNotFound, получено %v", err)
	}

	_, err = requests.ByIDForParticipant(ctx, req.ID, stranger.ID, false, true)
	if err == nil {
		t.Fatal("чужой дилер прочитал чужую заявку после закрепления")
	}
	if err != ErrNotFound {
		t.Fatalf("чужая заявка должна давать ErrNotFound, получено %v", err)
	}

	moved, err := deals.ChangeStage(ctx, ChangeStageParams{
		DealID:    deal.ID,
		DealerID:  dealer.ID,
		ToStage:   domain.StageNeeds,
		Comment:   "интеграционный переход",
		ChangedBy: dealer.ID,
	})
	if err != nil {
		t.Fatalf("смена этапа: %v", err)
	}
	if moved.Stage != domain.StageNeeds {
		t.Fatalf("этап %s, ожидалось needs", moved.Stage)
	}
}

func testPool(t *testing.T) *Pool {
	t.Helper()
	loadDotEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		host := getenv("POSTGRES_HOST", "127.0.0.1")
		port := getenv("POSTGRES_PORT", "15433")
		db := getenv("POSTGRES_DB", "autoimport")
		user := getenv("POSTGRES_USER", "autoimport_app")
		pass := os.Getenv("POSTGRES_PASSWORD")
		ssl := getenv("POSTGRES_SSLMODE", "disable")
		if pass == "" {
			t.Skip("нет TEST_DATABASE_URL и POSTGRES_PASSWORD — интеграционный тест пропущен")
		}
		dsn = "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + db + "?sslmode=" + ssl
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skip("некорректный DSN: " + err.Error())
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second

	raw, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skip("не удалось подключиться к Postgres: " + err.Error())
	}
	if err := raw.Ping(ctx); err != nil {
		raw.Close()
		t.Skip("Postgres недоступен: " + err.Error())
	}

	pool := &Pool{Pool: raw, log: slog.Default()}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func loadDotEnv() {
	candidates := []string{
		".env",
		filepath.Join("..", "..", "..", ".env"),
		filepath.Join("..", "..", ".env"),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "..", "..", "..", ".env"))
	}
	for _, path := range candidates {
		if err := godotenv.Load(path); err == nil {
			return
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestGetenvPortParses(t *testing.T) {
	// Защита от регрессии: порт из .env должен быть числом, иначе DSN ломается.
	if _, err := strconv.Atoi(getenv("POSTGRES_PORT", "15433")); err != nil {
		t.Fatal(err)
	}
}
