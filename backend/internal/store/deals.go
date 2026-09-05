package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

// Deals — доступ к сделкам и воронке.
type Deals struct {
	pool *Pool
}

func NewDeals(pool *Pool) *Deals { return &Deals{pool: pool} }

const dealColumns = `
	deals.id, deals.public_number,
	deals.client_id, deals.dealer_id, deals.car_id, deals.seller_id,
	deals.stage, deals.outcome,
	deals.title, deals.amount_minor, deals.currency, deals.amount_rub_minor, deals.paid_rub_minor,
	deals.services_note, deals.destination_port, deals.shipping_tracking,
	deals.customs_duties_rub_minor, deals.sbkts_number, deals.sbkts_issued_at,
	deals.first_contacted_at,
	deals.contract_signed_at, deals.paid_at, deals.shipped_at, deals.arrived_at,
	deals.customs_cleared_at, deals.handed_over_at,
	deals.stage_changed_at, deals.expected_handover_at,
	COALESCE(deals.lost_reason, ''), deals.manager_note,
	deals.closed_at, deals.created_at, deals.updated_at`

func scanDeal(row pgx.Row) (*domain.Deal, error) {
	var deal domain.Deal
	err := row.Scan(
		&deal.ID, &deal.PublicNumber,
		&deal.ClientID, &deal.DealerID, &deal.CarID, &deal.SellerID,
		&deal.Stage, &deal.Outcome,
		&deal.Title, &deal.AmountMinor, &deal.Currency, &deal.AmountRubMinor, &deal.PaidRubMinor,
		&deal.ServicesNote, &deal.DestinationPort, &deal.ShippingTracking,
		&deal.CustomsDutiesRubMinor, &deal.SBKTSNumber, &deal.SBKTSIssuedAt,
		&deal.FirstContactedAt,
		&deal.ContractSignedAt, &deal.PaidAt, &deal.ShippedAt, &deal.ArrivedAt,
		&deal.CustomsClearedAt, &deal.HandedOverAt,
		&deal.StageChangedAt, &deal.ExpectedHandoverAt,
		&deal.LostReason, &deal.ManagerNote,
		&deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение сделки: %w", err)
	}
	return &deal, nil
}

// CreateDealParams — параметры создания сделки.
type CreateDealParams struct {
	RequestID *uuid.UUID
	ClientID  uuid.UUID
	DealerID  uuid.UUID
	CarID     *uuid.UUID
	SellerID  *uuid.UUID

	Title          string
	AmountMinor    *int64
	Currency       domain.Currency
	AmountRubMinor *int64
	Stage          domain.Stage
}

