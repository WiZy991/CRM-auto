package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Origin — страна происхождения автомобиля.
type Origin string

const (
	OriginChina Origin = "cn"
	OriginJapan Origin = "jp"
)

func (o Origin) Valid() bool { return o == OriginChina || o == OriginJapan }

func (o Origin) Title() string {
	switch o {
	case OriginChina:
		return "Китай"
	case OriginJapan:
		return "Япония"
	default:
		return string(o)
	}
}

// CarStatus — состояние объявления.
type CarStatus string

const (
	CarDraft      CarStatus = "draft"
	CarModeration CarStatus = "moderation"
	CarActive     CarStatus = "active"
	CarReserved   CarStatus = "reserved"
	CarSold       CarStatus = "sold"
	CarArchived   CarStatus = "archived"
)

// PubliclyVisible сообщает, показывается ли объявление в каталоге.
//
// Зарезервированные машины остаются видны намеренно: клиент должен понимать,
// что автомобиль уже кто-то забрал, а не удивляться исчезновению карточки.
func (s CarStatus) PubliclyVisible() bool {
	return s == CarActive || s == CarReserved
}

func (s CarStatus) Valid() bool {
	switch s {
	case CarDraft, CarModeration, CarActive, CarReserved, CarSold, CarArchived:
		return true
	default:
		return false
	}
}

func (s CarStatus) Title() string {
	switch s {
	case CarDraft:
		return "Черновик"
	case CarModeration:
		return "На проверке"
	case CarActive:
		return "В продаже"
	case CarReserved:
		return "Забронирован"
	case CarSold:
		return "Продан"
	case CarArchived:
		return "В архиве"
	default:
		return string(s)
	}
}

// Перечисления характеристик автомобиля. Значения совпадают со
// значениями enum-типов в базе.
type (
	Transmission string
	Drivetrain   string
	BodyType     string
	FuelType     string
	Currency     string
)

const (
	TransmissionAT  Transmission = "at"
	TransmissionMT  Transmission = "mt"
	TransmissionCVT Transmission = "cvt"
	TransmissionDCT Transmission = "dct"
	TransmissionAMT Transmission = "amt"
)

const (
	DriveFWD Drivetrain = "fwd"
	DriveRWD Drivetrain = "rwd"
	DriveAWD Drivetrain = "awd"
)

const (
	BodySedan     BodyType = "sedan"
	BodySUV       BodyType = "suv"
	BodyCrossover BodyType = "crossover"
	BodyHatchback BodyType = "hatchback"
	BodyWagon     BodyType = "wagon"
	BodyCoupe     BodyType = "coupe"
	BodyMinivan   BodyType = "minivan"
	BodyPickup    BodyType = "pickup"
	BodyVan       BodyType = "van"
	BodyLiftback  BodyType = "liftback"
)

const (
	FuelPetrol   FuelType = "petrol"
	FuelDiesel   FuelType = "diesel"
	FuelHybrid   FuelType = "hybrid"
	FuelPHEV     FuelType = "phev"
	FuelElectric FuelType = "electric"
)

const (
	CurrencyRUB Currency = "rub"
	CurrencyUSD Currency = "usd"
	CurrencyCNY Currency = "cny"
	CurrencyJPY Currency = "jpy"
)

// Справочники значений и их названия для интерфейса.
//
// Названия живут на сервере, а не в коде фронтенда: список значений уже
// задан enum-типами в базе, и второй источник правды неизбежно разошёлся бы.
var (
	transmissionTitles = map[Transmission]string{
		TransmissionAT:  "Автомат",
		TransmissionMT:  "Механика",
		TransmissionCVT: "Вариатор",
		TransmissionDCT: "Робот с двумя сцеплениями",
		TransmissionAMT: "Робот",
	}
	drivetrainTitles = map[Drivetrain]string{
		DriveFWD: "Передний",
		DriveRWD: "Задний",
		DriveAWD: "Полный",
	}
	bodyTitles = map[BodyType]string{
		BodySedan:     "Седан",
		BodySUV:       "Внедорожник",
		BodyCrossover: "Кроссовер",
		BodyHatchback: "Хэтчбек",
		BodyWagon:     "Универсал",
		BodyCoupe:     "Купе",
		BodyMinivan:   "Минивэн",
		BodyPickup:    "Пикап",
		BodyVan:       "Фургон",
		BodyLiftback:  "Лифтбек",
	}
	fuelTitles = map[FuelType]string{
		FuelPetrol:   "Бензин",
		FuelDiesel:   "Дизель",
		FuelHybrid:   "Гибрид",
		FuelPHEV:     "Гибрид с зарядкой",
		FuelElectric: "Электро",
	}
	currencyTitles = map[Currency]string{
		CurrencyRUB: "Рубль",
		CurrencyUSD: "Доллар США",
		CurrencyCNY: "Юань",
		CurrencyJPY: "Иена",
	}
)

