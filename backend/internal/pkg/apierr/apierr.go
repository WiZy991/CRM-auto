// Package apierr описывает единый формат ошибок API.
//
// Формат ответа фиксирован для всех эндпоинтов:
//
//	{"error": {"code": "validation_failed", "message": "...", "details": {...}}}
//
// Клиент реагирует на машинный code, человек читает message. Внутренние
// подробности (текст ошибки драйвера БД, стек) наружу не выходят никогда —
// они уходят только в лог. Это одновременно и удобство, и защита: сообщения
// вида "duplicate key value violates unique constraint users_email_unique"
// раскрывают структуру базы и факт существования учётной записи.
package apierr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Code — машинно-читаемый код ошибки.
type Code string

const (
	CodeValidation       Code = "validation_failed"
	CodeBadRequest       Code = "bad_request"
	CodeUnauthorized     Code = "unauthorized"
	CodeForbidden        Code = "forbidden"
	CodeNotFound         Code = "not_found"
	CodeConflict         Code = "conflict"
	CodeTooManyRequests  Code = "too_many_requests"
	CodePayloadTooLarge  Code = "payload_too_large"
	CodeUnsupportedMedia Code = "unsupported_media_type"
	CodeInternal         Code = "internal_error"
	CodeUnavailable      Code = "service_unavailable"
	CodeCanceled         Code = "request_canceled"

	// Специализированные коды, на которые интерфейс реагирует по-особому.
	CodeInvalidCredentials Code = "invalid_credentials"
	CodeAccountLocked      Code = "account_locked"
	CodeEmailNotVerified   Code = "email_not_verified"
	CodePhoneNotVerified   Code = "phone_not_verified"
	CodeCaptchaRequired    Code = "captcha_required"
	CodeCaptchaInvalid     Code = "captcha_invalid"
	CodeTokenExpired       Code = "token_expired"
	CodeTokenReused        Code = "token_reused"
	CodeStageTransition    Code = "invalid_stage_transition"
)

// Error — ошибка, безопасная для отдачи клиенту.
type Error struct {
	Status  int
	Code    Code
	Message string
	Details map[string]string

	// Internal хранит первопричину для логов. Наружу не сериализуется.
	Internal error
}

func (e *Error) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Internal }

// WithInternal прикрепляет первопричину, не меняя текст для клиента.
func (e *Error) WithInternal(err error) *Error {
	clone := *e
	clone.Internal = err
	return &clone
}

// WithDetails добавляет подробности (например, ошибки по полям формы).
func (e *Error) WithDetails(details map[string]string) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

func newError(status int, code Code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// --- Конструкторы типовых ошибок -------------------------------------------

func Validation(details map[string]string) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    CodeValidation,
		Message: "Проверьте правильность заполнения полей",
		Details: details,
	}
}

func BadRequest(message string) *Error {
	return newError(http.StatusBadRequest, CodeBadRequest, message)
}

func Unauthorized(message string) *Error {
	if message == "" {
		message = "Требуется авторизация"
	}
	return newError(http.StatusUnauthorized, CodeUnauthorized, message)
}

func Forbidden(message string) *Error {
	if message == "" {
		message = "Недостаточно прав для этого действия"
	}
	return newError(http.StatusForbidden, CodeForbidden, message)
}

// NotFound намеренно не различает «объекта нет» и «объект есть, но чужой»:
// иначе перебором идентификаторов можно составить карту чужих данных.
func NotFound(what string) *Error {
	if what == "" {
		what = "Запрошенный объект"
	}
	return newError(http.StatusNotFound, CodeNotFound, what+" не найден")
}

func Conflict(message string) *Error {
	return newError(http.StatusConflict, CodeConflict, message)
}

func TooManyRequests(message string) *Error {
	if message == "" {
		message = "Слишком много запросов, попробуйте позже"
	}
	return newError(http.StatusTooManyRequests, CodeTooManyRequests, message)
}

func PayloadTooLarge(message string) *Error {
	if message == "" {
		message = "Размер запроса превышает допустимый"
	}
	return newError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, message)
}

func UnsupportedMedia(message string) *Error {
	return newError(http.StatusUnsupportedMediaType, CodeUnsupportedMedia, message)
}

func Internal(err error) *Error {
	return &Error{
		Status:   http.StatusInternalServerError,
		Code:     CodeInternal,
		Message:  "Внутренняя ошибка сервиса. Попробуйте повторить позже",
		Internal: err,
	}
}

func Unavailable(message string) *Error {
	if message == "" {
		message = "Сервис временно недоступен"
	}
	return newError(http.StatusServiceUnavailable, CodeUnavailable, message)
}

// StatusClientClosed — клиент оборвал запрос (навигация, смена вкладки).
// Это не сбой сервиса: в логах и в консоли браузера не должно быть 500.
const StatusClientClosed = 499

func Canceled() *Error {
	return newError(StatusClientClosed, CodeCanceled, "Запрос отменён")
}

func IsCanceled(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

// InvalidCredentials — единый ответ и на неверный пароль, и на несуществующий
// аккаунт. Разные сообщения позволили бы перебором выяснить, какие адреса
// зарегистрированы на платформе.
func InvalidCredentials() *Error {
	return newError(http.StatusUnauthorized, CodeInvalidCredentials, "Неверный email или пароль")
}

func AccountLocked(message string) *Error {
	if message == "" {
		message = "Вход временно заблокирован из-за неудачных попыток"
	}
	return newError(http.StatusLocked, CodeAccountLocked, message)
}

func CaptchaRequired() *Error {
	return newError(http.StatusForbidden, CodeCaptchaRequired, "Подтвердите, что вы не робот")
}

func CaptchaInvalid() *Error {
	return newError(http.StatusForbidden, CodeCaptchaInvalid, "Проверка не пройдена, попробуйте снова")
}

func TokenExpired() *Error {
	return newError(http.StatusUnauthorized, CodeTokenExpired, "Срок действия сессии истёк")
}

// TokenReused — использование уже отозванного refresh-токена. Признак кражи
// токена, поэтому вся семья токенов гасится, а событие уходит в журнал.
func TokenReused() *Error {
	return newError(http.StatusUnauthorized, CodeTokenReused,
		"Сессия завершена по соображениям безопасности, войдите заново")
}

func InvalidStageTransition(message string) *Error {
	return newError(http.StatusConflict, CodeStageTransition, message)
}

// From приводит произвольную ошибку к *Error.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if IsCanceled(err) {
		return Canceled()
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		if IsCanceled(apiErr.Internal) {
			return Canceled()
		}
		return apiErr
	}
	return Internal(err)
}