// Create создаёт сделку и первую запись в истории этапов.
//
// Запись истории создаётся сразу: иначе для только что созданной сделки
// нельзя посчитать, сколько она провела на этапе «Лид», и аналитика первой
// стадии оказывается пустой.
func (d *Deals) Create(ctx context.Context, params CreateDealParams) (*domain.Deal, error) {
	var result *domain.Deal

	err := d.pool.InTx(ctx, func(tx pgx.Tx) error {
		stage := params.Stage
		if stage == "" {
			stage = domain.StageLead
		}

		row := tx.QueryRow(ctx, `
			INSERT INTO deals (request_id, client_id, dealer_id, car_id, seller_id,
			                   title, amount_minor, currency, amount_rub_minor, stage)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING `+dealColumns,
			params.RequestID, params.ClientID, params.DealerID, params.CarID, params.SellerID,
			params.Title, params.AmountMinor, params.Currency, params.AmountRubMinor, stage,
		)

		deal, err := scanDeal(row)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stage_history (deal_id, from_stage, to_stage, changed_by, comment)
			VALUES ($1, NULL, $3, $2, 'Сделка создана')`,
			deal.ID, params.DealerID, stage); err != nil {
			return fmt.Errorf("запись начальной истории этапов: %w", err)
		}

		// Заявка помечается сконвертированной в той же транзакции: сделка
		// без отметки в заявке означала бы, что дилер видит её в общем пуле
		// как необработанную.
		if params.RequestID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE requests SET status = 'converted', dealer_id = COALESCE(dealer_id, $2)
				WHERE id = $1`, *params.RequestID, params.DealerID); err != nil {
				return fmt.Errorf("отметка заявки как сконвертированной: %w", err)
			}
		}

		result = deal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DealFilter — параметры выборки сделок.
type DealFilter struct {
	// Роль запрашивающего определяет, по какому полю фильтровать.
	// Ни один запрос не выполняется без одного из этих ограничений, кроме
	// админского.
	ClientID *uuid.UUID
	DealerID *uuid.UUID
	// AdminAll снимает ограничение по участнику: только для администратора.
	AdminAll bool

	Stages   []string
	Outcomes []string
	Search   string

	StaleOnly bool

	Limit  int
	Offset int
}

// ListWithParticipants возвращает сделки вместе с именами участников.
type DealListItem struct {
	Deal domain.Deal

	ClientName string
	DealerName string
	CarTitle   string

	OpenTasks      int
	UnreadMessages int
}

// List возвращает сделки по фильтру.
func (d *Deals) List(ctx context.Context, filter DealFilter, viewerID uuid.UUID) ([]DealListItem, int, error) {
	builder := &argBuilder{}
	where := make([]string, 0, 8)

	switch {
	case filter.DealerID != nil:
		where = append(where, "deals.dealer_id = "+builder.add(*filter.DealerID))
	case filter.ClientID != nil:
		where = append(where, "deals.client_id = "+builder.add(*filter.ClientID))
	case filter.AdminAll:
		// Администратор видит все сделки площадки.
	default:
		// Пустой фильтр допустим только для администратора, и вызывающий
		// код обязан это проверить. Здесь стоит защита от ошибки: без
		// ограничения по участнику выборка не выполняется.
		return nil, 0, errors.New("выборка сделок без ограничения по участнику запрещена")
	}

	if len(filter.Stages) > 0 {
		where = append(where, fmt.Sprintf("deals.stage = ANY(%s::deal_stage[])", builder.add(filter.Stages)))
	}
	if len(filter.Outcomes) > 0 {
		where = append(where, fmt.Sprintf("deals.outcome = ANY(%s::deal_outcome[])", builder.add(filter.Outcomes)))
	}
	if filter.Search != "" {
		where = append(where, fmt.Sprintf(
			"(deals.title ILIKE %s OR deals.public_number::text = %s)",
			builder.add("%"+filter.Search+"%"), builder.add(filter.Search)))
	}
	if filter.StaleOnly {
		// Зависшие сделки: срок на этапе превышен. Нормативы заданы в
		// домене, поэтому сюда они приходят как выражение CASE, собранное
		// из того же источника.
		where = append(where, "deals.outcome = 'open' AND deals.stage_changed_at < now() - "+
			staleIntervalExpression())
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	whereSQL := "true"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	var total int
	if err := d.pool.QueryRow(ctx,
		`SELECT count(*) FROM deals WHERE `+whereSQL, builder.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт сделок: %w", err)
	}

	limitPlaceholder := builder.add(limit)
	offsetPlaceholder := builder.add(filter.Offset)
	viewerPlaceholder := builder.add(viewerID)

	query := fmt.Sprintf(`
		SELECT %s,
		       client.full_name, dealer.full_name,
		       COALESCE(cars.title, ''),
		       COALESCE(tasks.open_count, 0),
		       COALESCE(unread.count, 0)
		FROM deals
		JOIN users AS client ON client.id = deals.client_id
		JOIN users AS dealer ON dealer.id = deals.dealer_id
		LEFT JOIN cars ON cars.id = deals.car_id
		LEFT JOIN LATERAL (
			SELECT count(*) AS open_count FROM deal_tasks
			WHERE deal_tasks.deal_id = deals.id AND deal_tasks.done_at IS NULL
		) AS tasks ON true
		LEFT JOIN LATERAL (
			SELECT count(*) AS count FROM deal_messages
			WHERE deal_messages.deal_id = deals.id
			  AND deal_messages.author_id <> %s
			  AND deal_messages.read_at IS NULL
		) AS unread ON true
		WHERE %s
		ORDER BY deals.stage_changed_at DESC, deals.id DESC
		LIMIT %s OFFSET %s`,
		dealColumns, viewerPlaceholder, whereSQL, limitPlaceholder, offsetPlaceholder)

	rows, err := d.pool.Query(ctx, query, builder.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("выборка сделок: %w", err)
	}
	defer rows.Close()

	var items []DealListItem
	for rows.Next() {
		var item DealListItem
		deal := &item.Deal

		if err := rows.Scan(
			&deal.ID, &deal.PublicNumber,
			&deal.ClientID, &deal.DealerID, &deal.CarID, &deal.SellerID,
			&deal.Stage, &deal.Outcome,
			&deal.Title, &deal.AmountMinor, &deal.Currency, &deal.AmountRubMinor, &deal.PaidRubMinor,
			&deal.StageChangedAt, &deal.ExpectedHandoverAt,
			&deal.LostReason, &deal.ManagerNote,
			&deal.ClosedAt, &deal.CreatedAt, &deal.UpdatedAt,
			&item.ClientName, &item.DealerName, &item.CarTitle,
			&item.OpenTasks, &item.UnreadMessages,
		); err != nil {
			return nil, 0, fmt.Errorf("разбор строки сделки: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// staleIntervalExpression собирает выражение нормативного срока по этапам.
//
// Нормативы описаны в домене, а SQL получает их как явный CASE. Так значение
// не дублируется руками в двух местах: изменение норматива в одном файле
// меняет и отчёт.
func staleIntervalExpression() string {
	var b strings.Builder
	b.WriteString("(CASE deals.stage")
	for _, meta := range domain.StagesCatalog() {
		fmt.Fprintf(&b, " WHEN '%s' THEN interval '%d days'", meta.Stage, meta.NormativeDays)
	}
	b.WriteString(" ELSE interval '30 days' END)")
	return b.String()
}

// ByIDForParticipant возвращает сделку, если запрашивающий — её участник.
//
// Проверка участия выполнена условием в SQL, а не сравнением после выборки.
// Это защита от обращения к чужим объектам по идентификатору: строка просто
// не попадает в результат, и ошибиться в порядке проверок невозможно.
func (d *Deals) ByIDForParticipant(ctx context.Context, dealID, userID uuid.UUID, isAdmin bool) (*domain.Deal, error) {
	if isAdmin {
		return scanDeal(d.pool.QueryRow(ctx,
			`SELECT `+dealColumns+` FROM deals WHERE deals.id = $1`, dealID))
	}

	return scanDeal(d.pool.QueryRow(ctx,
		`SELECT `+dealColumns+` FROM deals
		 WHERE deals.id = $1 AND (deals.client_id = $2 OR deals.dealer_id = $2)`,
		dealID, userID))
}

// ChangeStageParams — параметры смены этапа.
type ChangeStageParams struct {
	DealID    uuid.UUID
	DealerID  uuid.UUID
	IsAdmin   bool
	ToStage   domain.Stage
	Comment   string
	ChangedBy uuid.UUID
}

// ChangeStage переводит сделку на другой этап.
//
// Транзакция со блокировкой строки решает вполне реальную проблему: два
// менеджера дилера, открывшие канбан одновременно, могут перетащить одну
// карточку в разные колонки. Без блокировки в истории окажутся два перехода
// из одного состояния, а нормативные сроки будут посчитаны неверно.
func (d *Deals) ChangeStage(ctx context.Context, params ChangeStageParams) (*domain.Deal, error) {
	var result *domain.Deal

	err := d.pool.InTx(ctx, func(tx pgx.Tx) error {
		var (
			currentStage   domain.Stage
			currentOutcome domain.Outcome
			stageChangedAt time.Time
		)

		accessCondition := "deals.dealer_id = $2"
		if params.IsAdmin {
			accessCondition = "($2 IS NOT NULL)"
		}

		err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT stage, outcome, stage_changed_at
			FROM deals
			WHERE deals.id = $1 AND %s
			FOR UPDATE`, accessCondition),
			params.DealID, params.DealerID,
		).Scan(&currentStage, &currentOutcome, &stageChangedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("чтение текущего этапа: %w", err)
		}

		if err := domain.ValidateStageTransition(currentStage, params.ToStage, currentOutcome); err != nil {
			return err
		}

		duration := int(time.Since(stageChangedAt).Seconds())

		// $2 присваивается колонке deal_stage и сравнивается со строками.
		// Без явного приведения Postgres не выводит единый тип (42P08).
		row := tx.QueryRow(ctx, `
			UPDATE deals SET
				stage = $2::deal_stage,
				stage_changed_at = now(),
				first_contacted_at = CASE
					WHEN $3::boolean THEN COALESCE(first_contacted_at, now())
					ELSE first_contacted_at
				END,
				contract_signed_at = CASE WHEN $2::deal_stage = 'contract' THEN COALESCE(contract_signed_at, now()) ELSE contract_signed_at END,
				paid_at            = CASE WHEN $2::deal_stage = 'payment'  THEN COALESCE(paid_at, now())            ELSE paid_at END,
				shipped_at         = CASE WHEN $2::deal_stage = 'shipping' THEN COALESCE(shipped_at, now())         ELSE shipped_at END,
				customs_cleared_at = CASE WHEN $2::deal_stage = 'customs'  THEN COALESCE(customs_cleared_at, now()) ELSE customs_cleared_at END,
				handed_over_at     = CASE WHEN $2::deal_stage = 'handover' THEN COALESCE(handed_over_at, now())     ELSE handed_over_at END
			WHERE id = $1
			RETURNING `+dealColumns,
			params.DealID, string(params.ToStage),
			currentStage == domain.StageLead && params.ToStage != domain.StageLead)

		deal, err := scanDeal(row)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stage_history
			    (deal_id, from_stage, to_stage, outcome, changed_by, comment, duration_seconds)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			params.DealID, currentStage, params.ToStage, currentOutcome,
			params.ChangedBy, truncate(params.Comment, 1000), duration); err != nil {
			return fmt.Errorf("запись истории этапа: %w", err)
		}

		result = deal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CloseParams — параметры закрытия сделки.
type CloseParams struct {
	DealID     uuid.UUID
	DealerID   uuid.UUID
	IsAdmin    bool
	Outcome    domain.Outcome
	LostReason string
	ChangedBy  uuid.UUID
}

// Close закрывает сделку с указанным исходом.
func (d *Deals) Close(ctx context.Context, params CloseParams) (*domain.Deal, error) {
	var result *domain.Deal

	err := d.pool.InTx(ctx, func(tx pgx.Tx) error {
		var (
			currentStage   domain.Stage
			currentOutcome domain.Outcome
		)

		accessCondition := "deals.dealer_id = $2"
		if params.IsAdmin {
			accessCondition = "($2 IS NOT NULL)"
		}

		err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT stage, outcome FROM deals
			WHERE deals.id = $1 AND %s
			FOR UPDATE`, accessCondition),
			params.DealID, params.DealerID,
		).Scan(&currentStage, &currentOutcome)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("чтение состояния сделки: %w", err)
		}

		if currentOutcome != domain.OutcomeOpen {
			return errors.New("сделка уже закрыта")
		}
		if params.Outcome == domain.OutcomeWon && !domain.CanCloseAsWon(currentStage) {
			return errors.New("успешной можно признать только сделку, дошедшую до выдачи автомобиля")
		}
		if params.Outcome == domain.OutcomeLost && strings.TrimSpace(params.LostReason) == "" {
			return errors.New("укажите причину отказа")
		}

		row := tx.QueryRow(ctx, `
			UPDATE deals SET
				outcome = $2,
				closed_at = now(),
				lost_reason = NULLIF($3, '')
			WHERE id = $1
			RETURNING `+dealColumns,
			params.DealID, params.Outcome, truncate(params.LostReason, 1000))

		deal, err := scanDeal(row)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO deal_stage_history (deal_id, from_stage, to_stage, outcome, changed_by, comment)
			VALUES ($1, $2, $2, $3, $4, $5)`,
			params.DealID, currentStage, params.Outcome, params.ChangedBy,
			truncate(params.LostReason, 1000)); err != nil {
			return fmt.Errorf("запись истории закрытия: %w", err)
		}

		result = deal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// StageHistoryEntry — запись истории переходов.
type StageHistoryEntry struct {
	FromStage       *domain.Stage
	ToStage         domain.Stage
	Outcome         domain.Outcome
	ChangedByName   string
	Comment         string
	DurationSeconds *int
	CreatedAt       time.Time
}

// StageHistory возвращает историю переходов сделки.
func (d *Deals) StageHistory(ctx context.Context, dealID uuid.UUID) ([]StageHistoryEntry, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT h.from_stage, h.to_stage, h.outcome,
		       COALESCE(u.full_name, 'Система'), h.comment, h.duration_seconds, h.created_at
		FROM deal_stage_history h
		LEFT JOIN users u ON u.id = h.changed_by
		WHERE h.deal_id = $1
		ORDER BY h.created_at, h.id`, dealID)
	if err != nil {
		return nil, fmt.Errorf("чтение истории этапов: %w", err)
	}
	defer rows.Close()

	var out []StageHistoryEntry
	for rows.Next() {
		var entry StageHistoryEntry
		if err := rows.Scan(&entry.FromStage, &entry.ToStage, &entry.Outcome,
			&entry.ChangedByName, &entry.Comment, &entry.DurationSeconds, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("разбор записи истории: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// UpdateDealParams — изменяемые поля сделки.
type UpdateDealParams struct {
	DealID   uuid.UUID
	DealerID uuid.UUID
	IsAdmin  bool

	Title              string
	AmountMinor        *int64
	Currency           domain.Currency
	AmountRubMinor     *int64
	PaidRubMinor       *int64
	ExpectedHandoverAt *time.Time
	ManagerNote        *string
	SellerID           *uuid.UUID

	CarID      *uuid.UUID
	ClearCarID bool

	ServicesNote          *string
	DestinationPort       *string
	ShippingTracking      *string
	CustomsDutiesRubMinor *int64
	SBKTSNumber           *string
	SBKTSIssuedAt         *time.Time
	ClearSBKTSIssuedAt    bool
	ArrivedAt             *time.Time
	ClearArrivedAt        bool
	FirstContactedAt      *time.Time
}

// Update изменяет реквизиты сделки.
func (d *Deals) Update(ctx context.Context, params UpdateDealParams) (*domain.Deal, error) {
	accessCondition := "dealer_id = $2"
	if params.IsAdmin {
		accessCondition = "($2 IS NOT NULL)"
	}

	row := d.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE deals SET
			title = COALESCE(NULLIF($3, ''), title),
			amount_minor = COALESCE($4, amount_minor),
			currency = COALESCE($5, currency),
			amount_rub_minor = COALESCE($6, amount_rub_minor),
			paid_rub_minor = COALESCE($7, paid_rub_minor),
			expected_handover_at = COALESCE($8, expected_handover_at),
			manager_note = COALESCE($9, manager_note),
			seller_id = COALESCE($10, seller_id),
			car_id = CASE
				WHEN $11::boolean THEN NULL
				WHEN $12::uuid IS NOT NULL THEN $12
				ELSE car_id
			END,
			services_note = COALESCE($13, services_note),
			destination_port = COALESCE($14, destination_port),
			shipping_tracking = COALESCE($15, shipping_tracking),
			customs_duties_rub_minor = COALESCE($16, customs_duties_rub_minor),
			sbkts_number = COALESCE($17, sbkts_number),
			sbkts_issued_at = CASE
				WHEN $18::boolean THEN NULL
				WHEN $19::timestamptz IS NOT NULL THEN $19
				ELSE sbkts_issued_at
			END,
			arrived_at = CASE
				WHEN $20::boolean THEN NULL
				WHEN $21::timestamptz IS NOT NULL THEN $21
				ELSE arrived_at
			END,
			first_contacted_at = COALESCE($22, first_contacted_at)
		WHERE id = $1 AND %s
		RETURNING %s`, accessCondition, dealColumns),
		params.DealID, params.DealerID,
		params.Title, params.AmountMinor, nullableCurrency(params.Currency), params.AmountRubMinor,
		params.PaidRubMinor, params.ExpectedHandoverAt, params.ManagerNote, params.SellerID,
		params.ClearCarID, params.CarID,
		params.ServicesNote, params.DestinationPort, params.ShippingTracking,
		params.CustomsDutiesRubMinor, params.SBKTSNumber,
		params.ClearSBKTSIssuedAt, params.SBKTSIssuedAt,
		params.ClearArrivedAt, params.ArrivedAt,
		params.FirstContactedAt)

	return scanDeal(row)
}

func nullableCurrency(currency domain.Currency) any {
	if currency == "" {
		return nil
	}
	return currency
}

// --- Аналитика --------------------------------------------------------------

// StageStat — сводка по одному этапу.
type StageStat struct {
	Stage          domain.Stage `json:"stage"`
	Title          string       `json:"title"`
	Count          int          `json:"count"`
	AmountRubMinor int64        `json:"amount_rub_minor"`
	StaleCount     int          `json:"stale_count"`
	AvgDaysOnStage float64      `json:"avg_days_on_stage"`
}

// PipelineSummary — сводка воронки дилера.
type PipelineSummary struct {
	Stages []StageStat `json:"stages"`

	OpenCount  int `json:"open_count"`
	WonCount   int `json:"won_count"`
	LostCount  int `json:"lost_count"`
	StaleCount int `json:"stale_count"`

	OpenAmountRubMinor int64 `json:"open_amount_rub_minor"`
	WonAmountRubMinor  int64 `json:"won_amount_rub_minor"`
	LostAmountRubMinor int64 `json:"lost_amount_rub_minor"`

	Conversion   float64 `json:"conversion"`
	AvgCycleDays float64 `json:"avg_cycle_days"`

	RevenueByMonth   []MonthRevenue `json:"revenue_by_month"`
	CreatedByMonth   []MonthCount   `json:"created_by_month"`
	ByOrigin         []OriginStat   `json:"by_origin"`
	RequestsByStatus []CountBucket  `json:"requests_by_status"`
	ListingsByStatus []CountBucket  `json:"listings_by_status"`
	Ads              AdStats        `json:"ads"`
}

// MonthRevenue — выручка закрытых сделок за календарный месяц.
type MonthRevenue struct {
	Month          string `json:"month"`
	AmountRubMinor int64  `json:"amount_rub_minor"`
	DealsCount     int    `json:"deals_count"`
}

// MonthCount — сколько сделок открыли за календарный месяц.
type MonthCount struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// OriginStat — воронка и выручка в разрезе рынка лота.
type OriginStat struct {
	Origin             string `json:"origin"`
	Title              string `json:"title"`
	Open               int    `json:"open"`
	Won                int    `json:"won"`
	Lost               int    `json:"lost"`
	OpenAmountRubMinor int64  `json:"open_amount_rub_minor"`
	WonAmountRubMinor  int64  `json:"won_amount_rub_minor"`
}

// CountBucket — число записей с ключом и подписью для отчёта.
type CountBucket struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// AdStats — показы и клики баннеров дилера.
type AdStats struct {
	Count       int     `json:"count"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	CTR         float64 `json:"ctr"`
}

// Summary считает сводку воронки.
//
// Считается одним проходом по таблице сделок с группировкой. Отдельные
// запросы на каждый этап давали бы семь обращений к базе на открытие
// дашборда.
func (d *Deals) Summary(ctx context.Context, dealerID uuid.UUID) (*PipelineSummary, error) {
	summary := &PipelineSummary{}

	rows, err := d.pool.Query(ctx, fmt.Sprintf(`
		SELECT stage,
		       count(*),
		       COALESCE(sum(COALESCE(amount_rub_minor, 0)), 0),
		       count(*) FILTER (WHERE stage_changed_at < now() - %s),
		       COALESCE(avg(EXTRACT(EPOCH FROM (now() - stage_changed_at)) / 86400.0), 0)
		FROM deals
		WHERE dealer_id = $1 AND outcome = 'open'
		GROUP BY stage`, staleIntervalExpression()), dealerID)
	if err != nil {
		return nil, fmt.Errorf("сводка по этапам: %w", err)
	}
	defer rows.Close()

	byStage := make(map[domain.Stage]StageStat)
	for rows.Next() {
		var stat StageStat
		if err := rows.Scan(&stat.Stage, &stat.Count, &stat.AmountRubMinor,
			&stat.StaleCount, &stat.AvgDaysOnStage); err != nil {
			return nil, fmt.Errorf("разбор сводки по этапу: %w", err)
		}
		byStage[stat.Stage] = stat
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение сводки по этапам: %w", err)
	}

	// Этапы возвращаются всегда все семь, даже пустые: канбан не должен
	// терять колонки из-за отсутствия сделок в них.
	for _, meta := range domain.StagesCatalog() {
		stat, ok := byStage[meta.Stage]
		if !ok {
			stat = StageStat{Stage: meta.Stage}
		}
		stat.Title = meta.Title
		summary.Stages = append(summary.Stages, stat)
		summary.OpenCount += stat.Count
		summary.OpenAmountRubMinor += stat.AmountRubMinor
		summary.StaleCount += stat.StaleCount
	}

	err = d.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE outcome = 'won'),
			count(*) FILTER (WHERE outcome = 'lost'),
			COALESCE(sum(COALESCE(amount_rub_minor, 0)) FILTER (WHERE outcome = 'won'), 0),
			COALESCE(sum(COALESCE(amount_rub_minor, 0)) FILTER (WHERE outcome = 'lost'), 0),
			COALESCE(avg(EXTRACT(EPOCH FROM (closed_at - created_at)) / 86400.0)
			         FILTER (WHERE outcome = 'won'), 0)
		FROM deals
		WHERE dealer_id = $1 AND outcome <> 'open'`, dealerID,
	).Scan(&summary.WonCount, &summary.LostCount, &summary.WonAmountRubMinor, &summary.LostAmountRubMinor, &summary.AvgCycleDays)
	if err != nil {
		return nil, fmt.Errorf("сводка по закрытым сделкам: %w", err)
	}

	if closed := summary.WonCount + summary.LostCount; closed > 0 {
		summary.Conversion = float64(summary.WonCount) / float64(closed)
	}

	monthRows, err := d.pool.Query(ctx, `
		SELECT to_char(date_trunc('month', closed_at), 'YYYY-MM'),
		       COALESCE(sum(COALESCE(amount_rub_minor, 0)), 0),
		       count(*)::int
		FROM deals
		WHERE dealer_id = $1
		  AND outcome = 'won'
		  AND closed_at IS NOT NULL
		  AND closed_at >= date_trunc('month', now()) - interval '11 months'
		GROUP BY 1
		ORDER BY 1`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("выручка по месяцам: %w", err)
	}
	defer monthRows.Close()

	summary.RevenueByMonth = make([]MonthRevenue, 0, 12)
	for monthRows.Next() {
		var row MonthRevenue
		if err := monthRows.Scan(&row.Month, &row.AmountRubMinor, &row.DealsCount); err != nil {
			return nil, fmt.Errorf("разбор выручки за месяц: %w", err)
		}
		summary.RevenueByMonth = append(summary.RevenueByMonth, row)
	}
	if err := monthRows.Err(); err != nil {
		return nil, fmt.Errorf("чтение выручки по месяцам: %w", err)
	}
	monthRows.Close()

	createdRows, err := d.pool.Query(ctx, `
		SELECT to_char(date_trunc('month', created_at), 'YYYY-MM'),
		       count(*)::int
		FROM deals
		WHERE dealer_id = $1
		  AND created_at >= date_trunc('month', now()) - interval '11 months'
		GROUP BY 1
		ORDER BY 1`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("сделки по месяцам: %w", err)
	}
	summary.CreatedByMonth = make([]MonthCount, 0, 12)
	for createdRows.Next() {
		var row MonthCount
		if err := createdRows.Scan(&row.Month, &row.Count); err != nil {
			createdRows.Close()
			return nil, fmt.Errorf("разбор сделок за месяц: %w", err)
		}
		summary.CreatedByMonth = append(summary.CreatedByMonth, row)
	}
	if err := createdRows.Err(); err != nil {
		createdRows.Close()
		return nil, fmt.Errorf("чтение сделок по месяцам: %w", err)
	}
	createdRows.Close()

	originRows, err := d.pool.Query(ctx, `
		SELECT COALESCE(cars.origin::text, ''),
		       count(*) FILTER (WHERE deals.outcome = 'open'),
		       count(*) FILTER (WHERE deals.outcome = 'won'),
		       count(*) FILTER (WHERE deals.outcome = 'lost'),
		       COALESCE(sum(COALESCE(deals.amount_rub_minor, 0)) FILTER (WHERE deals.outcome = 'open'), 0),
		       COALESCE(sum(COALESCE(deals.amount_rub_minor, 0)) FILTER (WHERE deals.outcome = 'won'), 0)
		FROM deals
		LEFT JOIN cars ON cars.id = deals.car_id
		WHERE deals.dealer_id = $1
		GROUP BY 1
		ORDER BY 1`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("сводка по рынкам: %w", err)
	}
	summary.ByOrigin = make([]OriginStat, 0, 3)
	for originRows.Next() {
		var row OriginStat
		if err := originRows.Scan(&row.Origin, &row.Open, &row.Won, &row.Lost,
			&row.OpenAmountRubMinor, &row.WonAmountRubMinor); err != nil {
			originRows.Close()
			return nil, fmt.Errorf("разбор сводки по рынку: %w", err)
		}
		row.Title = originBucketTitle(row.Origin)
		summary.ByOrigin = append(summary.ByOrigin, row)
	}
	if err := originRows.Err(); err != nil {
		originRows.Close()
		return nil, fmt.Errorf("чтение сводки по рынкам: %w", err)
	}
	originRows.Close()

	requestBuckets, err := d.countByKey(ctx, `
		SELECT status::text, count(*)::int
		FROM requests
		WHERE dealer_id = $1
		GROUP BY 1
		ORDER BY 1`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("заявки по статусам: %w", err)
	}
	for i := range requestBuckets {
		requestBuckets[i].Title = domain.RequestStatus(requestBuckets[i].Key).Title()
	}
	summary.RequestsByStatus = requestBuckets

	listingBuckets, err := d.countByKey(ctx, `
		SELECT status::text, count(*)::int
		FROM cars
		WHERE dealer_id = $1
		GROUP BY 1
		ORDER BY 1`, dealerID)
	if err != nil {
		return nil, fmt.Errorf("объявления по статусам: %w", err)
	}
	for i := range listingBuckets {
		listingBuckets[i].Title = domain.CarStatus(listingBuckets[i].Key).Title()
	}
	summary.ListingsByStatus = listingBuckets

	err = d.pool.QueryRow(ctx, `
		SELECT count(*)::int,
		       COALESCE(sum(impressions), 0)::bigint,
		       COALESCE(sum(clicks), 0)::bigint
		FROM banners
		WHERE dealer_id = $1`, dealerID,
	).Scan(&summary.Ads.Count, &summary.Ads.Impressions, &summary.Ads.Clicks)
	if err != nil {
		return nil, fmt.Errorf("сводка по рекламе: %w", err)
	}
	if summary.Ads.Impressions > 0 {
		summary.Ads.CTR = float64(summary.Ads.Clicks) / float64(summary.Ads.Impressions)
	}

	return summary, nil
}

func originBucketTitle(origin string) string {
	if origin == "" {
		return "Без лота"
	}
	value := domain.Origin(origin)
	if value.Valid() {
		return value.Title()
	}
	return origin
}

func (d *Deals) countByKey(ctx context.Context, query string, dealerID uuid.UUID) ([]CountBucket, error) {
	rows, err := d.pool.Query(ctx, query, dealerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CountBucket, 0)
	for rows.Next() {
		var row CountBucket
		if err := rows.Scan(&row.Key, &row.Count); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
