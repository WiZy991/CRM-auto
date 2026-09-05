// Package store отвечает за доступ к PostgreSQL.
//
// Используется pgx напрямую, без database/sql и без ORM. Причины:
//
//   - все запросы к базе видны в коде как SQL, поэтому план выполнения
//     предсказуем, а не зависит от того, что сгенерирует ORM;
//   - pgx умеет протокол PostgreSQL нативно, включая массивы, jsonb,
//     enum-типы и COPY, без прослойки database/sql;
//   - параметризованные запросы — единственный способ обращения к базе,
//     поэтому SQL-инъекция исключена конструктивно, а не проверками.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/autoimport/crm/internal/config"
)

// Pool — пул соединений с базой.
type Pool struct {
	*pgxpool.Pool
	log *slog.Logger
}

// NewPool создаёт и проверяет пул соединений.
func NewPool(ctx context.Context, cfg config.Postgres, log *slog.Logger) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("разбор строки подключения: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns

	// Соединения принудительно обновляются: это ограничивает рост памяти
	// на стороне сервера и не даёт «залипнуть» на упавшем узле при
	// использовании пулера или реплики.
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnLifetimeJitter = 5 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	// Таймаут на установление соединения: без него запрос к упавшей базе
	// висит до таймаута ОС, удерживая горутину и слот пула.
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second

	// Параметры сессии выставляются на уровне подключения, а не запроса:
	// даже если какой-то запрос забудет про таймаут, база оборвёт его сама.
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "autoimport-api"
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = msString(cfg.StatementTimeout)
	poolCfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = msString(30 * time.Second)
	poolCfg.ConnConfig.RuntimeParams["lock_timeout"] = msString(5 * time.Second)

	// Кеш подготовленных выражений на стороне соединения: экономит один
	// round-trip на повторяющихся запросах каталога и воронки.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	poolCfg.ConnConfig.StatementCacheCapacity = 256

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("создание пула соединений: %w", err)
	}

	var pingErr error
	for attempt := 0; attempt < 8; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pingErr = pool.Ping(pingCtx)
		cancel()
		if pingErr == nil {
			break
		}
		time.Sleep(750 * time.Millisecond)
	}
	if pingErr != nil {
		pool.Close()
		return nil, fmt.Errorf("проверка соединения с базой: %w", pingErr)
	}

	log.Info("подключение к PostgreSQL установлено",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("database", cfg.Database),
		slog.Int("max_conns", int(cfg.MaxConns)),
	)

	return &Pool{Pool: pool, log: log}, nil
}

func msString(d time.Duration) string {
	return fmt.Sprintf("%d", d.Milliseconds())
}

// InTx выполняет функцию в транзакции: при ошибке откат, при успехе фиксация.
//
// Обёртка нужна, чтобы ни один сценарий не забыл rollback при panic или
// раннем return — самая частая причина «зависших» транзакций и блокировок.
func (p *Pool) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		// Откат выполняется в контексте без отмены: если исходный контекст
		// уже истёк, откат всё равно должен дойти до сервера.
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !isTxClosed(rbErr) {
			return fmt.Errorf("%w (а также ошибка откага: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}
	return nil
}

func isTxClosed(err error) bool {
	return err != nil && err.Error() == pgx.ErrTxClosed.Error()
}

// Stats отдаёт статистику пула для метрик и /readyz.
func (p *Pool) Stats() map[string]int64 {
	s := p.Pool.Stat()
	return map[string]int64{
		"total_conns":      int64(s.TotalConns()),
		"idle_conns":       int64(s.IdleConns()),
		"acquired_conns":   int64(s.AcquiredConns()),
		"max_conns":        int64(s.MaxConns()),
		"acquire_count":    s.AcquireCount(),
		"canceled_acquire": s.CanceledAcquireCount(),
		"empty_acquire":    s.EmptyAcquireCount(),
	}
}
