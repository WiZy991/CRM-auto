package service

import (
	"strings"
	"testing"
	"time"
)

func TestMaskPersonalNameHidesSurname(t *testing.T) {
	// Общий пул заявок видят все проверенные дилеры. Полная фамилия вместе
	// с параметрами запроса — это уже персональные данные человека,
	// который ещё не выбрал, с кем работать.
	cases := []struct {
		full string
		want string
	}{
		{full: "Иван Петров", want: "Иван П."},
		{full: "Иван Петрович Сидоров", want: "Иван П."},
		{full: "Иван", want: "Иван"},
		{full: "  ", want: "Клиент"},
		{full: "", want: "Клиент"},
	}

	for _, testCase := range cases {
		if got := maskPersonalName(testCase.full); got != testCase.want {
			t.Errorf("maskPersonalName(%q) = %q, ожидалось %q", testCase.full, got, testCase.want)
		}
	}
}

func TestNormalizeBudgetConvertsRublesToMinorUnits(t *testing.T) {
	from := int64(1_500_000)
	to := int64(2_000_000)

	gotFrom, gotTo, err := normalizeBudget(&from, &to)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	// Наружу бюджет приходит в рублях, внутри хранится в копейках.
	// Перепутанная единица измерения даёт ошибку в сто раз.
	if *gotFrom != 150_000_000 {
		t.Errorf("нижняя граница = %d копеек, ожидалось 150000000", *gotFrom)
	}
	if *gotTo != 200_000_000 {
		t.Errorf("верхняя граница = %d копеек, ожидалось 200000000", *gotTo)
	}
}

func TestNormalizeBudgetRejectsBadRanges(t *testing.T) {
	from := int64(3_000_000)
	to := int64(1_000_000)
	if _, _, err := normalizeBudget(&from, &to); err == nil {
		t.Error("нижняя граница выше верхней должна быть отклонена")
	}

	negative := int64(-1)
	if _, _, err := normalizeBudget(&negative, nil); err == nil {
		t.Error("отрицательный бюджет должен быть отклонён")
	}

	huge := int64(500_000_000)
	if _, _, err := normalizeBudget(nil, &huge); err == nil {
		t.Error("бюджет вне разумных границ должен быть отклонён")
	}

	gotFrom, gotTo, err := normalizeBudget(nil, nil)
	if err != nil || gotFrom != nil || gotTo != nil {
		t.Error("незаполненный бюджет не должен быть ошибкой")
	}
}

func TestValidateYearRange(t *testing.T) {
	currentYear := time.Now().Year()

	from, to := 2015, 2020
	if err := validateYearRange(&from, &to); err != nil {
		t.Errorf("обычный диапазон должен приниматься: %v", err)
	}

	// Заказ автомобиля следующего модельного года — обычное дело,
	// поэтому верхняя граница на год впереди текущего.
	next := currentYear + 1
	if err := validateYearRange(nil, &next); err != nil {
		t.Errorf("следующий модельный год должен приниматься: %v", err)
	}

	tooFar := currentYear + 2
	if err := validateYearRange(nil, &tooFar); err == nil {
		t.Error("год далеко в будущем должен быть отклонён")
	}

	tooOld := 1950
	if err := validateYearRange(&tooOld, nil); err == nil {
		t.Error("слишком старый год должен быть отклонён")
	}

	swappedFrom, swappedTo := 2020, 2015
	if err := validateYearRange(&swappedFrom, &swappedTo); err == nil {
		t.Error("перепутанные границы должны быть отклонены")
	}
}

func TestNormalizeShortTextCutsByRunes(t *testing.T) {
	value := strings.Repeat("щ", 100)

	got := normalizeShortText("  "+value+"  ", 30)

	if count := len([]rune(got)); count != 30 {
		t.Errorf("обрезано до %d символов, ожидалось 30", count)
	}
	// Обрезка по байтам на кириллице оставила бы половину символа и
	// последовательность, которую Postgres отверг бы при вставке.
	if !isValidUTF8(got) {
		t.Error("после обрезки получилась некорректная строка UTF-8")
	}
}

func isValidUTF8(value string) bool {
	for _, symbol := range value {
		if symbol == '\uFFFD' {
			return false
		}
	}
	return true
}
