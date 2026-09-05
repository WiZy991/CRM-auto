package service

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"+7 (912) 345-67-89", "+79123456789", false},
		{"8 912 345 67 89", "+79123456789", false},
		{"79123456789", "+79123456789", false},
		{"9123456789", "+79123456789", false},
		{"+81 90 1234 5678", "+819012345678", false},   // продавец из Японии
		{"+86 138 0013 8000", "+8613800138000", false}, // продавец из Китая
		{"123", "", true},
		{"", "", true},
		{"телефон", "", true},
	}

	for _, tc := range tests {
		got, err := normalizePhone(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizePhone(%q): ожидалась ошибка, получено %q", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizePhone(%q): неожидаемая ошибка %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizePhone(%q) = %q, ожидалось %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizePhoneIsIdempotent(t *testing.T) {
	// Повторная нормализация не должна менять результат: иначе один и тот же
	// номер попал бы в базу в двух формах и обошёл уникальный индекс.
	once, err := normalizePhone("8 (912) 345-67-89")
	if err != nil {
		t.Fatalf("неожидаемая ошибка: %v", err)
	}
	twice, err := normalizePhone(once)
	if err != nil {
		t.Fatalf("неожидаемая ошибка при повторной нормализации: %v", err)
	}
	if once != twice {
		t.Errorf("нормализация не идемпотентна: %q -> %q", once, twice)
	}
}

func TestDeviceLabel(t *testing.T) {
	tests := []struct {
		userAgent string
		want      string
	}{
		{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36",
			"Windows, Chrome",
		},
		{
			"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1",
			"iOS, Safari",
		},
		{
			"Mozilla/5.0 (Windows NT 10.0) YaBrowser/24.1.0 Safari/537.36",
			"Windows, Яндекс.Браузер",
		},
		{"curl/8.5.0", "Неизвестное устройство"},
		{"", "Неизвестное устройство"},
	}

	for _, tc := range tests {
		if got := deviceLabel(tc.userAgent); got != tc.want {
			t.Errorf("deviceLabel(%q) = %q, ожидалось %q", tc.userAgent, got, tc.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  User@Example.COM "); got != "user@example.com" {
		t.Errorf("normalizeEmail вернул %q", got)
	}
}
