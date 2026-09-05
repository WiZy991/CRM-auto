package store

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation проверяет, что ошибка вызвана нарушением конкретного
// уникального индекса.
//
// Имя индекса указывается явно: обобщённое «что-то уже существует» не даёт
// пользователю понять, какое поле исправить.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == constraint
}

// encodeStringArray готовит список строк для колонки jsonb.
func encodeStringArray(values []string) []byte {
	if len(values) == 0 {
		return []byte("[]")
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}

// decodeStringArray читает список строк из колонки jsonb.
//
// Ошибка разбора не превращается в отказ: повреждённая комплектация — не
// повод не показать карточку автомобиля целиком.
func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

// truncate обрезает строку до предела по количеству символов.
//
// Обрезка идёт по рунам, а не по байтам: обрезка «на середине» многобайтного
// символа даёт битую последовательность, которую Postgres отвергнет
// при вставке в текстовое поле.
func truncate(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}

	count := 0
	for index := range value {
		if count == limit {
			return value[:index]
		}
		count++
	}
	return value
}
