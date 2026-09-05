package docs

import (
	"fmt"
	"strings"
	"time"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/store"
)

const missing = "не указано"

// Party — сторона сделки в печатной форме.
type Party struct {
	Name     string
	Legal    string
	INN      string
	Passport string
	Address  string
	Phone    string
	Email    string
	City     string
}

// Vehicle — автомобиль, если он уже привязан к сделке.
type Vehicle struct {
	Title    string
	Brand    string
	Model    string
	Year     string
	VIN      string
	Origin   string
	Mileage  string
	Engine   string
	Power    string
	Fuel     string
	Gearbox  string
	Color    string
	Steering string
	Auction  string
}

// Payload — данные для автозаполнения комплекта документов.
type Payload struct {
	Kind  string
	Title string
	Date  string
	Now   time.Time

	DealNumber string
	DealTitle  string
	Stage      string
	Created    string
	Handover   string
	Note       string

	Amount        string
	AmountWords   string
	Paid          string
	Remainder     string
	Currency      string
	PaidShare     string

	Client Party
	Dealer Party
	Seller string

	Car Vehicle

	Services string
	Warnings []string
}

// FromSources собирает печатные поля из сделки и связанных карточек.
func FromSources(
	kind store.DocumentKind,
	deal *domain.Deal,
	client *store.UserRecord,
	dealerUser *store.UserRecord,
	dealer *store.DealerProfile,
	car *domain.Car,
	seller *domain.Seller,
	clientPassport, clientAddress string,
	now time.Time,
) Payload {
	p := Payload{
		Kind:       string(kind),
		Title:      kind.Title(),
		Date:       now.Format("02.01.2006"),
		Now:        now,
		DealNumber: fmt.Sprintf("%d", deal.PublicNumber),
		DealTitle:  dash(deal.Title),
		Stage:      deal.Stage.Title(),
		Created:    deal.CreatedAt.Format("02.01.2006"),
		Note:       dash(deal.ManagerNote),
	}
	if deal.ExpectedHandoverAt != nil {
		p.Handover = deal.ExpectedHandoverAt.Format("02.01.2006")
	} else {
		p.Handover = missing
	}

	amountMinor := int64(0)
	if deal.AmountRubMinor != nil {
		amountMinor = *deal.AmountRubMinor
	} else if deal.AmountMinor != nil && deal.Currency == domain.CurrencyRUB {
		amountMinor = *deal.AmountMinor
	}
	p.Amount = money.FormatRub(amountMinor)
	p.AmountWords = RublesInWords(amountMinor)
	p.Paid = money.FormatRub(deal.PaidRubMinor)
	remain := amountMinor - deal.PaidRubMinor
	if remain < 0 {
		remain = 0
	}
	p.Remainder = money.FormatRub(remain)
	p.Currency = strings.ToUpper(string(deal.Currency))
	if p.Currency == "" {
		p.Currency = "RUB"
	}
	if amountMinor > 0 {
		p.PaidShare = fmt.Sprintf("%d%%", deal.PaidRubMinor*100/amountMinor)
	} else {
		p.PaidShare = "0%"
	}

	if client != nil {
		p.Client = Party{
			Name:     dash(client.FullName),
			Passport: dash(clientPassport),
			Address:  dash(clientAddress),
			Phone:    dash(client.Phone),
			Email:    dash(client.Email),
		}
	} else {
		p.Client = emptyParty()
	}

	p.Dealer = Party{
		Name:    missing,
		Legal:   missing,
		INN:     missing,
		Address: missing,
		Phone:   missing,
		Email:   missing,
		City:    missing,
	}
	if dealer != nil {
		p.Dealer.Name = dash(dealer.CompanyName)
		p.Dealer.Legal = first(dealer.LegalName, dealer.CompanyName)
		p.Dealer.INN = dash(dealer.INN)
		p.Dealer.Address = dash(dealer.Address)
		p.Dealer.City = dash(dealer.City)
		p.Services = strings.Join(dealer.Services, ", ")
		if p.Services == "" {
			p.Services = missing
		}
	}
	if dealerUser != nil {
		if p.Dealer.Name == missing {
			p.Dealer.Name = dash(dealerUser.FullName)
		}
		p.Dealer.Phone = dash(dealerUser.Phone)
		p.Dealer.Email = dash(dealerUser.Email)
	}

	if seller != nil {
		parts := []string{seller.Name, seller.Kind.Title(), seller.Country.Title()}
		if seller.City != "" {
			parts = append(parts, seller.City)
		}
		p.Seller = strings.Join(parts, ", ")
	} else {
		p.Seller = missing
	}

	if car != nil {
		engine := missing
		if car.EngineCC != nil {
			engine = fmt.Sprintf("%d см³", *car.EngineCC)
		}
		power := missing
		if car.PowerHP != nil {
			power = fmt.Sprintf("%d л.с.", *car.PowerHP)
		}
		steering := "левый"
		if car.SteeringRight {
			steering = "правый"
		}
		p.Car = Vehicle{
			Title:    dash(car.DisplayName()),
			Brand:    dash(car.Brand),
			Model:    dash(car.Model),
			Year:     fmt.Sprintf("%d", car.Year),
			VIN:      dash(car.VIN),
			Origin:   car.Origin.Title(),
			Mileage:  formatInt(int64(car.MileageKM)) + " км",
			Engine:   engine,
			Power:    power,
			Fuel:     car.Fuel.Title(),
			Gearbox:  car.Gearbox.Title(),
			Color:    dash(car.Color),
			Steering: steering,
			Auction:  dash(strings.TrimSpace(car.AuctionGrade + " " + car.AuctionLotNumber)),
		}
	} else {
		p.Car = Vehicle{
			Title: dash(deal.Title), Brand: missing, Model: missing, Year: missing,
			VIN: missing, Origin: missing, Mileage: missing, Engine: missing,
			Power: missing, Fuel: missing, Gearbox: missing, Color: missing,
			Steering: missing, Auction: missing,
		}
	}

	if clientPassport == "" {
		p.Warnings = append(p.Warnings, "В профиле клиента нет паспортных данных")
	}
	if clientAddress == "" {
		p.Warnings = append(p.Warnings, "В профиле клиента нет адреса")
	}
	if dealer == nil || dealer.INN == "" {
		p.Warnings = append(p.Warnings, "У дилера не заполнен ИНН")
	}
	if amountMinor == 0 {
		p.Warnings = append(p.Warnings, "В сделке не указана сумма")
	}
	if car == nil {
		p.Warnings = append(p.Warnings, "К сделке не привязан автомобиль — в формах стоит название сделки")
	}

	return p
}

func emptyParty() Party {
	return Party{
		Name: missing, Legal: missing, INN: missing, Passport: missing,
		Address: missing, Phone: missing, Email: missing, City: missing,
	}
}

func dash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return missing
	}
	return value
}

func formatInt(value int64) string {
	if value < 0 {
		return "−" + formatInt(-value)
	}
	digits := fmt.Sprintf("%d", value)
	if len(digits) <= 3 {
		return digits
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	return strings.Join(parts, "\u202f")
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return missing
}
