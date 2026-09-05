package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Раннер миграций написан вручную вместо готовой библиотеки. Причины:
//
//   - миграции применяются под отдельной ролью с правами на DDL, а не под
//     ролью приложения, и логика подключения здесь своя;
//   - каждая миграция выполняется в одной транзакции вместе с записью в
//     журнал версий, поэтому частично применённой миграции быть не может;
//   - контрольная сумма файла проверяется при каждом запуске: изменение уже
//     применённой миграции обнаруживается сразу, а не превращается в
//     расхождение схемы между стендами;
//   - весь код занимает меньше двухсот строк и не тянет за собой драйверы
//     десяти других баз данных.

// Migration — одна миграция, собранная из пары up/down файлов.
type Migration struct {
	Version  int
	Name     string
	UpSQL    string
	DownSQL  string
	Checksum string
}

// Migrator применяет миграции из файловой системы (обычно embed.FS).
type Migrator struct {
	fsys fs.FS
	dir  string
	log  *slog.Logger
}

func NewMigrator(fsys fs.FS, dir string, log *slog.Logger) *Migrator {
	if dir == "" {
		dir = "."
	}
	return &Migrator{fsys: fsys, dir: dir, log: log}
}

const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     integer     PRIMARY KEY,
    name        text        NOT NULL,
    checksum    text        NOT NULL,
    applied_at  timestamptz NOT NULL DEFAULT now(),
    duration_ms integer     NOT NULL
)`

// Load читает и разбирает файлы миграций.
//
// Ожидаемый формат имени: 000007_add_something.up.sql
func (m *Migrator) Load() ([]Migration, error) {
	entries, err := fs.ReadDir(m.fsys, m.dir)
	if err != nil {
		return nil, fmt.Errorf("чтение каталога миграций: %w", err)
	}

	byVersion := make(map[int]*Migration)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, direction, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, err
		}

		content, err := fs.ReadFile(m.fsys, path.Join(m.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("чтение %s: %w", entry.Name(), err)
		}

		migration, ok := byVersion[version]
		if !ok {
			migration = &Migration{Version: version, Name: name}
			byVersion[version] = migration
		}

		switch direction {
		case "up":
			migration.UpSQL = string(content)
			sum := sha256.Sum256(content)
			migration.Checksum = hex.EncodeToString(sum[:])
		case "down":
			migration.DownSQL = string(content)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if strings.TrimSpace(migration.UpSQL) == "" {
			return nil, fmt.Errorf("миграция %d (%s): отсутствует файл .up.sql", migration.Version, migration.Name)
		}
		out = append(out, *migration)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	return out, nil
}

func parseMigrationName(filename string) (version int, name, direction string, err error) {
	base := strings.TrimSuffix(filename, ".sql")

	dotIdx := strings.LastIndex(base, ".")
	if dotIdx < 0 {
		return 0, "", "", fmt.Errorf("имя %q не содержит направления .up/.down", filename)
	}
	direction = base[dotIdx+1:]
	if direction != "up" && direction != "down" {
		return 0, "", "", fmt.Errorf("имя %q: направление должно быть up или down", filename)
	}

	rest := base[:dotIdx]
	underscore := strings.Index(rest, "_")
	if underscore <= 0 {
		return 0, "", "", fmt.Errorf("имя %q: ожидался формат 000001_название.up.sql", filename)
	}

	version, err = strconv.Atoi(rest[:underscore])
	if err != nil {
		return 0, "", "", fmt.Errorf("имя %q: номер версии не является числом", filename)
	}
	return version, rest[underscore+1:], direction, nil
}

type appliedMigration struct {
	Version  int
	Name     string
	Checksum string
}

// Up применяет все непримененные миграции.
func (m *Migrator) Up(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, migrationsTableDDL); err != nil {
		return fmt.Errorf("создание таблицы schema_migrations: %w", err)
	}

	migrations, err := m.Load()
	if err != nil {
		return err
	}

	applied, err := m.applied(ctx, conn)
	if err != nil {
		return err
	}

	pending := 0
	for _, migration := range migrations {
		if existing, ok := applied[migration.Version]; ok {
			if existing.Checksum != migration.Checksum {
				return fmt.Errorf(
					"миграция %d (%s) уже применена, но её содержимое изменилось "+
						"(в базе %s, в файле %s). Изменять применённые миграции нельзя — "+
						"создайте новую",
					migration.Version, migration.Name,
					existing.Checksum[:12], migration.Checksum[:12],
				)
			}
			continue
		}

		if err := m.applyOne(ctx, conn, migration); err != nil {
			return err
		}
		pending++
	}

	if pending == 0 {
		m.log.Info("миграции: изменений нет", slog.Int("applied_total", len(applied)))
	} else {
		m.log.Info("миграции применены", slog.Int("count", pending))
	}
	return nil
}

func (m *Migrator) applyOne(ctx context.Context, conn *pgx.Conn, migration Migration) error {
	start := time.Now()

	// DDL и запись в журнал версий выполняются в одной транзакции:
	// PostgreSQL поддерживает транзакционный DDL, поэтому при ошибке
	// схема остаётся в исходном состоянии целиком.
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("миграция %d: начало транзакции: %w", migration.Version, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
		return fmt.Errorf("миграция %d (%s): %w", migration.Version, migration.Name, err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, duration_ms)
		 VALUES ($1, $2, $3, $4)`,
		migration.Version, migration.Name, migration.Checksum, int(time.Since(start).Milliseconds()),
	); err != nil {
		return fmt.Errorf("миграция %d: запись в журнал: %w", migration.Version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("миграция %d: фиксация: %w", migration.Version, err)
	}

	m.log.Info("миграция применена",
		slog.Int("version", migration.Version),
		slog.String("name", migration.Name),
		slog.Duration("took", time.Since(start)),
	)
	return nil
}

