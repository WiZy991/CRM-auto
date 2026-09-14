package docs

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// FieldTitle — человекочитаемое имя ключа Payload.
func FieldTitle(key string) string {
	return fieldTitles[key]
}

// FieldCatalog — ключи Payload, доступные для маппинга шаблона.
func FieldCatalog() []DictionaryField {
	keys := make([]string, 0, len(fieldTitles))
	for k := range fieldTitles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]DictionaryField, 0, len(keys))
	for _, k := range keys {
		out = append(out, DictionaryField{Key: k, Title: fieldTitles[k]})
	}
	return out
}

// DictionaryField — пункт справочника полей.
type DictionaryField struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

var fieldTitles = map[string]string{
	"deal.number":       "Номер сделки",
	"deal.title":        "Название сделки",
	"deal.stage":        "Этап",
	"deal.created":      "Дата создания",
	"deal.handover":     "Плановая выдача",
	"deal.note":         "Заметка",
	"deal.date":         "Дата документа",
	"deal.amount":       "Сумма",
	"deal.amount_words": "Сумма прописью",
	"deal.paid":         "Оплачено",
	"deal.remainder":    "Остаток",
	"deal.currency":     "Валюта",
	"deal.paid_share":   "Доля оплаты",
	"deal.services":     "Услуги",
	"client.name":       "Клиент: ФИО",
	"client.legal":      "Клиент: юр. название",
	"client.inn":        "Клиент: ИНН",
	"client.passport":   "Клиент: паспорт",
	"client.address":    "Клиент: адрес",
	"client.phone":      "Клиент: телефон",
	"client.email":      "Клиент: email",
	"client.city":       "Клиент: город",
	"dealer.name":       "Дилер: ФИО",
	"dealer.legal":      "Дилер: компания",
	"dealer.inn":        "Дилер: ИНН",
	"dealer.passport":   "Дилер: паспорт",
	"dealer.address":    "Дилер: адрес",
	"dealer.phone":      "Дилер: телефон",
	"dealer.email":      "Дилер: email",
	"dealer.city":       "Дилер: город",
	"seller.name":       "Поставщик",
	"car.title":         "Авто: название",
	"car.brand":         "Авто: марка",
	"car.model":         "Авто: модель",
	"car.year":          "Авто: год",
	"car.vin":           "Авто: VIN",
	"car.origin":        "Авто: происхождение",
	"car.mileage":       "Авто: пробег",
	"car.engine":        "Авто: двигатель",
	"car.power":         "Авто: мощность",
	"car.fuel":          "Авто: топливо",
	"car.gearbox":       "Авто: КПП",
	"car.color":         "Авто: цвет",
	"car.steering":      "Авто: руль",
	"car.auction":       "Авто: аукцион",
}

// ValuesFromPayload раскладывает Payload в плоскую карту ключ → значение.
func ValuesFromPayload(p Payload) map[string]string {
	return map[string]string{
		"deal.number":       p.DealNumber,
		"deal.title":        p.DealTitle,
		"deal.stage":        p.Stage,
		"deal.created":      p.Created,
		"deal.handover":     p.Handover,
		"deal.note":         p.Note,
		"deal.date":         p.Date,
		"deal.amount":       p.Amount,
		"deal.amount_words": p.AmountWords,
		"deal.paid":         p.Paid,
		"deal.remainder":    p.Remainder,
		"deal.currency":     p.Currency,
		"deal.paid_share":   p.PaidShare,
		"deal.services":     p.Services,
		"client.name":       p.Client.Name,
		"client.legal":      p.Client.Legal,
		"client.inn":        p.Client.INN,
		"client.passport":   p.Client.Passport,
		"client.address":    p.Client.Address,
		"client.phone":      p.Client.Phone,
		"client.email":      p.Client.Email,
		"client.city":       p.Client.City,
		"dealer.name":       p.Dealer.Name,
		"dealer.legal":      p.Dealer.Legal,
		"dealer.inn":        p.Dealer.INN,
		"dealer.passport":   p.Dealer.Passport,
		"dealer.address":    p.Dealer.Address,
		"dealer.phone":      p.Dealer.Phone,
		"dealer.email":      p.Dealer.Email,
		"dealer.city":       p.Dealer.City,
		"seller.name":       p.Seller,
		"car.title":         p.Car.Title,
		"car.brand":         p.Car.Brand,
		"car.model":         p.Car.Model,
		"car.year":          p.Car.Year,
		"car.vin":           p.Car.VIN,
		"car.origin":        p.Car.Origin,
		"car.mileage":       p.Car.Mileage,
		"car.engine":        p.Car.Engine,
		"car.power":         p.Car.Power,
		"car.fuel":          p.Car.Fuel,
		"car.gearbox":       p.Car.Gearbox,
		"car.color":         p.Car.Color,
		"car.steering":      p.Car.Steering,
		"car.auction":       p.Car.Auction,
	}
}

