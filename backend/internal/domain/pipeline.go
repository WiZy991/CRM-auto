package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Stage — этап воронки продаж из пункта 3.5 технического задания.
type Stage string

const (
	StageLead     Stage = "lead"     // 1. Лид
	StageNeeds    Stage = "needs"    // 2. Выявление потребности
	StageContract Stage = "contract" // 3. Договор
	StagePayment  Stage = "payment"  // 4. Оплата
	StageShipping Stage = "shipping" // 5. Привоз
	StageCustoms  Stage = "customs"  // 6. Растаможка
	StageHandover Stage = "handover" // 7. Выдача автомобиля
)

// StageOrder задаёт порядок этапов. Индекс используется для проверки
// переходов и для отображения прогресса.
var StageOrder = []Stage{
	StageLead, StageNeeds, StageContract,
	StagePayment, StageShipping, StageCustoms, StageHandover,
}

// StageMeta — описание этапа для интерфейса.
type StageMeta struct {
	Stage       Stage  `json:"stage"`
	Position    int    `json:"position"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// ClientHint — что видит клиент на этом этапе. Формулировки разные:
	// дилеру нужна инструкция к действию, клиенту — понимание статуса.
	ClientHint string `json:"client_hint"`
	// NormativeDays — ориентировочный срок этапа. Используется в аналитике
	// для подсветки зависших сделок.
	NormativeDays int `json:"normative_days"`
}

var stageMeta = map[Stage]StageMeta{
	StageLead: {
		Stage: StageLead, Position: 1,
		Title:         "Лид",
		Description:   "Обращение получено, нужен первый контакт с клиентом",
		ClientHint:    "Заявка принята, дилер скоро свяжется с вами",
		NormativeDays: 1,
	},
	StageNeeds: {
		Stage: StageNeeds, Position: 2,
		Title:         "Выявление потребности",
		Description:   "Согласуйте требования к автомобилю, бюджет и сроки",
		ClientHint:    "Обсуждаем параметры автомобиля и условия",
		NormativeDays: 5,
	},
	StageContract: {
		Stage: StageContract, Position: 3,
		Title:         "Договор",
		Description:   "Подготовьте и подпишите договор на подбор и поставку",
		ClientHint:    "Готовится договор на поставку автомобиля",
		NormativeDays: 3,
	},
	StagePayment: {
		Stage: StagePayment, Position: 4,
		Title:         "Оплата",
		Description:   "Получите оплату или задаток, приложите платёжные документы",
		ClientHint:    "Ожидается оплата по договору",
		NormativeDays: 3,
	},
	StageShipping: {
		Stage: StageShipping, Position: 5,
		Title:         "Привоз",
		Description:   "Выкуп у продавца, отправка и доставка до порта",
		ClientHint:    "Автомобиль выкуплен и едет в Россию",
		NormativeDays: 30,
	},
	StageCustoms: {
		Stage: StageCustoms, Position: 6,
		Title:         "Растаможка",
		Description:   "Оформление на таможне, оплата пошлин и сборов",
		ClientHint:    "Автомобиль проходит таможенное оформление",
		NormativeDays: 10,
	},
	StageHandover: {
		Stage: StageHandover, Position: 7,
		Title:         "Выдача автомобиля",
		Description:   "Передача автомобиля и документов клиенту",
		ClientHint:    "Автомобиль готов к выдаче",
		NormativeDays: 5,
	},
}

// StagesCatalog возвращает описание всех этапов по порядку.
func StagesCatalog() []StageMeta {
	out := make([]StageMeta, 0, len(StageOrder))
	for _, stage := range StageOrder {
		out = append(out, stageMeta[stage])
	}
	return out
}

// Meta возвращает описание этапа.
func (s Stage) Meta() StageMeta {
	if meta, ok := stageMeta[s]; ok {
		return meta
	}
	return StageMeta{Stage: s, Title: string(s)}
}

// Position — номер этапа, начиная с единицы. Ноль означает неизвестный этап.
func (s Stage) Position() int { return s.Meta().Position }

func (s Stage) Valid() bool {
	_, ok := stageMeta[s]
	return ok
}

func (s Stage) Title() string { return s.Meta().Title }

// ParseStage разбирает этап из строки.
func ParseStage(raw string) (Stage, error) {
	stage := Stage(strings.ToLower(strings.TrimSpace(raw)))
	if !stage.Valid() {
		return "", fmt.Errorf("неизвестный этап воронки %q", raw)
	}
	return stage, nil
}

// Outcome — исход сделки.
type Outcome string

const (
	OutcomeOpen Outcome = "open"
	OutcomeWon  Outcome = "won"
	OutcomeLost Outcome = "lost"
)

func (o Outcome) Valid() bool {
	return o == OutcomeOpen || o == OutcomeWon || o == OutcomeLost
}

// --- Правила переходов ------------------------------------------------------

// Правила переходов заданы явной таблицей, а не проверкой «номер этапа
// отличается на единицу». Так формулируются реальные ограничения бизнеса:
//
//   - вперёд можно двигаться только на один шаг: нельзя объявить растаможку,
//     не оплатив и не привезя автомобиль;
//   - назад можно вернуться на любой предыдущий этап — это нормальная
//     ситуация, когда клиент передумал по комплектации или сорвалась оплата;
//   - после «Выдачи» сделка закрывается и переходы запрещены.
//
// Возврат назад ограничен рубежом «Оплата»: если деньги получены и машина
// выкуплена у зарубежного продавца, откат сделки к обсуждению потребностей
// означает, что где-то потеряны деньги, и такое действие требует отдельного
// сценария отмены, а не обычной смены этапа.
var stageRollbackFloor = map[Stage]Stage{
	StageShipping: StagePayment,
	StageCustoms:  StagePayment,
	StageHandover: StagePayment,
}

// StageTransitionError описывает недопустимый переход.
type StageTransitionError struct {
	From   Stage
	To     Stage
	Reason string
}

func (e *StageTransitionError) Error() string {
	return fmt.Sprintf("переход %s -> %s недопустим: %s", e.From, e.To, e.Reason)
}

// ValidateStageTransition проверяет допустимость смены этапа.
func ValidateStageTransition(from, to Stage, currentOutcome Outcome) error {
	if !from.Valid() {
		return &StageTransitionError{From: from, To: to, Reason: "исходный этап неизвестен"}
	}
	if !to.Valid() {
		return &StageTransitionError{From: from, To: to, Reason: "целевой этап неизвестен"}
	}
	if currentOutcome != OutcomeOpen {
		return &StageTransitionError{From: from, To: to,
			Reason: "сделка уже закрыта, изменение этапа невозможно"}
	}
	if from == to {
		return &StageTransitionError{From: from, To: to, Reason: "сделка уже находится на этом этапе"}
	}

	fromPos, toPos := from.Position(), to.Position()

	if toPos > fromPos {
		if toPos-fromPos > 1 {
			return &StageTransitionError{From: from, To: to,
				Reason: fmt.Sprintf("этапы нельзя пропускать, следующий этап — «%s»",
					StageOrder[fromPos].Title())}
		}
		return nil
	}

	if floor, ok := stageRollbackFloor[from]; ok && toPos < floor.Position() {
		return &StageTransitionError{From: from, To: to,
			Reason: fmt.Sprintf("с этапа «%s» вернуться можно не ранее чем к «%s»",
				from.Title(), floor.Title())}
	}
	return nil
}

// AllowedNextStages возвращает этапы, доступные для перехода.
// Используется интерфейсом, чтобы не показывать заведомо запрещённые кнопки.
func AllowedNextStages(from Stage, outcome Outcome) []Stage {
	if outcome != OutcomeOpen {
		return nil
	}
	allowed := make([]Stage, 0, len(StageOrder))
	for _, candidate := range StageOrder {
		if ValidateStageTransition(from, candidate, outcome) == nil {
			allowed = append(allowed, candidate)
		}
	}
	return allowed
}

// CanCloseAsWon — сделку можно признать успешной только после выдачи
// автомобиля. Иначе статистика дилеров теряет смысл.
func CanCloseAsWon(stage Stage) bool { return stage == StageHandover }

// Deal — сделка в воронке.
type Deal struct {
	ID           uuid.UUID
	PublicNumber int64

	ClientID uuid.UUID
	DealerID uuid.UUID
	CarID    *uuid.UUID
	SellerID *uuid.UUID

	Stage   Stage
	Outcome Outcome

	Title          string
	AmountMinor    *int64
	Currency       Currency
	AmountRubMinor *int64
	PaidRubMinor   int64

	StageChangedAt     time.Time
	ExpectedHandoverAt *time.Time
	LostReason         string
	ManagerNote        string

	ClosedAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsParticipant проверяет, относится ли пользователь к сделке.
//
// Проверка живёт в домене, но не заменяет условия в SQL: запросы всегда
// фильтруют по client_id или dealer_id, чтобы чужая сделка не попадала
// в выборку даже теоретически.
func (d *Deal) IsParticipant(userID uuid.UUID) bool {
	return d.ClientID == userID || d.DealerID == userID
}

// DaysOnStage — сколько полных дней сделка находится на текущем этапе.
func (d *Deal) DaysOnStage(now time.Time) int {
	return int(now.Sub(d.StageChangedAt).Hours() / 24)
}

// IsStale сообщает, что сделка задержалась на этапе дольше нормативного срока.
//
// Сравниваются именно длительности, а не целые дни: округление вниз давало
// бы сутки бесплатной просрочки на каждом этапе, и сделка, висящая на лиде
// тридцать часов при норме в один день, считалась бы уложившейся в срок.
func (d *Deal) IsStale(now time.Time) bool {
	if d.Outcome != OutcomeOpen {
		return false
	}
	normative := d.Stage.Meta().NormativeDays
	if normative <= 0 {
		return false
	}
	return now.Sub(d.StageChangedAt) > time.Duration(normative)*24*time.Hour
}
