package service

import (
	"strings"
	"testing"
)

func TestNormalizeBrandsRemovesCaseInsensitiveDuplicates(t *testing.T) {
	// Без свёртки по регистру «Toyota» и «toyota» остаются двумя марками,
	// и фильтр по марке показывает одного продавца дважды.
	input := []string{"Toyota", "toyota", "  TOYOTA  ", "Lexus", "", "   ", "Zeekr"}

	brands, err := normalizeBrands(input)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	want := []string{"Toyota", "Lexus", "Zeekr"}
	if len(brands) != len(want) {
		t.Fatalf("получено %d марок (%v), ожидалось %d", len(brands), brands, len(want))
	}
	for index := range want {
		if brands[index] != want[index] {
			t.Errorf("марка %d = %q, ожидалось %q", index, brands[index], want[index])
		}
	}
}

func TestNormalizeBrandsRejectsTooMany(t *testing.T) {
	input := make([]string, maxSellerBrands+1)
	for index := range input {
		input[index] = "Brand" + string(rune('A'+index%26)) + string(rune('0'+index/26))
	}

	if _, err := normalizeBrands(input); err == nil {
		t.Error("список сверх предела должен быть отклонён")
	}
}

func TestNormalizeContactsDropsEmptyAndTrims(t *testing.T) {
	contacts, err := normalizeContacts(map[string]string{
		"  WeChat ": "  crm_import  ",
		"phone":     "+81 90 1234 5678",
		"empty":     "   ",
		"":          "значение без ключа",
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if len(contacts) != 2 {
		t.Fatalf("получено %d контактов (%v), ожидалось 2", len(contacts), contacts)
	}
	if contacts["WeChat"] != "crm_import" {
		t.Errorf("ключ и значение должны быть обрезаны, получено %q", contacts["WeChat"])
	}
}

func TestNormalizeContactsLimitsLength(t *testing.T) {
	long := strings.Repeat("я", maxContactValueLen+50)

	contacts, err := normalizeContacts(map[string]string{"note": long})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	// Обрезка идёт по символам, а не по байтам: кириллица занимает два
	// байта, и обрезка по длине в байтах порвала бы символ пополам.
	if got := len([]rune(contacts["note"])); got != maxContactValueLen {
		t.Errorf("длина значения %d символов, ожидалось %d", got, maxContactValueLen)
	}
}

func TestNormalizeContactsRejectsTooMany(t *testing.T) {
	contacts := make(map[string]string, maxSellerContacts+1)
	for index := 0; index <= maxSellerContacts; index++ {
		contacts["key"+string(rune('a'+index))] = "value"
	}

	if _, err := normalizeContacts(contacts); err == nil {
		t.Error("набор контактов сверх предела должен быть отклонён")
	}
}

func TestValidateHTTPURL(t *testing.T) {
	valid := []string{
		"https://example.com",
		"http://example.co.jp/path?a=1",
	}
	for _, raw := range valid {
		if _, err := validateHTTPURL(raw); err != nil {
			t.Errorf("validateHTTPURL(%q) вернула ошибку: %v", raw, err)
		}
	}

	invalid := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"ftp://example.com",
		"example.com",
		"https://",
		"",
	}
	for _, raw := range invalid {
		if _, err := validateHTTPURL(raw); err == nil {
			t.Errorf("validateHTTPURL(%q) приняла недопустимый адрес", raw)
		}
	}
}
