package domain

import (
	"testing"
	"time"
)

func TestValidateStageTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    Stage
		to      Stage
		outcome Outcome
		wantErr bool
	}{
		{"шаг вперёд разрешён", StageLead, StageNeeds, OutcomeOpen, false},
		{"шаг вперёд с оплаты на привоз", StagePayment, StageShipping, OutcomeOpen, false},
		{"пропуск этапа запрещён", StageLead, StageContract, OutcomeOpen, true},
		{"прыжок в конец запрещён", StageLead, StageHandover, OutcomeOpen, true},
		{"возврат на предыдущий этап разрешён", StageContract, StageNeeds, OutcomeOpen, false},
		{"возврат через два этапа разрешён", StagePayment, StageLead, OutcomeOpen, false},
		{"с привоза нельзя вернуться до оплаты", StageShipping, StageNeeds, OutcomeOpen, true},
		{"с растаможки нельзя вернуться к договору", StageCustoms, StageContract, OutcomeOpen, true},
		{"с привоза можно вернуться к оплате", StageShipping, StagePayment, OutcomeOpen, false},
		{"тот же этап запрещён", StageNeeds, StageNeeds, OutcomeOpen, true},
		{"закрытую сделку менять нельзя", StageHandover, StageCustoms, OutcomeWon, true},
		{"проигранную сделку менять нельзя", StageNeeds, StageContract, OutcomeLost, true},
		{"неизвестный этап", StageLead, Stage("unknown"), OutcomeOpen, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStageTransition(tc.from, tc.to, tc.outcome)
			if tc.wantErr && err == nil {
				t.Fatalf("ожидалась ошибка для перехода %s -> %s, но переход разрешён", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("переход %s -> %s должен быть разрешён, получено: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestStageOrderIsConsistentWithMeta(t *testing.T) {
	// Порядок в StageOrder и поле Position описывают одно и то же.
	// Расхождение сломало бы и проверку переходов, и отображение прогресса,
	// поэтому связь проверяется тестом, а не соглашением.
	for i, stage := range StageOrder {
		want := i + 1
		if got := stage.Position(); got != want {
			t.Errorf("этап %s: Position() = %d, ожидалось %d", stage, got, want)
		}
		if stage.Meta().Title == "" {
			t.Errorf("этап %s: не заполнено название", stage)
		}
		if stage.Meta().ClientHint == "" {
			t.Errorf("этап %s: не заполнена подсказка для клиента", stage)
		}
	}

	if len(StageOrder) != 7 {
		t.Fatalf("в воронке должно быть 7 этапов по техническому заданию, найдено %d", len(StageOrder))
	}
}

func TestAllowedNextStages(t *testing.T) {
	allowed := AllowedNextStages(StageLead, OutcomeOpen)
	if len(allowed) != 1 || allowed[0] != StageNeeds {
		t.Fatalf("с первого этапа должен быть доступен только следующий, получено %v", allowed)
	}

	// С привоза: вперёд на растаможку и назад до оплаты.
	allowed = AllowedNextStages(StageShipping, OutcomeOpen)
	want := map[Stage]bool{StagePayment: true, StageCustoms: true}
	if len(allowed) != len(want) {
		t.Fatalf("с этапа привоза ожидалось %d вариантов, получено %v", len(want), allowed)
	}
	for _, stage := range allowed {
		if !want[stage] {
			t.Errorf("неожидаемый доступный переход: %s", stage)
		}
	}

	if got := AllowedNextStages(StageHandover, OutcomeWon); got != nil {
		t.Errorf("у закрытой сделки не должно быть доступных переходов, получено %v", got)
	}
}

func TestCanCloseAsWon(t *testing.T) {
	for _, stage := range StageOrder {
		want := stage == StageHandover
		if got := CanCloseAsWon(stage); got != want {
			t.Errorf("CanCloseAsWon(%s) = %v, ожидалось %v", stage, got, want)
		}
	}
}

func TestDealIsStale(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// Норматив этапа «Лид» — один день.
	deal := &Deal{
		Stage:          StageLead,
		Outcome:        OutcomeOpen,
		StageChangedAt: now.Add(-30 * time.Hour),
	}
	if !deal.IsStale(now) {
		t.Error("сделка, висящая на лиде больше суток, должна считаться зависшей")
	}

	deal.StageChangedAt = now.Add(-2 * time.Hour)
	if deal.IsStale(now) {
		t.Error("сделка в пределах нормативного срока не должна считаться зависшей")
	}

	// Закрытые сделки из отчёта о зависших исключаются.
	deal.Outcome = OutcomeWon
	deal.StageChangedAt = now.Add(-100 * 24 * time.Hour)
	if deal.IsStale(now) {
		t.Error("закрытая сделка не может быть зависшей")
	}
}

func TestValidateStageAdvanceRequirements(t *testing.T) {
	zero := int64(0)
	amount := int64(1_500_000_00)

	t.Run("назад без проверок", func(t *testing.T) {
		deal := &Deal{Stage: StagePayment, AmountRubMinor: &zero}
		if err := ValidateStageAdvanceRequirements(deal, StageContract, StageAdvanceEvidence{}); err != nil {
			t.Fatalf("возврат не должен требовать гейтов: %v", err)
		}
	})

	t.Run("к оплате нужна сумма", func(t *testing.T) {
		deal := &Deal{Stage: StageContract, AmountRubMinor: &zero}
		if err := ValidateStageAdvanceRequirements(deal, StagePayment, StageAdvanceEvidence{}); err == nil {
			t.Fatal("ожидалась ошибка без суммы договора")
		}
		deal.AmountRubMinor = &amount
		if err := ValidateStageAdvanceRequirements(deal, StagePayment, StageAdvanceEvidence{}); err != nil {
			t.Fatalf("сумма есть — переход должен пройти: %v", err)
		}
	})

	t.Run("к привозу нужна оплата или документ", func(t *testing.T) {
		deal := &Deal{Stage: StagePayment, PaidRubMinor: 0}
		if err := ValidateStageAdvanceRequirements(deal, StageShipping, StageAdvanceEvidence{}); err == nil {
			t.Fatal("ожидалась ошибка без оплаты и платёжки")
		}
		if err := ValidateStageAdvanceRequirements(deal, StageShipping, StageAdvanceEvidence{HasPaymentDoc: true}); err != nil {
			t.Fatalf("платёжка должна пропускать: %v", err)
		}
		deal.PaidRubMinor = 10_000_00
		if err := ValidateStageAdvanceRequirements(deal, StageShipping, StageAdvanceEvidence{}); err != nil {
			t.Fatalf("оплата должна пропускать: %v", err)
		}
	})

	t.Run("к выдаче нужен СБКТС или декларация", func(t *testing.T) {
		deal := &Deal{Stage: StageCustoms}
		if err := ValidateStageAdvanceRequirements(deal, StageHandover, StageAdvanceEvidence{}); err == nil {
			t.Fatal("ожидалась ошибка без СБКТС и декларации")
		}
		deal.SBKTSNumber = "СБКТС-1"
		if err := ValidateStageAdvanceRequirements(deal, StageHandover, StageAdvanceEvidence{}); err != nil {
			t.Fatalf("СБКТС должен пропускать: %v", err)
		}
		deal.SBKTSNumber = ""
		if err := ValidateStageAdvanceRequirements(deal, StageHandover, StageAdvanceEvidence{HasCustomsDoc: true}); err != nil {
			t.Fatalf("декларация должна пропускать: %v", err)
		}
	})
}
