// Package httpx содержит транспортный слой: маршрутизацию, middleware,
// обработчики запросов и сериализацию ответов.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

// errorEnvelope — внешний формат ошибки.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    apierr.Code       `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// JSON записывает успешный ответ.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if payload == nil || status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Заголовки уже отправлены, ответ не исправить — остаётся только
		// зафиксировать факт обрыва.
		slog.Default().Error("не удалось записать тело ответа", slog.String("error", err.Error()))
	}
}

// NoContent отвечает 204.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error отправляет ошибку клиенту и подробности — в лог.
//
// Разделение обязательно: клиент получает безопасное сообщение, инженер
// видит первопричину. Сообщения драйвера базы наружу не попадают никогда.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	apiError := apierr.From(err)
	if apiError == nil {
		apiError = apierr.Internal(errors.New("неизвестная ошибка"))
	}

	log := LoggerFrom(r.Context())
	attrs := []any{
		slog.String("code", string(apiError.Code)),
		slog.Int("status", apiError.Status),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	}
	if apiError.Internal != nil {
		attrs = append(attrs, slog.String("internal", apiError.Internal.Error()))
	}

	switch {
	case apiError.Status == apierr.StatusClientClosed:
		// Клиент ушёл сам: React Query отменяет запрос при смене экрана.
		// Это не авария сервиса.
	case apiError.Status >= 500:
		log.Error("ошибка обработки запроса", attrs...)
	case apiError.Status == http.StatusTooManyRequests || apiError.Status == http.StatusUnauthorized:
		log.Warn("запрос отклонён", attrs...)
	default:
		log.Info("запрос отклонён", attrs...)
	}

	JSON(w, apiError.Status, errorEnvelope{Error: errorBody{
		Code:    apiError.Code,
		Message: apiError.Message,
		Details: apiError.Details,
	}})
}

// DecodeJSON читает и разбирает тело запроса.
//
// Здесь собраны три отдельные защиты:
//
//  1. проверка Content-Type — иначе форма с другого сайта могла бы отправить
//     запрос как simple request, минуя предполётный CORS-запрос;
//  2. ограничение размера тела вызывающей стороной (MaxBytesReader ставится
//     в middleware) — защита от «бесконечного» POST;
//  3. DisallowUnknownFields — опечатка в имени поля станет явной ошибкой,
//     а не молча проигнорированным значением. Это ловит и попытки
//     подсунуть поля, которых нет в контракте (role, is_admin).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
		if !strings.EqualFold(mediaType, "application/json") {
			return apierr.UnsupportedMedia("Ожидается Content-Type: application/json")
		}
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return decodeError(err)
	}

	// Второй вызов Decode должен вернуть io.EOF: несколько JSON-объектов
	// в одном теле — признак либо ошибки клиента, либо попытки запутать
	// разбор запроса.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest("Тело запроса должно содержать один JSON-объект")
	}
	return nil
}

func decodeError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxBytesErr *http.MaxBytesError

	switch {
	case errors.As(err, &maxBytesErr):
		return apierr.PayloadTooLarge(
			fmt.Sprintf("Размер запроса превышает %d байт", maxBytesErr.Limit))

	case errors.As(err, &syntaxErr):
		return apierr.BadRequest(
			fmt.Sprintf("Некорректный JSON в позиции %d", syntaxErr.Offset))

	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "тело запроса"
		}
		return apierr.Validation(map[string]string{
			field: fmt.Sprintf("ожидался тип %s", humanType(typeErr.Type.String())),
		})

	case errors.Is(err, io.EOF):
		return apierr.BadRequest("Тело запроса пусто")

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return apierr.Validation(map[string]string{field: "неизвестное поле"})

	default:
		return apierr.BadRequest("Не удалось разобрать тело запроса")
	}
}

func humanType(goType string) string {
	switch {
	case strings.Contains(goType, "int"), strings.Contains(goType, "float"):
		return "число"
	case goType == "string":
		return "строка"
	case goType == "bool":
		return "логическое значение"
	default:
		return goType
	}
}