func (t Transmission) Valid() bool { _, ok := transmissionTitles[t]; return ok }
func (t Transmission) Title() string {
	if title, ok := transmissionTitles[t]; ok {
		return title
	}
	return string(t)
}

func (d Drivetrain) Valid() bool { _, ok := drivetrainTitles[d]; return ok }
func (d Drivetrain) Title() string {
	if title, ok := drivetrainTitles[d]; ok {
		return title
	}
	return string(d)
}

func (b BodyType) Valid() bool { _, ok := bodyTitles[b]; return ok }
func (b BodyType) Title() string {
	if title, ok := bodyTitles[b]; ok {
		return title
	}
	return string(b)
}

func (f FuelType) Valid() bool { _, ok := fuelTitles[f]; return ok }
func (f FuelType) Title() string {
	if title, ok := fuelTitles[f]; ok {
		return title
	}
	return string(f)
}

func (c Currency) Valid() bool { _, ok := currencyTitles[c]; return ok }
func (c Currency) Title() string {
	if title, ok := currencyTitles[c]; ok {
		return title
	}
	return string(c)
}

// Minor — число минорных единиц в одной единице валюты.
//
// У иены минорных единиц нет: 1000 иен — это ровно 1000, а не 10.00.
// Пропущенная деталь такого рода даёт ошибку цены в сто раз.
func (c Currency) Minor() int64 {
	if c == CurrencyJPY {
		return 1
	}
	return 100
}

// DictionaryEntry — элемент справочника для интерфейса.
type DictionaryEntry struct {
	Value string `json:"value"`
	Title string `json:"title"`
}

// CarDictionaries отдаёт все справочники каталога одним ответом.
//
// Один запрос вместо шести: фронтенд получает справочники при старте и
// кеширует, а не дёргает сервер на каждый фильтр.
func CarDictionaries() map[string][]DictionaryEntry {
	return map[string][]DictionaryEntry{
		"origin": {
			{Value: string(OriginChina), Title: OriginChina.Title()},
			{Value: string(OriginJapan), Title: OriginJapan.Title()},
		},
		"transmission": dictionary(transmissionTitles, []Transmission{
			TransmissionAT, TransmissionCVT, TransmissionDCT, TransmissionAMT, TransmissionMT,
		}),
		"drive": dictionary(drivetrainTitles, []Drivetrain{DriveAWD, DriveFWD, DriveRWD}),
		"body": dictionary(bodyTitles, []BodyType{
			BodyCrossover, BodySUV, BodySedan, BodyHatchback, BodyLiftback,
			BodyWagon, BodyCoupe, BodyMinivan, BodyPickup, BodyVan,
		}),
		"fuel": dictionary(fuelTitles, []FuelType{
			FuelPetrol, FuelHybrid, FuelPHEV, FuelElectric, FuelDiesel,
		}),
		"currency": dictionary(currencyTitles, []Currency{
			CurrencyRUB, CurrencyCNY, CurrencyJPY, CurrencyUSD,
		}),
		"car_status": {
			{Value: string(CarActive), Title: CarActive.Title()},
			{Value: string(CarReserved), Title: CarReserved.Title()},
			{Value: string(CarSold), Title: CarSold.Title()},
			{Value: string(CarDraft), Title: CarDraft.Title()},
			{Value: string(CarModeration), Title: CarModeration.Title()},
			{Value: string(CarArchived), Title: CarArchived.Title()},
		},
	}
}

// dictionary сохраняет заданный порядок значений: справочники показываются
// от самых частых к редким, а не по алфавиту.
func dictionary[T ~string](titles map[T]string, order []T) []DictionaryEntry {
	out := make([]DictionaryEntry, 0, len(order))
	for _, value := range order {
		out = append(out, DictionaryEntry{Value: string(value), Title: titles[value]})
	}
	return out
}

