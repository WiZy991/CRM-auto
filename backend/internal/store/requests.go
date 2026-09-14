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

// Requests — заявки клиентов на подбор автомобиля.
type Requests struct {
	pool *Pool
}

func NewRequests(pool *Pool) *Requests { return &Requests{pool: pool} }

const requestColumns = `
	requests.id, requests.public_number,
	requests.client_id, requests.dealer_id, requests.car_id,
	requests.status,
	COALESCE(requests.desired_brand, ''), COALESCE(requests.desired_model, ''),
	requests.year_from, requests.year_to, requests.origin,
	requests.budget_from_rub_minor, requests.budget_to_rub_minor,
	requests.body, requests.gearbox,
	requests.comment, requests.contact_preference,
	COALESCE(requests.dealer_reply, ''), requests.replied_at,
	COALESCE(requests.rejected_reason, ''),
	requests.created_at, requests.updated_at`

func scanRequest(row pgx.Row) (*domain.Request, error) {
	var req domain.Request
	err := row.Scan(
		&req.ID, &req.PublicNumber,
		&req.ClientID, &req.DealerID, &req.CarID,
		&req.Status,
		&req.DesiredBrand, &req.DesiredModel,
		&req.YearFrom, &req.YearTo, &req.Origin,
		&req.BudgetFromRubMinor, &req.BudgetToRubMinor,
		&req.Body, &req.Gearbox,
		&req.Comment, &req.ContactPreference,
		&req.DealerReply, &req.RepliedAt,
		&req.RejectedReason,
		&req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение заявки: %w", err)
	}
	return &req, nil
}

// CreateRequestParams — параметры создания заявки.
type CreateRequestParams struct {
	ClientID uuid.UUID
	// DealerID пуст, если заявка уходит в общий пул.
	DealerID *uuid.UUID
	CarID    *uuid.UUID

	DesiredBrand string
	DesiredModel string
	YearFrom     *int
	YearTo       *int
	Origin       *domain.Origin

	BudgetFromRubMinor *int64
	BudgetToRubMinor   *int64

	Body    *domain.BodyType
	Gearbox *domain.Transmission

	Comment           string
	ContactPreference string
}

