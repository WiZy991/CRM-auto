package service

import (
	"net/http"
	"testing"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

func TestDealAccessCanManage(t *testing.T) {
	dealer := DealAccess{IsDealer: true}
	if !dealer.CanManage() {
		t.Error("дилер управляет сделкой")
	}

	admin := DealAccess{IsAdmin: true}
	if !admin.CanManage() {
		t.Error("админ управляет сделкой")
	}

	client := DealAccess{IsClient: true}
	if client.CanManage() {
		t.Error("клиент не управляет сделкой")
	}
}

func TestStrangerLooksLikeMissing(t *testing.T) {
	err := apierr.NotFound("Сделка")
	if err.Status != http.StatusNotFound {
		t.Fatalf("чужой объект отдаётся как %d, нужно 404", err.Status)
	}
	if err.Status == http.StatusForbidden {
		t.Fatal("нельзя отвечать 403 на чужой UUID: это подтверждает существование")
	}
}
