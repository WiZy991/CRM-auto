// Загрузка демонстрационных данных из ТЗ.
//
//	go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/pkg/logging"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/store"
)

const demoPassword = "Devpass12!"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nОшибка seed: %v\n", err)
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

	pool, err := store.NewPool(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	hasher := security.NewHasher(cfg.Auth.Argon2MemoryKiB, cfg.Auth.Argon2Iterations, cfg.Auth.Argon2Parallelism)
	hash, err := hasher.Hash(demoPassword)
	if err != nil {
		return err
	}

	adminID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	dealerID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	clientID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	users := []struct {
		id, role, email, phone, name string
	}{
		{adminID.String(), "admin", "admin@local.test", "+79990000001", "Администратор"},
		{dealerID.String(), "dealer", "dealer@local.test", "+79990000002", "Игорь Владивосток"},
		{clientID.String(), "client", "client@local.test", "+79990000003", "Анна Клиент"},
	}

	for _, u := range users {
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id, role, status, email, phone, password_hash, full_name, email_verified_at, phone_verified_at)
			VALUES ($1, $2, 'active', $3, $4, $5, $6, now(), now())
			ON CONFLICT (id) DO UPDATE
			  SET password_hash = EXCLUDED.password_hash,
			      status = 'active',
			      email_verified_at = now(),
			      phone_verified_at = now()`,
			u.id, u.role, u.email, u.phone, hash, u.name)
		if err != nil {
			return fmt.Errorf("пользователь %s: %w", u.email, err)
		}
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO dealer_profiles (user_id, slug, company_name, city, description, services, work_countries, verified_at)
		VALUES ($1, 'vostok-import', 'Восток Импорт', 'Владивосток',
		        'Подбор, выкуп с аукциона, доставка и растаможка из Китая и Японии.',
		        '["подбор","аукцион","доставка","растаможка"]'::jsonb,
		        ARRAY['cn','jp']::origin_country[], now())
		ON CONFLICT (user_id) DO NOTHING`, dealerID)
	if err != nil {
		return fmt.Errorf("профиль дилера: %w", err)
	}

	sellers := []struct {
		id, country, kind, name, region, brands, website, contacts string
	}{
		{
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "jp", "auction", "USS Tokyo", "Kanto",
			"{Toyota,Lexus,Nissan}", "https://www.ussnet.co.jp",
			`{"phone":"+81 3-3570-5711","wechat":"uss_tokyo_export"}`,
		},
		{
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", "jp", "auction", "TAA Yokohama", "Kanto",
			"{Honda,Nissan,Mazda}", "https://www.taa.jp",
			`{"phone":"+81 45-500-1234","wechat":"taa_yokohama"}`,
		},
		{
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3", "cn", "exporter", "Guangzhou Exchange", "Guangdong",
			"{Zeekr,Li,BYD}", "https://www.gdee.cn",
			`{"phone":"+86 20 3888 1000","wechat":"gz_exchange"}`,
		},
	}
	for _, s := range sellers {
		_, err := pool.Exec(ctx, `
			INSERT INTO sellers (id, created_by, country, kind, name, region, brands, description, website, contacts, verified_at, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7::text[], $5, $8, $9::jsonb, now(), true)
			ON CONFLICT (id) DO UPDATE
			  SET website = EXCLUDED.website,
			      contacts = EXCLUDED.contacts`,
			s.id, dealerID, s.country, s.kind, s.name, s.region, s.brands, s.website, s.contacts)
		if err != nil {
			return fmt.Errorf("продавец %s: %w", s.name, err)
		}
	}

	type car struct {
		id, origin, brand, model, title, fuel, gearbox, drive, body, vin, grade, interior string
		year, mileage, engine, power, price                                               int
		steer                                                                             bool
		seller                                                                            string
	}
	cars := []car{
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1", "jp", "Toyota", "Land Cruiser 300", "Toyota Land Cruiser 300, USS Tokyo",
			"petrol", "at", "awd", "suv", "JTMHV05J604012345", "4.5", "B", 2023, 18000, 3445, 415, 9_800_000_00, true,
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"},
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2", "jp", "Nissan", "X-Trail", "Nissan X-Trail, TAA Yokohama",
			"hybrid", "cvt", "awd", "crossover", "JTMBH31V606123456", "4", "A", 2022, 32000, 1997, 150, 2_450_000_00, true,
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"},
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3", "cn", "Zeekr", "001X", "Zeekr 001X, Guangzhou Exchange",
			"electric", "at", "awd", "liftback", "L6T79XES5NA123456", "", "", 2024, 2100, 0, 544, 6_200_000_00, false,
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"},
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb4", "cn", "Li", "L9", "Li Auto L9, Guangzhou Exchange",
			"phev", "at", "awd", "suv", "L6T84XES7NA234567", "", "", 2023, 8900, 1496, 449, 5_400_000_00, false,
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"},
		{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb5", "cn", "BYD", "Han", "BYD Han EV, Guangzhou Exchange",
			"electric", "at", "rwd", "sedan", "LGXCE4CB5N0123456", "", "", 2023, 12000, 0, 222, 3_150_000_00, false,
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"},
	}

	for _, item := range cars {
		engine := any(item.engine)
		if item.engine == 0 {
			engine = nil
		}
		grade := any(item.grade)
		if item.grade == "" {
			grade = nil
		}
		interior := any(item.interior)
		if item.interior == "" {
			interior = nil
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO cars (
			  id, dealer_id, seller_id, status, origin, brand, model, year, mileage_km,
			  engine_cc, power_hp, fuel, gearbox, drive, body, steering_right,
			  auction_grade, interior_grade, vin, price_minor, currency, price_rub_minor,
			  title, description, published_at)
			VALUES (
			  $1, $2, $3, 'active', $4, $5, $6, $7, $8,
			  $9, $10, $11, $12, $13, $14, $15,
			  $16, $17, $18, $19, 'rub', $19,
			  $20, $20, now())
			ON CONFLICT (id) DO NOTHING`,
			item.id, dealerID, item.seller, item.origin, item.brand, item.model, item.year, item.mileage,
			engine, item.power, item.fuel, item.gearbox, item.drive, item.body, item.steer,
			grade, interior, item.vin, item.price, item.title)
		if err != nil {
			return fmt.Errorf("лот %s: %w", item.title, err)
		}
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO cars (
		  id, dealer_id, status, origin, brand, model, year, mileage_km,
		  engine_cc, power_hp, fuel, gearbox, drive, body, steering_right,
		  vin, price_minor, currency, price_rub_minor, title, description)
		VALUES (
		  'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb6', $1, 'moderation', 'jp',
		  'Mazda', 'CX-5', 2021, 41000, 1998, 155, 'petrol', 'at', 'awd', 'crossover',
		  true, 'JM0KF2W7A01234567', 245000000, 'rub', 245000000,
		  'Mazda CX-5, очередь модерации', 'Черновик на проверке администратора')
		ON CONFLICT (id) DO UPDATE SET status = 'moderation'`, dealerID)
	if err != nil {
		return fmt.Errorf("лот на модерации: %w", err)
	}

	extraDealers := []struct {
		id, email, phone, name, slug, company, city, desc string
	}{
		{"11111111-1111-4111-8111-111111111112", "ussuriysk@local.test", "+79990000004",
			"Сергей Уссурийск", "primorie-auto", "Приморье Авто", "Уссурийск",
			"Выкуп с японских аукционов, доставка до Уссурийска и растаможка."},
		{"11111111-1111-4111-8111-111111111113", "moscow@local.test", "+79990000005",
			"Марина Москва", "stolica-import", "Столица Импорт", "Москва",
			"Китайский рынок под ключ: подбор, договор, логистика, СБКТС."},
	}
	for _, d := range extraDealers {
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id, role, status, email, phone, password_hash, full_name, email_verified_at, phone_verified_at)
			VALUES ($1, 'dealer', 'active', $2, $3, $4, $5, now(), now())
			ON CONFLICT (id) DO UPDATE
			  SET password_hash = EXCLUDED.password_hash, status = 'active'`,
			d.id, d.email, d.phone, hash, d.name)
		if err != nil {
			return fmt.Errorf("дилер %s: %w", d.email, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO dealer_profiles (user_id, slug, company_name, city, description, services, work_countries, verified_at)
			VALUES ($1, $2, $3, $4, $5, '["подбор","доставка","растаможка"]'::jsonb,
			        ARRAY['cn','jp']::origin_country[], now())
			ON CONFLICT (user_id) DO UPDATE
			  SET city = EXCLUDED.city, company_name = EXCLUDED.company_name`,
			d.id, d.slug, d.company, d.city, d.desc)
		if err != nil {
			return fmt.Errorf("профиль %s: %w", d.slug, err)
		}
	}

	type seedDeal struct {
		id, stage, outcome, title, lost string
		amount                          int64
		closedMonthsAgo                 int
		car                             string
	}
	pipelineDeals := []seedDeal{
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc01", "lead", "open", "Подбор кроссовера до 3 млн", "", 2_800_000_00, 0, ""},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc02", "needs", "open", "Nissan X-Trail, согласование комплектации", "", 2_450_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc03", "contract", "open", "Договор на Zeekr 001X", "", 6_200_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc04", "payment", "open", "Оплата Li Auto L9", "", 5_400_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb4"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc05", "shipping", "open", "Привоз BYD Han из Гуанчжоу", "", 3_150_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb5"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc06", "customs", "open", "Растаможка Toyota Land Cruiser 300", "", 9_800_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc07", "handover", "open", "Выдача Nissan X-Trail", "", 2_450_000_00, 0, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc08", "handover", "won", "Выдан Zeekr 001X", "", 6_200_000_00, 1, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc09", "handover", "won", "Выдан BYD Han", "", 3_150_000_00, 2, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb5"},
		{"cccccccc-cccc-4ccc-8ccc-cccccccccc10", "needs", "lost", "Отказ: бюджет не совпал", "Клиент выбрал другой рынок", 1_900_000_00, 3, ""},
	}

	for _, d := range pipelineDeals {
		var carID any
		if d.car != "" {
			carID = d.car
		}
		var closed any
		if d.outcome != "open" {
			closed = time.Now().AddDate(0, -d.closedMonthsAgo, 0)
		}
		var lost any
		if d.lost != "" {
			lost = d.lost
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO deals (
			  id, client_id, dealer_id, car_id, stage, outcome, title,
			  amount_minor, currency, amount_rub_minor, lost_reason, closed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'rub', $8, $9, $10)
			ON CONFLICT (id) DO UPDATE
			  SET stage = EXCLUDED.stage, outcome = EXCLUDED.outcome,
			      title = EXCLUDED.title, amount_minor = EXCLUDED.amount_minor,
			      amount_rub_minor = EXCLUDED.amount_rub_minor,
			      lost_reason = EXCLUDED.lost_reason, closed_at = EXCLUDED.closed_at`,
			d.id, clientID, dealerID, carID, d.stage, d.outcome, d.title, d.amount, lost, closed)
		if err != nil {
			return fmt.Errorf("сделка %s: %w", d.title, err)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO deal_stage_history (deal_id, from_stage, to_stage, outcome, changed_by, comment)
			SELECT $1, NULL, $2, $3, $4, 'Демо-данные'
			WHERE NOT EXISTS (SELECT 1 FROM deal_stage_history WHERE deal_id = $1)`,
			d.id, d.stage, d.outcome, dealerID)
		if err != nil {
			return fmt.Errorf("история сделки %s: %w", d.title, err)
		}
	}

	fmt.Println("Демо-данные готовы. Вход:")
	fmt.Println("  дилер     dealer@local.test        " + demoPassword)
	fmt.Println("  уссурийск ussuriysk@local.test     " + demoPassword)
	fmt.Println("  москва    moscow@local.test        " + demoPassword)
	fmt.Println("  клиент    client@local.test        " + demoPassword)
	fmt.Println("  админ     admin@local.test         " + demoPassword)
	fmt.Println("время:", time.Now().Format(time.RFC3339))
	return nil
}
