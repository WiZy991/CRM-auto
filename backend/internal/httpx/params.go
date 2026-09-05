package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

// chiURLParam возвращает параметр пути.
func chiURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// UUIDParam разбирает UUID из пути.
func UUIDParam(r *http.Request, key string) (uuid.UUID, error) {
	raw := chi.URLParam(r, key)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apierr.BadRequest("Некорректный идентификатор в адресе запроса")
	}
	return id, nil
}

// Query — типизированное чтение параметров строки запроса.
//
// Все значения проходят через явные ограничения. Это не только удобство:
// параметры сортировки и постраничности попадают в SQL, поэтому список
// допустимых значений задаётся здесь белым списком, а не проверкой на
// «плохие символы».
type Query struct {
	values map[string][]string
	errs   map[string]string
}

// NewQuery создаёт помощник разбора параметров запроса.
func NewQuery(r *http.Request) *Query {
	return &Query{values: r.URL.Query(), errs: map[string]string{}}
}

// Err возвращает накопленные ошибки разбора или nil.
func (q *Query) Err() error {
	if len(q.errs) == 0 {
		return nil
	}
	return apierr.Validation(q.errs)
}

func (q *Query) raw(key string) (string, bool) {
	list, ok := q.values[key]
	if !ok || len(list) == 0 {
		return "", false
	}
	value := strings.TrimSpace(list[0])
	if value == "" {
		return "", false
	}
	return value, true
}

// String читает строку с ограничением длины.
func (q *Query) String(key string, maxLen int) string {
	value, ok := q.raw(key)
	if !ok {
		return ""
	}
	if len([]rune(value)) > maxLen {
		q.errs[key] = "значение слишком длинное"
		return ""
	}
	return value
}

// Int читает целое число в заданных границах.
func (q *Query) Int(key string, min, max int) *int {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		q.errs[key] = "ожидалось целое число"
		return nil
	}
	if parsed < min || parsed > max {
		q.errs[key] = "значение вне допустимого диапазона"
		return nil
	}
	return &parsed
}

// Int64 читает 64-битное целое (используется для сумм в минорных единицах).
func (q *Query) Int64(key string, min, max int64) *int64 {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		q.errs[key] = "ожидалось целое число"
		return nil
	}
	if parsed < min || parsed > max {
		q.errs[key] = "значение вне допустимого диапазона"
		return nil
	}
	return &parsed
}

// Bool читает логическое значение.
func (q *Query) Bool(key string) *bool {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes":
		result := true
		return &result
	case "0", "false", "no":
		result := false
		return &result
	default:
		q.errs[key] = "ожидалось true или false"
		return nil
	}
}

// Enum читает значение из закрытого списка.
func (q *Query) Enum(key string, allowed ...string) string {
	value, ok := q.raw(key)
	if !ok {
		return ""
	}
	value = strings.ToLower(value)
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	q.errs[key] = "недопустимое значение, ожидается одно из: " + strings.Join(allowed, ", ")
	return ""
}

// EnumList читает список значений из закрытого списка.
func (q *Query) EnumList(key string, limit int, allowed ...string) []string {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}

	parts := strings.Split(value, ",")
	if len(parts) > limit {
		q.errs[key] = "слишком много значений"
		return nil
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, item := range allowed {
		allowedSet[item] = true
	}

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToLower(strings.TrimSpace(part))
		if item == "" {
			continue
		}
		if !allowedSet[item] {
			q.errs[key] = "недопустимое значение: " + item
			return nil
		}
		out = append(out, item)
	}
	return out
}

// StringList читает список произвольных строк с ограничениями.
func (q *Query) StringList(key string, limit, maxLen int) []string {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}

	parts := strings.Split(value, ",")
	if len(parts) > limit {
		q.errs[key] = "слишком много значений"
		return nil
	}

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if len([]rune(item)) > maxLen {
			q.errs[key] = "значение слишком длинное"
			return nil
		}
		out = append(out, item)
	}
	return out
}

// Date читает дату в формате ГГГГ-ММ-ДД.
func (q *Query) Date(key string) *time.Time {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		q.errs[key] = "ожидалась дата в формате ГГГГ-ММ-ДД"
		return nil
	}
	return &parsed
}

// UUID читает идентификатор.
func (q *Query) UUID(key string) *uuid.UUID {
	value, ok := q.raw(key)
	if !ok {
		return nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		q.errs[key] = "ожидался идентификатор в формате UUID"
		return nil
	}
	return &parsed
}

// Pagination — параметры постраничной выдачи.
//
// Используется курсор, а не смещение: OFFSET на больших таблицах заставляет
// базу читать и отбрасывать все предыдущие строки, и сотая страница
// каталога становится в сто раз дороже первой.
type Pagination struct {
	Limit  int
	Cursor string
}

// Pagination читает параметры постраничной выдачи с жёстким верхним пределом.
func (q *Query) Pagination(defaultLimit, maxLimit int) Pagination {
	limit := defaultLimit
	if value := q.Int("limit", 1, maxLimit); value != nil {
		limit = *value
	}
	return Pagination{Limit: limit, Cursor: q.String("cursor", 200)}
}