// CarPhoto — фотография объявления.
type CarPhoto struct {
	ID        uuid.UUID `json:"id"`
	URL       string    `json:"url"`
	ThumbURL  string    `json:"thumb_url,omitempty"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	SortOrder int       `json:"sort_order"`
}

// Car — объявление о продаже автомобиля.
type Car struct {
	ID       uuid.UUID
	DealerID *uuid.UUID
	SellerID *uuid.UUID

	Status CarStatus
	Origin Origin

	Brand      string
	Model      string
	Generation string
	TrimLevel  string
	Year       int

	MileageKM int
	EngineCC  *int
	PowerHP   *int

	Fuel          FuelType
	Gearbox       Transmission
	Drive         Drivetrain
	Body          BodyType
	Color         string
	Seats         *int
	SteeringRight bool

	AuctionGrade     string
	InteriorGrade    string
	AuctionLotNumber string
	AuctionDate      *time.Time

	VIN        string
	VINVisible bool

	PriceMinor      int64
	Currency        Currency
	PriceRubMinor   int64
	TurnkeyRubMinor *int64
	CustomsRubMinor *int64
	DeliveryDays    *int

	Title       string
	Description string
	Equipment   []string

	ViewsCount    int
	RequestsCount int

	Photos []CarPhoto

	PublishedAt *time.Time
	SoldAt      *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DisplayName — заголовок карточки, собранный из характеристик.
func (c *Car) DisplayName() string {
	parts := []string{c.Brand, c.Model}
	if c.Generation != "" {
		parts = append(parts, c.Generation)
	}
	parts = append(parts, fmt.Sprintf("%d г.", c.Year))
	return strings.Join(parts, " ")
}

// MaskedVIN отдаёт VIN с закрытой серединой.
//
// Полный VIN — это доступ к истории автомобиля и предмет торговли на рынке
// перекупов, поэтому в публичном каталоге он показывается частично. Полное
// значение видят только участники сделки по этой машине.
func (c *Car) MaskedVIN() string {
	if c.VIN == "" {
		return ""
	}
	if len(c.VIN) <= 6 {
		return strings.Repeat("*", len(c.VIN))
	}
	return c.VIN[:4] + strings.Repeat("*", len(c.VIN)-6) + c.VIN[len(c.VIN)-2:]
}

// VINFor возвращает VIN с учётом прав запрашивающего.
func (c *Car) VINFor(canSeeFull bool) string {
	if canSeeFull || c.VINVisible {
		return c.VIN
	}
	return c.MaskedVIN()
}

// OwnedBy проверяет принадлежность объявления дилеру.
//
// Используется как вторая линия защиты от обращения к чужим данным:
// первая — условие dealer_id в самом SQL-запросе.
func (c *Car) OwnedBy(dealerID uuid.UUID) bool {
	return c.DealerID != nil && *c.DealerID == dealerID
}

// CanBePublished проверяет готовность объявления к публикации.
//
// Требования продиктованы качеством каталога: объявление без фотографий и
// описания бесполезно для покупателя и портит выдачу.
func (c *Car) CanBePublished() error {
	if len(c.Photos) < 3 {
		return fmt.Errorf("для публикации нужно не менее трёх фотографий, загружено %d", len(c.Photos))
	}
	if len([]rune(c.Description)) < 80 {
		return fmt.Errorf("описание слишком короткое: нужно не менее 80 символов")
	}
	if c.PriceMinor <= 0 {
		return fmt.Errorf("укажите цену автомобиля")
	}
	if c.MileageKM == 0 && c.Year < time.Now().Year() {
		return fmt.Errorf("укажите пробег: нулевой пробег возможен только у новых автомобилей")
	}
	return nil
}

// ValidateCarStatusTransition проверяет смену статуса объявления.
//
// Таблица переходов не даёт, например, вернуть проданный автомобиль в
// продажу: у сделки уже есть история, и такое изменение исказило бы и
// статистику дилера, и отчётность.
func ValidateCarStatusTransition(from, to CarStatus) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("неизвестный статус объявления")
	}
	if from == to {
		return fmt.Errorf("объявление уже в этом статусе")
	}

	allowed := map[CarStatus][]CarStatus{
		CarDraft:      {CarModeration, CarArchived},
		CarModeration: {CarActive, CarDraft, CarArchived},
		CarActive:     {CarReserved, CarSold, CarArchived, CarDraft},
		CarReserved:   {CarActive, CarSold, CarArchived},
		CarSold:       {CarArchived},
		CarArchived:   {CarDraft},
	}

	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}
	return fmt.Errorf("переход из статуса «%s» в «%s» недопустим", from.Title(), to.Title())
}
