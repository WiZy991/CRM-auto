package apierr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestNotFoundIs404Not403(t *testing.T) {
	// Чужой UUID и отсутствующий объект неотличимы снаружи.
	err := NotFound("Сделка")
	if err.Status != http.StatusNotFound {
		t.Fatalf("NotFound.Status = %d, ожидалось 404", err.Status)
	}
	if err.Code != CodeNotFound {
		t.Fatalf("NotFound.Code = %q", err.Code)
	}

	forbidden := Forbidden("Изменять сделку может только дилер")
	if forbidden.Status != http.StatusForbidden {
		t.Fatalf("Forbidden.Status = %d, ожидалось 403", forbidden.Status)
	}
	if forbidden.Status == err.Status {
		t.Fatal("404 и 403 не должны совпадать")
	}
}

func TestFromCanceledIsNot500(t *testing.T) {
	got := From(fmt.Errorf("подсчёт сделок: %w", context.Canceled))
	if got.Status != StatusClientClosed {
		t.Fatalf("status = %d, ожидалось 499", got.Status)
	}
	if got.Code != CodeCanceled {
		t.Fatalf("code = %q", got.Code)
	}

	wrapped := Internal(errors.Join(errors.New("чтение сделки"), context.Canceled))
	got = From(wrapped)
	if got.Status != StatusClientClosed {
		t.Fatalf("Internal(canceled).Status = %d", got.Status)
	}
}
