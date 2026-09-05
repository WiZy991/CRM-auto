package domain

import (
	"testing"
	"time"
)

func TestRoleRegistrable(t *testing.T) {
	// Ключевое требование: администратора нельзя получить самостоятельной
	// регистрацией. Тест фиксирует это, чтобы правило нельзя было ослабить
	// незаметно.
	if RoleAdmin.Registrable() {
		t.Fatal("роль администратора не должна быть доступна при регистрации")
	}

	for _, role := range []Role{RoleClient, RoleDealer, RoleSeller} {
		if !role.Registrable() {
			t.Errorf("роль %s должна быть доступна при регистрации", role)
		}
	}
}

func TestParseRole(t *testing.T) {
	if role, err := ParseRole("  DEALER "); err != nil || role != RoleDealer {
		t.Errorf("ParseRole должен приводить регистр и обрезать пробелы, получено %q, %v", role, err)
	}
	if _, err := ParseRole("superadmin"); err == nil {
		t.Error("неизвестная роль должна отклоняться")
	}
	if _, err := ParseRole(""); err == nil {
		t.Error("пустая роль должна отклоняться")
	}
}

func TestLockoutDurationEscalatesAndCaps(t *testing.T) {
	base := time.Minute
	maxLockout := 30 * time.Minute

	// Первые попытки не блокируют: человек имеет право на опечатку.
	for attempts := 1; attempts <= 4; attempts++ {
		if got := LockoutDuration(attempts, base, maxLockout); got != 0 {
			t.Errorf("после %d неудач блокировки быть не должно, получено %s", attempts, got)
		}
	}

	if got := LockoutDuration(5, base, maxLockout); got != base {
		t.Errorf("на пятой неудаче ожидалась базовая блокировка %s, получено %s", base, got)
	}
	if got := LockoutDuration(6, base, maxLockout); got != 2*time.Minute {
		t.Errorf("блокировка должна удваиваться, ожидалось 2m, получено %s", got)
	}
	if got := LockoutDuration(7, base, maxLockout); got != 4*time.Minute {
		t.Errorf("ожидалось 4m, получено %s", got)
	}

	// Потолок обязателен: бессрочная блокировка превращается в способ
	// заблокировать чужой аккаунт, зная только его email.
	if got := LockoutDuration(50, base, maxLockout); got != maxLockout {
		t.Errorf("блокировка должна ограничиваться значением %s, получено %s", maxLockout, got)
	}
}

func TestUserVerificationState(t *testing.T) {
	now := time.Now()
	user := &User{}

	if user.FullyVerified() {
		t.Error("новый пользователь не может быть полностью подтверждён")
	}

	user.EmailVerifiedAt = &now
	if user.FullyVerified() {
		t.Error("подтверждения только email недостаточно")
	}

	user.PhoneVerifiedAt = &now
	if !user.FullyVerified() {
		t.Error("при подтверждённых email и телефоне ожидается полное подтверждение")
	}
}

func TestUserIsLocked(t *testing.T) {
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	future := now.Add(5 * time.Minute)
	past := now.Add(-5 * time.Minute)

	user := &User{LockedUntil: &future}
	if !user.IsLocked(now) {
		t.Error("блокировка в будущем должна действовать")
	}

	user.LockedUntil = &past
	if user.IsLocked(now) {
		t.Error("истёкшая блокировка не должна действовать")
	}

	user.LockedUntil = nil
	if user.IsLocked(now) {
		t.Error("без срока блокировки аккаунт не заблокирован")
	}
}

func TestUserStatusCanSignIn(t *testing.T) {
	cases := map[UserStatus]bool{
		StatusActive:    true,
		StatusPending:   true,
		StatusSuspended: false,
		StatusDeleted:   false,
	}
	for status, want := range cases {
		if got := status.CanSignIn(); got != want {
			t.Errorf("статус %s: CanSignIn() = %v, ожидалось %v", status, got, want)
		}
	}
}