// Create создаёт заявку.
func (r *Requests) Create(ctx context.Context, params CreateRequestParams) (*domain.Request, error) {
	var result *domain.Request

	err := r.pool.InTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO requests (
				client_id, dealer_id, car_id,
				desired_brand, desired_model, year_from, year_to, origin,
				budget_from_rub_minor, budget_to_rub_minor,
				body, gearbox, comment, contact_preference
			) VALUES (
				$1, $2, $3,
				NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8,
				$9, $10,
				$11, $12, $13, $14
			)
			RETURNING `+requestColumns,
			params.ClientID, params.DealerID, params.CarID,
			params.DesiredBrand, params.DesiredModel, params.YearFrom, params.YearTo, params.Origin,
			params.BudgetFromRubMinor, params.BudgetToRubMinor,
			params.Body, params.Gearbox, params.Comment, params.ContactPreference,
		)

		request, err := scanRequest(row)
		if err != nil {
			return err
		}

		// Счётчик заявок на объявлении обновляется в той же транзакции:
		// расхождение между числом заявок и реальными записями сразу
		// подрывает доверие дилера к статистике.
		if params.CarID != nil {
			if _, err := tx.Exec(ctx,
				`UPDATE cars SET requests_count = requests_count + 1 WHERE id = $1`,
				*params.CarID); err != nil {
				return fmt.Errorf("обновление счётчика заявок объявления: %w", err)
			}
		}

		result = request
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RequestFilter — параметры выборки заявок.
type RequestFilter struct {
	ClientID *uuid.UUID
	DealerID *uuid.UUID
	// OpenPool выбирает необработанные заявки без дилера.
	OpenPool bool

	Statuses []string
	Limit    int
	Offset   int
}

// RequestListItem — заявка с данными участников.
type RequestListItem struct {
	Request domain.Request

	ClientName  string
	ClientPhone string
	DealerName  string
	CarTitle    string
	// HasDeal показывает, создана ли по заявке сделка (для текущего дилера — своя).
	HasDeal bool
	// ActiveClaims — сколько дилеров сейчас ведут заявку (0..5).
	ActiveClaims int
	// MaxClaims — лимит слотов.
	MaxClaims int
}

// List возвращает заявки по фильтру.
func (r *Requests) List(ctx context.Context, filter RequestFilter) ([]RequestListItem, int, error) {
	builder := &argBuilder{}
	where := make([]string, 0, 6)

	switch {
	case filter.OpenPool:
		// Пул: заявка не финальная и есть свободный слот (< 5 active claims).
		where = append(where, "requests.status NOT IN ('converted', 'rejected', 'closed')")
		where = append(where, `(
			SELECT count(*) FROM request_claims c
			WHERE c.request_id = requests.id AND c.status = 'active'
		) < 5`)
		if filter.DealerID != nil {
			// Не показывать заявки, которые этот дилер уже взял.
			where = append(where, fmt.Sprintf(`NOT EXISTS (
				SELECT 1 FROM request_claims c
				WHERE c.request_id = requests.id AND c.dealer_id = %s AND c.status = 'active'
			)`, builder.add(*filter.DealerID)))
		}
	case filter.DealerID != nil:
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM request_claims c
			WHERE c.request_id = requests.id AND c.dealer_id = %s
			  AND c.status IN ('active', 'converted')
		)`, builder.add(*filter.DealerID)))
	case filter.ClientID != nil:
		where = append(where, "requests.client_id = "+builder.add(*filter.ClientID))
	default:
		return nil, 0, errors.New("выборка заявок без ограничения по участнику запрещена")
	}

	if len(filter.Statuses) > 0 {
		where = append(where, fmt.Sprintf(
			"requests.status = ANY(%s::request_status[])", builder.add(filter.Statuses)))
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM requests WHERE `+whereSQL, builder.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("подсчёт заявок: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT %s,
		       client.full_name, client.phone,
		       COALESCE(dealer.full_name, ''),
		       COALESCE(cars.title, ''),
		       EXISTS (SELECT 1 FROM deals WHERE deals.request_id = requests.id),
		       (SELECT count(*)::int FROM request_claims c
		        WHERE c.request_id = requests.id AND c.status = 'active')
		FROM requests
		JOIN users AS client ON client.id = requests.client_id
		LEFT JOIN users AS dealer ON dealer.id = requests.dealer_id
		LEFT JOIN cars ON cars.id = requests.car_id
		WHERE %s
		ORDER BY requests.created_at DESC, requests.id DESC
		LIMIT %s OFFSET %s`,
		requestColumns, whereSQL, builder.add(limit), builder.add(filter.Offset))

	rows, err := r.pool.Query(ctx, query, builder.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("выборка заявок: %w", err)
	}
	defer rows.Close()

	var items []RequestListItem
	for rows.Next() {
		var item RequestListItem
		req := &item.Request

		if err := rows.Scan(
			&req.ID, &req.PublicNumber,
			&req.ClientID, &req.DealerID, &req.CarID,
			&req.Status,
			&req.DesiredBrand, &req.DesiredModel,
			&req.YearFrom, &req.YearTo, &req.Origin,
			&req.BudgetFromRubMinor, &req.BudgetToRubMinor,
			&req.Body, &req.Gearbox,
			&req.Comment, &req.ContactPreference,
			&req.DealerReply, &req.RepliedAt,
			&req.RejectedReason,
			&req.CreatedAt, &req.UpdatedAt,
			&item.ClientName, &item.ClientPhone,
			&item.DealerName, &item.CarTitle, &item.HasDeal,
			&item.ActiveClaims,
		); err != nil {
			return nil, 0, fmt.Errorf("разбор строки заявки: %w", err)
		}
		item.MaxClaims = MaxActiveClaimsPerRequest
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ByIDForParticipant возвращает заявку, доступную запрашивающему.
func (r *Requests) ByIDForParticipant(ctx context.Context, requestID, userID uuid.UUID, isAdmin, isDealer bool) (*domain.Request, error) {
	if isAdmin {
		return scanRequest(r.pool.QueryRow(ctx,
			`SELECT `+requestColumns+` FROM requests WHERE requests.id = $1`, requestID))
	}

	return scanRequest(r.pool.QueryRow(ctx, `
		SELECT `+requestColumns+` FROM requests
		WHERE requests.id = $1
		  AND (requests.client_id = $2
		       OR requests.dealer_id = $2
		       OR EXISTS (
		            SELECT 1 FROM request_claims c
		            WHERE c.request_id = requests.id AND c.dealer_id = $2
		              AND c.status IN ('active', 'converted')
		          )
		       OR ($3 AND requests.status NOT IN ('rejected', 'closed')
		           AND (SELECT count(*) FROM request_claims c
		                WHERE c.request_id = requests.id AND c.status = 'active') < 5))`,
		requestID, userID, isDealer))
}

// Reply сохраняет ответ дилера.
func (r *Requests) Reply(ctx context.Context, requestID, dealerID uuid.UUID, reply string) (*domain.Request, error) {
	return scanRequest(r.pool.QueryRow(ctx, `
		UPDATE requests
		SET dealer_reply = $3, replied_at = now(),
		    status = CASE WHEN status IN ('new', 'in_progress') THEN 'answered'::request_status ELSE status END
		WHERE id = $1
		  AND status <> 'converted'
		  AND EXISTS (
		    SELECT 1 FROM request_claims c
		    WHERE c.request_id = requests.id AND c.dealer_id = $2 AND c.status IN ('active', 'converted')
		  )
		RETURNING `+requestColumns, requestID, dealerID, reply))
}

// Reject отклоняет заявку.
func (r *Requests) Reject(ctx context.Context, requestID, dealerID uuid.UUID, reason string) (*domain.Request, error) {
	// Отказ дилера = освобождение своего слота, заявка в пуле остаётся.
	_, err := r.pool.Exec(ctx, `
		UPDATE request_claims
		SET status = 'lost', updated_at = now()
		WHERE request_id = $1 AND dealer_id = $2 AND status = 'active'`,
		requestID, dealerID)
	if err != nil {
		return nil, err
	}
	return scanRequest(r.pool.QueryRow(ctx, `
		UPDATE requests
		SET rejected_reason = $3
		WHERE id = $1
		RETURNING `+requestColumns, requestID, dealerID, reason))
}

// Close закрывает заявку по инициативе клиента.
func (r *Requests) Close(ctx context.Context, requestID, clientID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE requests SET status = 'closed'
		WHERE id = $1 AND client_id = $2 AND status <> 'converted'`, requestID, clientID)
	if err != nil {
		return fmt.Errorf("закрытие заявки: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountRecentByClient считает заявки клиента за период.
//
// Нужно как прикладное ограничение поверх ограничения частоты: лимит по
// адресу не мешает создать сто заявок с разных адресов, а сто заявок от
// одного клиента — это спам для дилеров.
func (r *Requests) CountRecentByClient(ctx context.Context, clientID uuid.UUID, window time.Duration) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM requests
		WHERE client_id = $1 AND created_at > now() - $2::interval`,
		clientID, window).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("подсчёт заявок клиента: %w", err)
	}
	return count, nil
}
