package money

import (
	"testing"

	"github.com/autoimport/crm/internal/domain"
)

func TestToRubMinor(t *testing.T) {
	converter := NewConverter()

	// Рубли не конвертируются.
	if got, err := converter.ToRubMinor(150_000_00, domain.CurrencyRUB); err != nil || got != 150_000_00 {
		t.Errorf("рубли не должны пересчитываться: получено %d, %v", got, err)
	}

	// 100 000 юаней при курсе 11.40 = 1 140 000 рублей.
	if err := converter.SetRate(domain.CurrencyCNY, 1_140); err != nil {
		t.Fatalf("не удалось задать курс: %v", err)
	}
	got, err := converter.ToRubMinor(100_000_00, domain.CurrencyCNY)
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	if want := int64(1_140_000_00); got != want {
		t.Errorf("100 000 CNY = %d копеек, ожидалось %d", got, want)
	}
}

func TestToRubMinorHandlesYenWithoutMinorUnits(t *testing.T) {
	converter := NewConverter()
	if err := converter.SetRate(domain.CurrencyJPY, 55); err != nil {
		t.Fatalf("не удалось задать курс: %v", err)
	}

	// У иены нет минорных единиц: 1 000 000 иен — это ровно миллион иен.
	// При курсе 0.55 рубля это 550 000 рублей = 55 000 000 копеек.
	got, err := converter.ToRubMinor(1_000_000, domain.CurrencyJPY)
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	if want := int64(55_000_000); got != want {
		t.Errorf("1 000 000 JPY = %d копеек, ожидалось %d", got, want)
	}
}

func TestToRubMinorRoundsToNearest(t *testing.T) {
	converter := NewConverter()
	if err := converter.SetRate(domain.CurrencyUSD, 8_155); err != nil {
		t.Fatalf("не удалось задать курс: %v", err)
	}

	// 1 цент при курсе 81.55 рубля = 81.55 копейки: дробная часть больше
	// половины, поэтому округление вверх даёт 82.
	got, err := converter.ToRubMinor(1, domain.CurrencyUSD)
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	if got != 82 {
		t.Errorf("округление к ближайшему дало %d, ожидалось 82 копейки", got)
	}

	// 3 цента = 244.65 копейки, округление вверх даёт 245.
	got, err = converter.ToRubMinor(3, domain.CurrencyUSD)
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	if got != 245 {
		t.Errorf("округление дало %d, ожидалось 245 копеек", got)
	}

	// 2 цента = 163.1 копейки, дробная часть меньше половины — вниз.
	got, err = converter.ToRubMinor(2, domain.CurrencyUSD)
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	if got != 163 {
		t.Errorf("округление дало %d, ожидалось 163 копейки", got)
	}
}

func TestToRubMinorRejectsUnknownCurrency(t *testing.T) {
	converter := NewConverter()
	if _, err := converter.ToRubMinor(100, domain.Currency("eur")); err == nil {
		t.Error("для неизвестной валюты ожидалась ошибка, а не молчаливый ноль")
	}
}

func TestSetRateRejectsNonPositive(t *testing.T) {
	converter := NewConverter()
	if err := converter.SetRate(domain.CurrencyUSD, 0); err == nil {
		t.Error("нулевой курс должен отклоняться")
	}
	if err := converter.SetRate(domain.CurrencyUSD, -100); err == nil {
		t.Error("отрицательный курс должен отклоняться")
	}
}

func TestFormatRub(t *testing.T) {
	const nbsp = "\u202f"

	tests := []struct {
		minor int64
		want  string
	}{
		{0, "0 ₽"},
		{99, "0 ₽"},
		{100_00, "100 ₽"},
		{1_234_567_00, "1" + nbsp + "234" + nbsp + "567 ₽"},
		{2_850_000_00, "2" + nbsp + "850" + nbsp + "000 ₽"},
	}

	for _, tc := range tests {
		if got := FormatRub(tc.minor); got != tc.want {
			t.Errorf("FormatRub(%d) = %q, ожидалось %q", tc.minor, got, tc.want)
		}
	}
}

func TestFormatUsesCurrencyMinorUnits(t *testing.T) {
	const nbsp = "\u202f"

	// Иена выводится без деления на 100.
	if got, want := Format(3_500_000, domain.CurrencyJPY), "3"+nbsp+"500"+nbsp+"000 ¥"; got != want {
		t.Errorf("Format для иены = %q, ожидалось %q", got, want)
	}
	if got, want := Format(120_000_00, domain.CurrencyCNY), "120"+nbsp+"000 ¥"; got != want {
		t.Errorf("Format для юаня = %q, ожидалось %q", got, want)
	}
}
