package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/autoimport/crm/internal/domain"
)

// Ошибки уровня хранилища. Сервисный слой превращает их в ответы API,
// чтобы наружу не уходил текст ошибки PostgreSQL.
var (
	ErrNotFound   = errors.New("запись не найдена")
	ErrEmailTaken = errors.New("адрес электронной почты уже используется")
	ErrPhoneTaken = errors.New("номер телефона уже используется")
	ErrDuplicate  = errors.New("запись с такими данными уже существует")
)

// Users — доступ к учётным записям.
type Users struct {
	pool *Pool
}

func NewUsers(pool *Pool) *Users { return &Users{pool: pool} }

// CreateUserParams — параметры создания учётной записи.
type CreateUserParams struct {
	Role         domain.Role
	Email        string
	Phone        string
	PasswordHash string
	FullName     string
	// ImmediateActive — development: учётка сразу active и с подтверждёнными
	// контактами, пока коды на почту не подключены.
	ImmediateActive bool
}

// UserRecord — строка таблицы users вместе с полем хеша пароля.
//
// Хеш вынесен в отдельную структуру и не входит в domain.User: так его
// физически невозможно случайно отдать в JSON-ответе.
type UserRecord struct {
	domain.User
	PasswordHash string
}

const userColumns = `
	id, role, status, email, phone, password_hash, full_name,
	COALESCE(avatar_url, ''), locale,
	email_verified_at, phone_verified_at,
	failed_login_count, locked_until, last_login_at,
	created_at, updated_at`

func scanUser(row pgx.Row) (*UserRecord, error) {
	var rec UserRecord
	err := row.Scan(
		&rec.ID, &rec.Role, &rec.Status, &rec.Email, &rec.Phone, &rec.PasswordHash, &rec.FullName,
		&rec.AvatarURL, &rec.Locale,
		&rec.EmailVerifiedAt, &rec.PhoneVerifiedAt,
		&rec.FailedLoginCount, &rec.LockedUntil, &rec.LastLoginAt,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение пользователя: %w", err)
	}
	return &rec, nil
}

// Create создаёт учётную запись.
func (u *Users) Create(ctx context.Context, params CreateUserParams) (*UserRecord, error) {
	status := domain.StatusPending
	if params.ImmediateActive {
		status = domain.StatusActive
	}

	row := u.pool.QueryRow(ctx, `
		INSERT INTO users (
			role, email, phone, password_hash, full_name, status,
			email_verified_at, phone_verified_at)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			CASE WHEN $7 THEN now() ELSE NULL END,
			CASE WHEN $7 THEN now() ELSE NULL END)
		RETURNING `+userColumns,
		params.Role, strings.ToLower(strings.TrimSpace(params.Email)),
		params.Phone, params.PasswordHash, params.FullName,
		status, params.ImmediateActive,
	)

	rec, err := scanUser(row)
	if err != nil {
		return nil, mapUserConstraint(err)
	}
	return rec, nil
}

// ActivatePending переводит pending в active и отмечает контакты.
//
// Нужен только в development, пока подтверждение почты отключено: иначе
// уже созданные учётки навсегда остаются невидимыми в каталоге дилеров.
func (u *Users) ActivatePending(ctx context.Context, userID uuid.UUID) (*UserRecord, error) {
	row := u.pool.QueryRow(ctx, `
		UPDATE users
		SET status = 'active',
		    email_verified_at = COALESCE(email_verified_at, now()),
		    phone_verified_at = COALESCE(phone_verified_at, now())
		WHERE id = $1 AND deleted_at IS NULL AND status = 'pending'
		RETURNING `+userColumns, userID)
	rec, err := scanUser(row)
	if errors.Is(err, ErrNotFound) {
		return u.ByID(ctx, userID)
	}
	return rec, err
}

// pgUniqueViolation — код ошибки PostgreSQL «нарушение уникальности».
const pgUniqueViolation = "23505"