var (
	reMustache = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
	reGuillemet = regexp.MustCompile(`«([^»]{2,80})»`)
	reBracket   = regexp.MustCompile(`\[([^\]]{2,80})\]`)
)

// ExtractMarkers находит плейсхолдеры в тексте DOCX (после склейки XML).
func ExtractMarkers(text string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	for _, m := range reMustache.FindAllStringSubmatch(text, -1) {
		add("{{" + m[1] + "}}")
	}
	// «…» и […] только если это известный ярлык поля — иначе цепляем цитаты из договора.
	for _, m := range reGuillemet.FindAllStringSubmatch(text, -1) {
		marker := "«" + m[1] + "»"
		if guessKey(marker) != "" {
			add(marker)
		}
	}
	for _, m := range reBracket.FindAllStringSubmatch(text, -1) {
		marker := "[" + m[1] + "]"
		if guessKey(marker) != "" {
			add(marker)
		}
	}
	sort.Strings(out)
	return out
}

// AutoMap сопоставляет маркеры с ключами Payload по словарю синонимов.
func AutoMap(markers []string) map[string]string {
	out := make(map[string]string, len(markers))
	for _, marker := range markers {
		if key := guessKey(marker); key != "" {
			out[marker] = key
		}
	}
	return out
}

func guessKey(marker string) string {
	inner := strings.TrimSpace(marker)
	inner = strings.TrimPrefix(inner, "{{")
	inner = strings.TrimSuffix(inner, "}}")
	inner = strings.Trim(inner, "«»[]")
	inner = strings.TrimSpace(inner)
	lower := strings.ToLower(inner)
	lower = strings.ReplaceAll(lower, " ", ".")
	if _, ok := fieldTitles[lower]; ok {
		return lower
	}
	norm := normalizeLabel(inner)
	for key, synonyms := range synonymIndex {
		for _, syn := range synonyms {
			if normalizeLabel(syn) == norm {
				return key
			}
		}
	}
	return ""
}

func normalizeLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var synonymIndex = map[string][]string{
	"client.name":     {"фио", "фио клиента", "покупатель", "заказчик", "клиент", "ф.и.о."},
	"client.passport": {"паспорт", "паспортные данные", "серия и номер паспорта"},
	"client.address":  {"адрес", "адрес регистрации", "прописка"},
	"client.phone":    {"телефон", "тел", "мобильный"},
	"client.email":    {"email", "e-mail", "почта"},
	"client.inn":      {"инн клиента", "инн покупателя"},
	"dealer.legal":    {"исполнитель", "продавец", "компания", "дилер", "импортёр", "наименование"},
	"dealer.inn":      {"инн", "инн дилера", "инн исполнителя"},
	"dealer.phone":    {"телефон дилера", "телефон компании"},
	"dealer.address":  {"адрес дилера", "юридический адрес"},
	"deal.amount":     {"сумма", "стоимость", "цена", "сумма договора"},
	"deal.amount_words": {"сумма прописью", "прописью"},
	"deal.number":     {"номер договора", "номер сделки", "№ договора", "номер"},
	"deal.date":       {"дата", "дата договора", "дата документа"},
	"deal.paid":       {"оплачено", "внесено"},
	"deal.remainder":  {"остаток", "к доплате"},
	"car.vin":         {"vin", "вин", "номер кузова"},
	"car.brand":       {"марка", "бренд"},
	"car.model":       {"модель"},
	"car.year":        {"год", "год выпуска"},
	"car.mileage":     {"пробег"},
	"car.title":       {"автомобиль", "транспортное средство", "тс"},
	"seller.name":     {"поставщик", "аукцион", "экспортёр"},
}
