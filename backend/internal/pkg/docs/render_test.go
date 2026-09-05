package docs

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/store"
)

func TestRublesInWords(t *testing.T) {
	got := RublesInWords(1_234_00)
	if !strings.Contains(got, "рубл") || !strings.Contains(got, "00 коп.") {
		t.Fatalf("неожиданная пропись: %s", got)
	}
}

func TestRenderFillsClientAndCar(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	amount := int64(2_500_000_00)
	deal := &domain.Deal{
		ID:             uuid.New(),
		PublicNumber:   1042,
		Title:          "Toyota Land Cruiser 300",
		AmountRubMinor: &amount,
		Currency:       domain.CurrencyRUB,
		Stage:          domain.StageContract,
		CreatedAt:      now,
	}
	client := &store.UserRecord{User: domain.User{
		FullName: "Иванов Иван Иванович",
		Email:    "ivan@local.test",
		Phone:    "+79990001122",
	}}
	dealerUser := &store.UserRecord{User: domain.User{
		FullName: "Дилер",
		Email:    "dealer@local.test",
		Phone:    "+79990000000",
	}}
	dealer := &store.DealerProfile{
		PublicDealer: store.PublicDealer{CompanyName: "Восток Авто", City: "Владивосток", Services: []string{"подбор", "растаможка"}},
		LegalName:    "ООО Восток Авто",
		INN:          "2540001111",
		Address:      "г. Владивосток, ул. Морская, 1",
	}
	car := &domain.Car{Brand: "Toyota", Model: "Land Cruiser", Year: 2021, VIN: "JTMCY7AJ4N4123456", Origin: domain.OriginJapan, MileageKM: 24000, Color: "белый"}

	payload := FromSources(store.DocContract, deal, client, dealerUser, dealer, car, nil, "4510 123456", "г. Москва, ул. Ленина, 1", now)
	if payload.Client.Name != "Иванов Иван Иванович" {
		t.Fatalf("имя клиента: %s", payload.Client.Name)
	}

	for _, kind := range PrintableKinds() {
		html, err := Render(kind, payload)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		body := string(html)
		for _, needle := range []string{"Иванов Иван Иванович", "1042", "JTMCY7AJ4N4123456", "ООО Восток Авто"} {
			if !strings.Contains(body, needle) {
				t.Fatalf("%s: нет %q", kind, needle)
			}
		}
	}
}