// mapUserConstraint переводит нарушение уникальности в понятную ошибку.
//
// Имя ограничения разбирается здесь, а не в обработчике: сервисный слой не
// должен знать про индексы, а клиент — про существование таблиц.
func mapUserConstraint(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgUniqueViolation {
		return err
	}

	switch pgErr.ConstraintName {
	case "users_email_unique":
		return ErrEmailTaken
	case "users_phone_unique":
		return ErrPhoneTaken
	default:
		return ErrDuplicate
	}
}

// ByID возвращает пользователя по идентификатору.
func (u *Users) ByID(ctx context.Context, id uuid.UUID) (*UserRecord, error) {
	return scanUser(u.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id))
}

// ByEmail возвращает пользователя по адресу электронной почты.
func (u *Users) ByEmail(ctx context.Context, email string) (*UserRecord, error) {
	return scanUser(u.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1 AND deleted_at IS NULL`,
		strings.ToLower(strings.TrimSpace(email))))
}

// ByPhone возвращает пользователя по номеру телефона в любом формате записи.
func (u *Users) ByPhone(ctx context.Context, phone string) (*UserRecord, error) {
	return scanUser(u.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users
		 WHERE normalize_phone(phone) = normalize_phone($1) AND deleted_at IS NULL`, phone))
}

// ClientLookup — краткая карточка покупателя для выбора в сделке.
type ClientLookup struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
	Phone    string    `json:"phone"`
}

