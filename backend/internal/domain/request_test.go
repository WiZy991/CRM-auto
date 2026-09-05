package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseContactPreference(t *testing.T) {
	cases := []struct {
		raw     string
		want    ContactPreference
		wantErr bool
	}{
		// Пустое поле не ошибка: форма может его не показывать, и звонок —
		// разумный вариант по умолчанию.
		{raw: "", want: ContactPhone},
		{raw: "   ", want: ContactPhone},
		{raw: "phone", want: ContactPhone},
		{raw: "EMAIL", want: ContactEmail},
		{raw: " Messenger ", want: ContactMessenger},
		{raw: "telegram", wantErr: true},
		{raw: "phone; DROP TABLE users", wantErr: true},
	}

	for _, testCase := range cases {
		got, err := ParseContactPreference(testCase.raw)
		if testCase.wantErr {
			if err == nil {
				t.Errorf("ParseContactPreference(%q) должна была вернуть ошибку", testCase.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseContactPreference(%q) вернула ошибку: %v", testCase.raw, err)
			continue
		}
		if got != testCase.want {
			t.Errorf("ParseContactPreference(%q) = %q, ожидалось %q", testCase.raw, got, testCase.want)
		}
	}
}

func TestRequestCanConvertToDeal(t *testing.T) {
	dealerID := uuid.New()

	unassigned := &Request{Status: RequestNew}
	if err := unassigned.CanConvertToDeal(); err == nil {
		t.Error("незакреплённая заявка не должна превращаться в сделку")
	}

	assigned := &Request{Status: RequestInProgress, DealerID: &dealerID}
	if err := assigned.CanConvertToDeal(); err != nil {
		t.Errorf("заявка в работе должна конвертироваться: %v", err)
	}

	for _, status := range []RequestStatus{RequestConverted, RequestRejected, RequestClosed} {
		finished := &Request{Status: status, DealerID: &dealerID}
		if err := finished.CanConvertToDeal(); err == nil {
			t.Errorf("заявка в состоянии %q не должна конвертироваться", status)
		}
	}
}

func TestRequestInvolves(t *testing.T) {
	clientID := uuid.New()
	dealerID := uuid.New()
	stranger := uuid.New()

	request := &Request{ClientID: clientID, DealerID: &dealerID}

	if !request.Involves(clientID) {
		t.Error("клиент должен считаться участником заявки")
	}
	if !request.Involves(dealerID) {
		t.Error("дилер должен считаться участником заявки")
	}
	if request.Involves(stranger) {
		t.Error("посторонний не должен считаться участником заявки")
	}

	// Заявка в общем пуле не закреплена ни за кем, и проверка участия не
	// должна разыменовывать пустой указатель.
	pooled := &Request{ClientID: clientID}
	if pooled.Involves(stranger) {
		t.Error("нераспределённая заявка не относится к постороннему")
	}
	if !pooled.InOpenPool() && pooled.Status == RequestNew {
		t.Error("заявка без дилера в статусе new должна быть в общем пуле")
	}
}

func TestRequestSummary(t *testing.T) {
	origin := OriginJapan
	body := BodySUV
	yearFrom, yearTo := 2018, 2021

	cases := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name:    "марка и модель с диапазоном лет",
			request: Request{DesiredBrand: "Toyota", DesiredModel: "Land Cruiser", YearFrom: &yearFrom, YearTo: &yearTo},
			want:    "Toyota Land Cruiser, 2018–2021",
		},
		{
			name:    "только марка",
			request: Request{DesiredBrand: "Zeekr"},
			want:    "Zeekr",
		},
		{
			name:    "только страна",
			request: Request{Origin: &origin},
			want:    "Автомобиль из Япония",
		},
		{
			name:    "ничего не указано",
			request: Request{},
			want:    "Подбор автомобиля",
		},
		{
			name:    "нижняя граница года и кузов",
			request: Request{DesiredBrand: "Haval", YearFrom: &yearFrom, Body: &body},
			want:    "Haval, от 2018, внедорожник",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.request.Summary(); got != testCase.want {
				t.Errorf("Summary() = %q, ожидалось %q", got, testCase.want)
			}
		})
	}
}

func TestRequestStatusIsFinal(t *testing.T) {
	final := []RequestStatus{RequestConverted, RequestRejected, RequestClosed}
	for _, status := range final {
		if !status.IsFinal() {
			t.Errorf("статус %q должен быть конечным", status)
		}
	}

	open := []RequestStatus{RequestNew, RequestInProgress, RequestAnswered}
	for _, status := range open {
		if status.IsFinal() {
			t.Errorf("статус %q не должен быть конечным", status)
		}
	}
}
