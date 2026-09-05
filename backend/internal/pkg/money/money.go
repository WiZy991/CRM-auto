// Package money работает с денежными суммами.
//
// Все суммы — целые числа минорных единиц (копейки, фэни, иены). Ни в одном
// месте платформы деньги не хранятся и не считаются в float: 0.1 + 0.2 в
// двоичной плавающей точке не равно 0.3, и на сумме сделки в несколько
// миллионов это превращается в расхождение в отчётности.
package money

import (
	"fmt"
	"strings"
	"sync"

	"github.com/autoimport/crm/internal/domain"
)

// Converter пересчитывает суммы в рубли.
//
// Курс хранится как число копеек за одну единицу иностранной валюты. Такое
// представление позволяет считать в целых числах: сумма в минорных единицах,
// умноженная на курс, делится на число минорных единиц исходной валюты.
type Converter struct {
	mu    sync.RWMutex
	rates map[domain.Currency]int64
}

// Курсы по умолчанию — ориентировочные, для работы в разработке.
//
// В production их обновляет фоновая задача из внешнего источника: цена в
// каталоге, посчитанная по курсу полугодовой давности, вводит покупателя в
// заблуждение и создаёт спор при заключении договора.
var defaultRates = map[domain.Currency]int64{
	domain.CurrencyRUB: 100,   // 1 рубль = 100 копеек
	domain.CurrencyUSD: 8_150, // 1 доллар ≈ 81.50 рубля
	domain.CurrencyCNY: 1_140, // 1 юань ≈ 11.40 рубля
	domain.CurrencyJPY: 55,    // 1 иена ≈ 0.55 рубля
}

func NewConverter() *Converter {
	rates := make(map[domain.Currency]int64, len(defaultRates))
	for currency, rate := range defaultRates {
		rates[currency] = rate
	}
	return &Converter{rates: rates}
}

// SetRate обновляет курс. Значение — копейки за единицу валюты.
func (c *Converter) SetRate(currency domain.Currency, kopecksPerUnit int64) error {
	if kopecksPerUnit <= 0 {
		return fmt.Errorf("курс валюты %s должен быть положительным", currency)
	}
	c.mu.Lock()
	c.rates[currency] = kopecksPerUnit
	c.mu.Unlock()
	return nil
}

// Rate возвращает текущий курс.
func (c *Converter) Rate(currency domain.Currency) (int64, bool) {
	c.mu.RLock()
	rate, ok := c.rates[currency]
	c.mu.RUnlock()
	return rate, ok
}

// Rates отдаёт копию всех курсов для отображения в интерфейсе.
func (c *Converter) Rates() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[string]int64, len(c.rates))
	for currency, rate := range c.rates {
		out[string(currency)] = rate
	}
	return out
}

// ToRubMinor пересчитывает сумму в копейки.
//
// Округление выполняется к ближайшему целому, а не отбрасыванием: при
// отбрасывании цена в каталоге систематически оказывалась бы ниже
// фактической, что при сравнении предложений выглядит как обман.
func (c *Converter) ToRubMinor(amountMinor int64, currency domain.Currency) (int64, error) {
	if amountMinor < 0 {
		return 0, fmt.Errorf("сумма не может быть отрицательной")
	}
	if currency == domain.CurrencyRUB {
		return amountMinor, nil
	}

	rate, ok := c.Rate(currency)
	if !ok {
		return 0, fmt.Errorf("неизвестный курс для валюты %s", currency)
	}

	minorUnits := currency.Minor()
	// amountMinor * rate — копейки, умноженные на число минорных единиц
	// исходной валюты, поэтому результат делится на minorUnits.
	numerator := amountMinor * rate
	result := numerator / minorUnits
	if remainder := numerator % minorUnits; remainder*2 >= minorUnits {
		result++
	}
	return result, nil
}

// FormatRub форматирует копейки как рублёвую сумму: 1 234 567 ₽.
//
// Дробная часть не показывается: цена автомобиля в копейках — это шум,
// а разряды разделяются узким пробелом, как принято в русской типографике.
func FormatRub(minor int64) string {
	rubles := minor / 100
	return groupDigits(rubles) + " ₽"
}

// Format форматирует сумму в указанной валюте.
func Format(minor int64, currency domain.Currency) string {
	units := minor / currency.Minor()
	symbol := map[domain.Currency]string{
		domain.CurrencyRUB: "₽",
		domain.CurrencyUSD: "$",
		domain.CurrencyCNY: "¥",
		domain.CurrencyJPY: "¥",
	}[currency]

	if symbol == "" {
		symbol = strings.ToUpper(string(currency))
	}
	return groupDigits(units) + " " + symbol
}

// groupDigits разделяет разряды узким неразрывным пробелом.
func groupDigits(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}

	digits := fmt.Sprintf("%d", value)
	if len(digits) <= 3 {
		if negative {
			return "−" + digits
		}
		return digits
	}

	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)

	result := strings.Join(parts, "\u202f")
	if negative {
		return "−" + result
	}
	return result
}
