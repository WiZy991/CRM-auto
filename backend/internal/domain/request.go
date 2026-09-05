package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RequestStatus — состояние заявки клиента.
type RequestStatus string

const (
	RequestNew        RequestStatus = "new"
	RequestInProgress RequestStatus = "in_progress"
	RequestAnswered   RequestStatus = "answered"
	RequestConverted  RequestStatus = "converted"
	RequestRejected   RequestStatus = "rejected"
	RequestClosed     RequestStatus = "closed"
)

func (s RequestStatus) Valid() bool {
	switch s {
	case RequestNew, RequestInProgress, RequestAnswered,
		RequestConverted, RequestRejected, RequestClosed:
		return true
	default:
		return false
	}
}

func (s RequestStatus) Title() string {
	switch s {
	case RequestNew:
		return "Новая"
	case RequestInProgress:
		return "В работе"
	case RequestAnswered:
		return "Есть ответ"
	case RequestConverted:
		return "Стала сделкой"
	case RequestRejected:
		return "Отклонена"
	case RequestClosed:
		return "Закрыта"
	default:
		return string(s)
	}
}

// IsFinal сообщает, что заявка больше не в работе.
func (s RequestStatus) IsFinal() bool {
	return s == RequestConverted || s == RequestRejected || s == RequestClosed
}

// ContactPreference — предпочтительный способ связи.
type ContactPreference string

const (
	ContactPhone     ContactPreference = "phone"
	ContactEmail     ContactPreference = "email"
	ContactMessenger ContactPreference = "messenger"
)

func (c ContactPreference) Valid() bool {
	return c == ContactPhone || c == ContactEmail || c == ContactMessenger
}

func (c ContactPreference) Title() string {
	switch c {
	case ContactPhone:
		return "Звонок"
	case ContactEmail:
		return "Электронная почта"
	case ContactMessenger:
		return "Мессенджер"
	default:
		return string(c)
	}
}

// ParseContactPreference разбирает способ связи, подставляя телефон по
// умолчанию: пустое поле не должно приводить к ошибке валидации формы.
func ParseContactPreference(raw string) (ContactPreference, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ContactPhone, nil
	}
	preference := ContactPreference(value)
	if !preference.Valid() {
		return "", fmt.Errorf("недопустимый способ связи %q", raw)
	}
	return preference, nil
}

// Request — заявка клиента на подбор или покупку автомобиля.
type Request struct {
	ID           uuid.UUID
	PublicNumber int64

	ClientID uuid.UUID
	DealerID *uuid.UUID
	CarID    *uuid.UUID

	Status RequestStatus

	DesiredBrand string
	DesiredModel string
	YearFrom     *int
	YearTo       *int
	Origin       *Origin

	BudgetFromRubMinor *int64
	BudgetToRubMinor   *int64

	Body    *BodyType
	Gearbox *Transmission

	Comment           string
	ContactPreference ContactPreference

	DealerReply    string
	RepliedAt      *time.Time
	RejectedReason string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// InOpenPool сообщает, что заявка ещё не закреплена за дилером.
func (r *Request) InOpenPool() bool {
	return r.DealerID == nil && r.Status == RequestNew
}

// Involves проверяет причастность пользователя к заявке.
func (r *Request) Involves(userID uuid.UUID) bool {
	if r.ClientID == userID {
		return true
	}
	return r.DealerID != nil && *r.DealerID == userID
}

// CanConvertToDeal сообщает, можно ли создать сделку по заявке.
func (r *Request) CanConvertToDeal() error {
	if r.DealerID == nil {
		return fmt.Errorf("заявка не закреплена за дилером")
	}
	if r.Status == RequestConverted {
		return fmt.Errorf("по этой заявке сделка уже создана")
	}
	if r.Status == RequestRejected || r.Status == RequestClosed {
		return fmt.Errorf("заявка закрыта, создать сделку нельзя")
	}
	return nil
}

// Summary — краткое описание запроса клиента для списка у дилера.
//
// Собирается на сервере: правила «показать бренд, если указан, иначе страну,
// иначе просто бюджет» одинаковы для всех интерфейсов, и дублировать их в
// вебе и мобильном клиенте незачем.
func (r *Request) Summary() string {
	parts := make([]string, 0, 4)

	switch {
	case r.DesiredBrand != "" && r.DesiredModel != "":
		parts = append(parts, r.DesiredBrand+" "+r.DesiredModel)
	case r.DesiredBrand != "":
		parts = append(parts, r.DesiredBrand)
	case r.Origin != nil:
		parts = append(parts, "Автомобиль из "+r.Origin.Title())
	default:
		parts = append(parts, "Подбор автомобиля")
	}

	if r.YearFrom != nil && r.YearTo != nil {
		parts = append(parts, fmt.Sprintf("%d–%d", *r.YearFrom, *r.YearTo))
	} else if r.YearFrom != nil {
		parts = append(parts, fmt.Sprintf("от %d", *r.YearFrom))
	}

	if r.Body != nil {
		parts = append(parts, strings.ToLower(r.Body.Title()))
	}
	return strings.Join(parts, ", ")
}
