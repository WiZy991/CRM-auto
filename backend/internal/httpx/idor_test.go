package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

func TestErrorMapsNotFoundTo404(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals/"+uuid.NewString(), nil)

	Error(rec, req, apierr.NotFound("Сделка"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, ожидалось 404", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if body.Error.Code != string(apierr.CodeNotFound) {
		t.Fatalf("code = %q, ожидалось not_found", body.Error.Code)
	}
}

func TestErrorMapsForbiddenTo403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deals/"+uuid.NewString()+"/stage", nil)

	Error(rec, req, apierr.Forbidden("Изменять сделку может только дилер"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, ожидалось 403", rec.Code)
	}
}