// Down откатывает последние steps миграций.
func (m *Migrator) Down(ctx context.Context, conn *pgx.Conn, steps int) error {
	if steps <= 0 {
		steps = 1
	}

	migrations, err := m.Load()
	if err != nil {
		return err
	}
	byVersion := make(map[int]Migration, len(migrations))
	for _, migration := range migrations {
		byVersion[migration.Version] = migration
	}

	applied, err := m.applied(ctx, conn)
	if err != nil {
		return err
	}

	versions := make([]int, 0, len(applied))
	for version := range applied {
		versions = append(versions, version)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	for i, version := range versions {
		if i >= steps {
			break
		}

		migration, ok := byVersion[version]
		if !ok {
			return fmt.Errorf("миграция %d применена, но её файлы отсутствуют", version)
		}
		if strings.TrimSpace(migration.DownSQL) == "" {
			return fmt.Errorf("миграция %d (%s) не имеет .down.sql — откат невозможен", version, migration.Name)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("откат %d: начало транзакции: %w", version, err)
		}

		if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			return fmt.Errorf("откат %d (%s): %w", version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			return fmt.Errorf("откат %d: очистка журнала: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("откат %d: фиксация: %w", version, err)
		}

		m.log.Info("миграция откачена", slog.Int("version", version), slog.String("name", migration.Name))
	}
	return nil
}

// Status возвращает список применённых и ожидающих миграций.
func (m *Migrator) Status(ctx context.Context, conn *pgx.Conn) (appliedList, pendingList []string, err error) {
	migrations, err := m.Load()
	if err != nil {
		return nil, nil, err
	}

	applied, err := m.applied(ctx, conn)
	if err != nil {
		// Таблицы может не быть на чистой базе — это не ошибка состояния.
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "42P01" {
			applied = map[int]appliedMigration{}
		} else {
			return nil, nil, err
		}
	}

	for _, migration := range migrations {
		label := fmt.Sprintf("%06d_%s", migration.Version, migration.Name)
		if _, ok := applied[migration.Version]; ok {
			appliedList = append(appliedList, label)
		} else {
			pendingList = append(pendingList, label)
		}
	}
	return appliedList, pendingList, nil
}

func (m *Migrator) applied(ctx context.Context, conn *pgx.Conn) (map[int]appliedMigration, error) {
	rows, err := conn.Query(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("чтение schema_migrations: %w", err)
	}
	defer rows.Close()

	result := make(map[int]appliedMigration)
	for rows.Next() {
		var item appliedMigration
		if err := rows.Scan(&item.Version, &item.Name, &item.Checksum); err != nil {
			return nil, fmt.Errorf("разбор строки schema_migrations: %w", err)
		}
		result[item.Version] = item
	}
	return result, rows.Err()
}