// SearchClients ищет покупателей по имени, почте или телефону.
func (u *Users) SearchClients(ctx context.Context, query string, limit int) ([]ClientLookup, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return []ClientLookup{}, nil
	}
	if limit <= 0 || limit > 30 {
		limit = 15
	}

	pattern := "%" + query + "%"
	rows, err := u.pool.Query(ctx, `
		SELECT id, full_name, email::text, phone
		FROM users
		WHERE deleted_at IS NULL AND role = 'client'
		  AND (full_name ILIKE $1 OR email::text ILIKE $1 OR phone ILIKE $1)
		ORDER BY full_name, created_at DESC
		LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("поиск клиентов: %w", err)
	}
	defer rows.Close()
	return scanClientLookups(rows)
}

// RecentClientsForDealer — покупатели, с которыми у дилера уже были сделки.
func (u *Users) RecentClientsForDealer(ctx context.Context, dealerID uuid.UUID, limit int) ([]ClientLookup, error) {
	if limit <= 0 || limit > 30 {
		limit = 15
	}
	rows, err := u.pool.Query(ctx, `
		SELECT users.id, users.full_name, users.email::text, users.phone
		FROM users
		JOIN LATERAL (
			SELECT deals.created_at
			FROM deals
			WHERE deals.client_id = users.id AND deals.dealer_id = $1
			ORDER BY deals.created_at DESC
			LIMIT 1
		) AS last_deal ON true
		WHERE users.deleted_at IS NULL AND users.role = 'client'
		ORDER BY last_deal.created_at DESC
		LIMIT $2`, dealerID, limit)
	if err != nil {
		return nil, fmt.Errorf("недавние клиенты дилера: %w", err)
	}
	defer rows.Close()
	return scanClientLookups(rows)
}

func scanClientLookups(rows pgx.Rows) ([]ClientLookup, error) {
	out := make([]ClientLookup, 0, 8)
	for rows.Next() {
		var item ClientLookup
		if err := rows.Scan(&item.ID, &item.FullName, &item.Email, &item.Phone); err != nil {
			return nil, fmt.Errorf("разбор клиента: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// EmailExists проверяет занятость адреса.
func (u *Users) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := u.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("проверка занятости email: %w", err)
	}
	return exists, nil
}

// RegisterFailedLogin увеличивает счётчик неудач и, при необходимости,
// ставит блокировку.
//
// Счётчик и блокировка обновляются одним запросом: два отдельных запроса
// позволяли бы обойти защиту, запустив попытки входа параллельно.
func (u *Users) RegisterFailedLogin(ctx context.Context, userID uuid.UUID, lockFor time.Duration) (int, error) {
	var count int
	var lockedUntil *time.Time

	err := u.pool.QueryRow(ctx, `
		UPDATE users
		SET failed_login_count = failed_login_count + 1,
		    locked_until = CASE
		        WHEN $2::interval > interval '0' THEN now() + $2::interval
		        ELSE locked_until
		    END
		WHERE id = $1
		RETURNING failed_login_count, locked_until`,
		userID, lockFor,
	).Scan(&count, &lockedUntil)
	if err != nil {
		return 0, fmt.Errorf("регистрация неудачного входа: %w", err)
	}
	return count, nil
}

// RegisterSuccessfulLogin сбрасывает счётчик неудач и запоминает вход.
func (u *Users) RegisterSuccessfulLogin(ctx context.Context, userID uuid.UUID, ip string) error {
	_, err := u.pool.Exec(ctx, `
		UPDATE users
		SET failed_login_count = 0,
		    locked_until = NULL,
		    last_login_at = now(),
		    last_login_ip = NULLIF($2, '')::inet
		WHERE id = $1`, userID, ip)
	if err != nil {
		return fmt.Errorf("обновление данных о входе: %w", err)
	}
	return nil
}

// UpdatePasswordHash сохраняет новый хеш пароля.
func (u *Users) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string) error {
	_, err := u.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash)
	if err != nil {
		return fmt.Errorf("обновление пароля: %w", err)
	}
	return nil
}

// MarkVerified отмечает контакт подтверждённым и активирует учётную запись,
// если подтверждены оба канала.
//
// Оба шага идут в одной транзакции: иначе между отметкой канала и
// активацией возможно состояние, когда контакты подтверждены, а статус
// остался pending, и пользователь не может пользоваться платформой.
func (u *Users) MarkVerified(ctx context.Context, userID uuid.UUID, channel domain.VerifyChannel) (*UserRecord, error) {
	// Имя колонки подставляется из закрытого перечня, а не из ввода
	// пользователя: значение channel уже проверено доменом.
	column := "email_verified_at"
	if channel == domain.ChannelPhone {
		column = "phone_verified_at"
	}

	var result *UserRecord
	err := u.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE users SET %s = COALESCE(%s, now())
			WHERE id = $1 AND deleted_at IS NULL`, column, column), userID); err != nil {
			return fmt.Errorf("отметка подтверждения контакта: %w", err)
		}

		row := tx.QueryRow(ctx, `
			UPDATE users
			SET status = 'active'
			WHERE id = $1
			  AND status = 'pending'
			  AND email_verified_at IS NOT NULL
			RETURNING `+userColumns, userID)

		rec, err := scanUser(row)
		switch {
		case err == nil:
			result = rec
			return nil
		case errors.Is(err, ErrNotFound):
			// Условие активации ещё не выполнено — это нормальный случай,
			// просто возвращаем текущее состояние записи.
			rec, err = scanUser(tx.QueryRow(ctx,
				`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
			if err != nil {
				return err
			}
			result = rec
			return nil
		default:
			return err
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateProfileParams — изменяемые пользователем поля профиля.
type UpdateProfileParams struct {
	FullName  string
	AvatarURL *string
	Locale    *string
}

// UpdateProfile обновляет профиль.
func (u *Users) UpdateProfile(ctx context.Context, userID uuid.UUID, params UpdateProfileParams) (*UserRecord, error) {
	row := u.pool.QueryRow(ctx, `
		UPDATE users
		SET full_name  = COALESCE(NULLIF($2, ''), full_name),
		    avatar_url = COALESCE($3, avatar_url),
		    locale     = COALESCE($4, locale)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns,
		userID, params.FullName, params.AvatarURL, params.Locale)

	return scanUser(row)
}

// EncryptedPII — паспорт и адрес в том виде, в каком они лежат в базе.
type EncryptedPII struct {
	Passport []byte
	Address  []byte
}

// EncryptedPII читает зашифрованные поля. Расшифровка — зона сервиса:
// хранилище не знает ключ и не должно его знать.
func (u *Users) EncryptedPII(ctx context.Context, userID uuid.UUID) (*EncryptedPII, error) {
	var pii EncryptedPII
	err := u.pool.QueryRow(ctx, `
		SELECT passport_encrypted, address_encrypted
		FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&pii.Passport, &pii.Address)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение персональных данных: %w", err)
	}
	return &pii, nil
}

// SetEncryptedPII записывает уже зашифрованные паспорт и адрес.
func (u *Users) SetEncryptedPII(ctx context.Context, userID uuid.UUID, passport, address []byte) error {
	_, err := u.pool.Exec(ctx, `
		UPDATE users
		SET passport_encrypted = $2, address_encrypted = $3
		WHERE id = $1 AND deleted_at IS NULL`, userID, passport, address)
	if err != nil {
		return fmt.Errorf("запись персональных данных: %w", err)
	}
	return nil
}

// ChangeEmail меняет адрес и сбрасывает его подтверждение.
func (u *Users) ChangeEmail(ctx context.Context, userID uuid.UUID, email string) (*UserRecord, error) {
	row := u.pool.QueryRow(ctx, `
		UPDATE users
		SET email = $2, email_verified_at = NULL,
		    status = CASE WHEN status = 'active' THEN 'pending'::user_status ELSE status END
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns,
		userID, strings.ToLower(strings.TrimSpace(email)))

	rec, err := scanUser(row)
	if err != nil {
		return nil, mapUserConstraint(err)
	}
	return rec, nil
}

// ChangePhone меняет телефон и сбрасывает его подтверждение.
func (u *Users) ChangePhone(ctx context.Context, userID uuid.UUID, phone string) (*UserRecord, error) {
	row := u.pool.QueryRow(ctx, `
		UPDATE users
		SET phone = $2, phone_verified_at = NULL,
		    status = CASE WHEN status = 'active' THEN 'pending'::user_status ELSE status END
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns, userID, phone)

	rec, err := scanUser(row)
	if err != nil {
		return nil, mapUserConstraint(err)
	}
	return rec, nil
}

// SetStatus меняет статус учётной записи (действие администратора).
func (u *Users) SetStatus(ctx context.Context, userID uuid.UUID, status domain.UserStatus) error {
	_, err := u.pool.Exec(ctx,
		`UPDATE users SET status = $2 WHERE id = $1 AND deleted_at IS NULL`, userID, status)
	if err != nil {
		return fmt.Errorf("изменение статуса пользователя: %w", err)
	}
	return nil
}

// SoftDelete помечает запись удалённой.
//
// Полное удаление невозможно: на пользователя ссылаются сделки и документы,
// которые дилер обязан хранить. Персональные данные при этом обезличиваются,
// а уникальные индексы освобождают email и телефон для повторной регистрации.
func (u *Users) SoftDelete(ctx context.Context, userID uuid.UUID) error {
	return u.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE users
			SET deleted_at = now(),
			    status = 'deleted',
			    full_name = 'Удалённый пользователь',
			    passport_encrypted = NULL,
			    address_encrypted = NULL,
			    avatar_url = NULL,
			    email = 'deleted+' || id::text || '@deleted.local',
			    phone = '0000000000' || substr(id::text, 1, 5)
			WHERE id = $1`, userID); err != nil {
			return fmt.Errorf("обезличивание пользователя: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE refresh_sessions
			SET revoked_at = now(), revoked_reason = 'user_deleted'
			WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return fmt.Errorf("отзыв сессий удалённого пользователя: %w", err)
		}
		return nil
	})
}
