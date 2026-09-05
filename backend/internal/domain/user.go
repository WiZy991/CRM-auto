// Package domain описывает сущности и бизнес-правила платформы.
//
// Пакет не знает ни про HTTP, ни про PostgreSQL: здесь живут только правила,
// которые остаются верными независимо от способа доставки данных. Благодаря
// этому правила переходов воронки и матрицу прав можно покрыть тестами без
// поднятия базы.
package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Role — роль пользователя на платформе.
type Role string

const (
	// RoleClient — физическое лицо, покупающее автомобиль.
	RoleClient Role = "client"
	// RoleDealer — дилер, оказывающий услуги привоза и продажи.
	RoleDealer Role = "dealer"
	// RoleSeller — зарубежный продавец из Китая или Японии.
	RoleSeller Role = "seller"
	// RoleAdmin — сотрудник платформы.
	RoleAdmin Role = "admin"
)

// AllRegistrableRoles — роли, доступные при самостоятельной регистрации.
// Администратор в этот список не входит намеренно: такую учётную запись
// можно создать только командой из консоли.
var AllRegistrableRoles = []Role{RoleClient, RoleDealer, RoleSeller}

func (r Role) Valid() bool {
	switch r {
	case RoleClient, RoleDealer, RoleSeller, RoleAdmin:
		return true
	default:
		return false
	}
}

// Registrable сообщает, можно ли зарегистрироваться с этой ролью.
func (r Role) Registrable() bool {
	for _, allowed := range AllRegistrableRoles {
		if r == allowed {
			return true
		}
	}
	return false
}

func (r Role) String() string { return string(r) }

// Title — название роли для интерфейса.
func (r Role) Title() string {
	switch r {
	case RoleClient:
		return "Клиент"
	case RoleDealer:
		return "Дилер"
	case RoleSeller:
		return "Продавец"
	case RoleAdmin:
		return "Администратор"
	default:
		return string(r)
	}
}

// ParseRole разбирает роль из строки.
func ParseRole(raw string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(raw)))
	if !role.Valid() {
		return "", fmt.Errorf("неизвестная роль %q", raw)
	}
	return role, nil
}

// UserStatus — состояние учётной записи.
type UserStatus string

const (
	// StatusPending — зарегистрирован, но не подтвердил контакты.
	StatusPending UserStatus = "pending"
	// StatusActive — полноценный доступ.
	StatusActive UserStatus = "active"
	// StatusSuspended — доступ приостановлен администратором.
	StatusSuspended UserStatus = "suspended"
	// StatusDeleted — учётная запись удалена по требованию пользователя.
	StatusDeleted UserStatus = "deleted"
)

// CanSignIn сообщает, разрешён ли вход с таким статусом.
func (s UserStatus) CanSignIn() bool {
	return s == StatusActive || s == StatusPending
}

// Valid проверяет, что состояние входит в перечисление.
func (s UserStatus) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusSuspended, StatusDeleted:
		return true
	default:
		return false
	}
}

func (s UserStatus) Title() string {
	switch s {
	case StatusPending:
		return "Ожидает подтверждения"
	case StatusActive:
		return "Активна"
	case StatusSuspended:
		return "Заблокирована"
	case StatusDeleted:
		return "Удалена"
	default:
		return string(s)
	}
}

// User — учётная запись.
type User struct {
	ID     uuid.UUID
	Role   Role
	Status UserStatus

	Email    string
	Phone    string
	FullName string

	AvatarURL string
	Locale    string

	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time

	FailedLoginCount int
	LockedUntil      *time.Time
	LastLoginAt      *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmailVerified сообщает, подтверждён ли адрес.
func (u *User) EmailVerified() bool { return u.EmailVerifiedAt != nil }

// PhoneVerified сообщает, подтверждён ли телефон.
func (u *User) PhoneVerified() bool { return u.PhoneVerifiedAt != nil }

// FullyVerified — оба контакта подтверждены, как требует ТЗ.
func (u *User) FullyVerified() bool { return u.EmailVerified() && u.PhoneVerified() }

// IsLocked сообщает, действует ли временная блокировка входа.
func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// VerifyChannel — канал подтверждения контакта.
type VerifyChannel string

const (
	ChannelEmail VerifyChannel = "email"
	ChannelPhone VerifyChannel = "phone"
)

func (c VerifyChannel) Valid() bool {
	return c == ChannelEmail || c == ChannelPhone
}

// LockoutDuration рассчитывает срок блокировки входа по числу неудач.
//
// Рост экспоненциальный, но с потолком: бесконечная блокировка превращается
// в инструмент отказа в обслуживании для чужого аккаунта — достаточно знать
// email жертвы и вводить неверные пароли.
func LockoutDuration(failedCount int, base, max time.Duration) time.Duration {
	const freeAttempts = 4
	if failedCount <= freeAttempts {
		return 0
	}

	duration := base
	for range failedCount - freeAttempts - 1 {
		duration *= 2
		if duration >= max {
			return max
		}
	}
	if duration > max {
		return max
	}
	return duration
}
