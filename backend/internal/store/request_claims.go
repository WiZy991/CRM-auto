package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

const MaxActiveClaimsPerRequest = 5

// RequestClaim — взятие заявки конкретным дилером.
type RequestClaim struct {
	ID        uuid.UUID
	RequestID uuid.UUID
	DealerID  uuid.UUID
	DealID    *uuid.UUID
	Status    domain.ClaimStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func scanClaim(row pgx.Row) (*RequestClaim, error) {
	var c RequestClaim
	err := row.Scan(&c.ID, &c.RequestID, &c.DealerID, &c.DealID, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение claim: %w", err)
	}
	return &c, nil
}

// Claim закрепляет заявку за дилером при свободном слоте (макс. 5 active).
func (r *Requests) Claim(ctx context.Context, requestID, dealerID uuid.UUID) (*domain.Request, error) {
	var result *domain.Request

	err := r.pool.InTx(ctx, func(tx pgx.Tx) error {
		var status domain.RequestStatus
		var existingDealer *uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT status, dealer_id FROM requests WHERE id = $1 FOR UPDATE`,
			requestID).Scan(&status, &existingDealer)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status.IsFinal() {
			return ErrAlreadyClaimed
		}

		var already int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM request_claims
			WHERE request_id = $1 AND dealer_id = $2 AND status = 'active'`,
			requestID, dealerID).Scan(&already); err != nil {
			return err
		}
		if already > 0 {
			return ErrAlreadyClaimed
		}

		// Повторное взятие после lost/refuse — реактивируем строку.
		var prior uuid.UUID
		priorErr := tx.QueryRow(ctx, `
			SELECT id FROM request_claims
			WHERE request_id = $1 AND dealer_id = $2`, requestID, dealerID).Scan(&prior)
		if priorErr == nil {
			var activeCount int
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM request_claims
				WHERE request_id = $1 AND status = 'active'`, requestID).Scan(&activeCount); err != nil {
				return err
			}
			if activeCount >= MaxActiveClaimsPerRequest {
				return ErrPoolFull
			}
			if _, err := tx.Exec(ctx, `
				UPDATE request_claims SET status = 'active', deal_id = NULL, updated_at = now()
				WHERE id = $1`, prior); err != nil {
				return err
			}
		} else if errors.Is(priorErr, pgx.ErrNoRows) {
			var activeCount int
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM request_claims
				WHERE request_id = $1 AND status = 'active'`, requestID).Scan(&activeCount); err != nil {
				return err
			}
			if activeCount >= MaxActiveClaimsPerRequest {
				return ErrPoolFull
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO request_claims (request_id, dealer_id, status)
				VALUES ($1, $2, 'active')`, requestID, dealerID); err != nil {
				return err
			}
		} else {
			return priorErr
		}

		row := tx.QueryRow(ctx, `
			UPDATE requests
			SET status = CASE
			      WHEN status = 'new' THEN 'in_progress'::request_status
			      ELSE status
			    END,
			    dealer_id = COALESCE(dealer_id, $2)
			WHERE id = $1
			RETURNING `+requestColumns, requestID, dealerID)

		req, err := scanRequest(row)
		if err != nil {
			return err
		}
		result = req
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ErrAlreadyClaimed — у дилера уже есть активный claim или заявка закрыта.
var ErrAlreadyClaimed = errors.New("заявка уже закреплена за этим дилером или недоступна")

// ErrPoolFull — все 5 слотов заняты.
var ErrPoolFull = errors.New("по заявке уже работают 5 дилеров")

// ActiveClaimCount — число активных взятий.
func (r *Requests) ActiveClaimCount(ctx context.Context, requestID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM request_claims
		WHERE request_id = $1 AND status = 'active'`, requestID).Scan(&n)
	return n, err
}

// MarkClaimConverted связывает claim со сделкой.
func (r *Requests) MarkClaimConverted(ctx context.Context, requestID, dealerID, dealID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE request_claims
		SET status = 'converted', deal_id = $3, updated_at = now()
		WHERE request_id = $1 AND dealer_id = $2 AND status = 'active'`,
		requestID, dealerID, dealID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Claim мог отсутствовать у старых данных — создаём converted.
		_, err = r.pool.Exec(ctx, `
			INSERT INTO request_claims (request_id, dealer_id, deal_id, status)
			VALUES ($1, $2, $3, 'converted')
			ON CONFLICT (request_id, dealer_id) DO UPDATE
			SET status = 'converted', deal_id = EXCLUDED.deal_id, updated_at = now()`,
			requestID, dealerID, dealID)
		return err
	}
	return nil
}

// ReleaseClaimOnDealLost освобождает слот при отказе по сделке.
func (r *Requests) ReleaseClaimOnDealLost(ctx context.Context, dealID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE request_claims
		SET status = 'lost', updated_at = now()
		WHERE deal_id = $1 AND status IN ('active', 'converted')`, dealID)
	return err
}

// RefuseClaimByClient — клиент отказался от конкретного дилера.
// Возвращает закрытую сделку (если была открыта).
func (r *Requests) RefuseClaimByClient(ctx context.Context, requestID, clientID, dealerID uuid.UUID) (*RequestClaim, error) {
	var claim *RequestClaim

	err := r.pool.InTx(ctx, func(tx pgx.Tx) error {
		var owner uuid.UUID
		err := tx.QueryRow(ctx, `SELECT client_id FROM requests WHERE id = $1 FOR UPDATE`, requestID).Scan(&owner)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if owner != clientID {
			return ErrNotFound
		}

		row := tx.QueryRow(ctx, `
			UPDATE request_claims
			SET status = 'refused_by_client', updated_at = now()
			WHERE request_id = $1 AND dealer_id = $2 AND status IN ('active', 'converted')
			RETURNING id, request_id, dealer_id, deal_id, status, created_at, updated_at`,
			requestID, dealerID)
		c, err := scanClaim(row)
		if err != nil {
			return err
		}
		claim = c

		if c.DealID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE deals SET
					outcome = 'lost',
					closed_at = now(),
					lost_reason = 'Клиент отказался от компании'
				WHERE id = $1 AND client_id = $2 AND outcome = 'open'`,
				*c.DealID, clientID); err != nil {
				return err
			}
		}
		return nil
	})
	return claim, err
}

// ListClaimsForRequest — все claims заявки (для клиента).
func (r *Requests) ListClaimsForRequest(ctx context.Context, requestID, clientID uuid.UUID) ([]RequestClaimView, error) {
	var owner uuid.UUID
	if err := r.pool.QueryRow(ctx, `SELECT client_id FROM requests WHERE id = $1`, requestID).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if owner != clientID {
		return nil, ErrNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.request_id, c.dealer_id, c.deal_id, c.status, c.created_at, c.updated_at,
		       COALESCE(u.full_name, ''), COALESCE(d.company_name, '')
		FROM request_claims c
		JOIN users u ON u.id = c.dealer_id
		LEFT JOIN dealer_profiles d ON d.user_id = c.dealer_id
		WHERE c.request_id = $1
		ORDER BY c.created_at`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RequestClaimView
	for rows.Next() {
		var v RequestClaimView
		if err := rows.Scan(
			&v.ID, &v.RequestID, &v.DealerID, &v.DealID, &v.Status, &v.CreatedAt, &v.UpdatedAt,
			&v.DealerName, &v.CompanyName,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RequestClaimView — claim с именем дилера.
type RequestClaimView struct {
	RequestClaim
	DealerName  string
	CompanyName string
}
