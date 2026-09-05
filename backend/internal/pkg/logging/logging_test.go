package logging

import (
	"log/slog"
	"testing"
)

func TestRedactMasksPassport(t *testing.T) {
	got := redact(nil, slog.String("passport", "4500 123456"))
	if got.Value.String() != "[REDACTED]" {
		t.Fatalf("passport в логе = %q", got.Value.String())
	}

	email := redact(nil, slog.String("email", "ivanov@mail.ru"))
	if email.Value.String() == "ivanov@mail.ru" {
		t.Fatal("email не должен попадать в лог целиком")
	}
}
