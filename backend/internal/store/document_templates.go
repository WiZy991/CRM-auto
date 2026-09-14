package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/autoimport/crm/internal/domain"
)

// DocumentTemplate — DOCX-шаблон дилера с картой полей.
type DocumentTemplate struct {
	ID           uuid.UUID
	DealerID     uuid.UUID
	Title        string
	Kind         DocumentKind
	Stage        *domain.Stage
	StorageKey   string
	Bytes        int
	FieldMap     map[string]string
	Placeholders []string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DocumentTemplates — доступ к шаблонам документов.
type DocumentTemplates struct {
	pool *Pool
}

func NewDocumentTemplates(pool *Pool) *DocumentTemplates {
	return &DocumentTemplates{pool: pool}
}

const documentTemplateColumns = `
	id, dealer_id, title, kind, stage, storage_key, bytes,
	field_map, placeholders, status, created_at, updated_at`

func scanDocumentTemplate(row pgx.Row) (*DocumentTemplate, error) {
	var (
		t          DocumentTemplate
		fieldRaw   []byte
		stage      *string
		kind       string
		placeholders []string
	)
	err := row.Scan(
		&t.ID, &t.DealerID, &t.Title, &kind, &stage, &t.StorageKey, &t.Bytes,
		&fieldRaw, &placeholders, &t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("чтение шаблона: %w", err)
	}
	t.Kind = DocumentKind(kind)
	t.Placeholders = placeholders
	if t.Placeholders == nil {
		t.Placeholders = []string{}
	}
	t.FieldMap = map[string]string{}
	if len(fieldRaw) > 0 {
		_ = json.Unmarshal(fieldRaw, &t.FieldMap)
	}
	if stage != nil && *stage != "" {
		s := domain.Stage(*stage)
		t.Stage = &s
	}
	return &t, nil
}

// CreateTemplateParams — создание шаблона после загрузки DOCX.
type CreateTemplateParams struct {
	DealerID     uuid.UUID
	Title        string
	Kind         DocumentKind
	Stage        *domain.Stage
	StorageKey   string
	Bytes        int
	FieldMap     map[string]string
	Placeholders []string
}

func (d *DocumentTemplates) Create(ctx context.Context, p CreateTemplateParams) (*DocumentTemplate, error) {
	if p.FieldMap == nil {
		p.FieldMap = map[string]string{}
	}
	if p.Placeholders == nil {
		p.Placeholders = []string{}
	}
	fieldJSON, err := json.Marshal(p.FieldMap)
	if err != nil {
		return nil, err
	}
	var stage any
	if p.Stage != nil {
		stage = string(*p.Stage)
	}
	kind := p.Kind
	if kind == "" {
		kind = DocOther
	}
	return scanDocumentTemplate(d.pool.QueryRow(ctx, `
		INSERT INTO document_templates
			(dealer_id, title, kind, stage, storage_key, bytes, field_map, placeholders)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)
		RETURNING `+documentTemplateColumns,
		p.DealerID, p.Title, string(kind), stage, p.StorageKey, p.Bytes, fieldJSON, p.Placeholders,
	))
}

func (d *DocumentTemplates) ListByDealer(ctx context.Context, dealerID uuid.UUID) ([]DocumentTemplate, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT `+documentTemplateColumns+`
		FROM document_templates
		WHERE dealer_id = $1 AND status = 'active'
		ORDER BY created_at DESC`, dealerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DocumentTemplate
	for rows.Next() {
		item, err := scanDocumentTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (d *DocumentTemplates) ByID(ctx context.Context, id, dealerID uuid.UUID, isAdmin bool) (*DocumentTemplate, error) {
	if isAdmin {
		return scanDocumentTemplate(d.pool.QueryRow(ctx, `
			SELECT `+documentTemplateColumns+` FROM document_templates WHERE id = $1 AND status = 'active'`, id))
	}
	return scanDocumentTemplate(d.pool.QueryRow(ctx, `
		SELECT `+documentTemplateColumns+`
		FROM document_templates
		WHERE id = $1 AND dealer_id = $2 AND status = 'active'`, id, dealerID))
}

// UpdateMapParams — правка названия и field_map.
type UpdateMapParams struct {
	ID       uuid.UUID
	DealerID uuid.UUID
	IsAdmin  bool
	Title    string
	Kind     DocumentKind
	Stage    *domain.Stage
	FieldMap map[string]string
}

func (d *DocumentTemplates) UpdateMap(ctx context.Context, p UpdateMapParams) (*DocumentTemplate, error) {
	if p.FieldMap == nil {
		p.FieldMap = map[string]string{}
	}
	fieldJSON, err := json.Marshal(p.FieldMap)
	if err != nil {
		return nil, err
	}
	var stage any
	if p.Stage != nil {
		stage = string(*p.Stage)
	}
	kind := p.Kind
	if kind == "" {
		kind = DocOther
	}
	if p.IsAdmin {
		return scanDocumentTemplate(d.pool.QueryRow(ctx, `
			UPDATE document_templates SET
				title = $2, kind = $3, stage = $4, field_map = $5::jsonb, updated_at = now()
			WHERE id = $1 AND status = 'active'
			RETURNING `+documentTemplateColumns,
			p.ID, p.Title, string(kind), stage, fieldJSON))
	}
	return scanDocumentTemplate(d.pool.QueryRow(ctx, `
		UPDATE document_templates SET
			title = $3, kind = $4, stage = $5, field_map = $6::jsonb, updated_at = now()
		WHERE id = $1 AND dealer_id = $2 AND status = 'active'
		RETURNING `+documentTemplateColumns,
		p.ID, p.DealerID, p.Title, string(kind), stage, fieldJSON))
}

func (d *DocumentTemplates) Archive(ctx context.Context, id, dealerID uuid.UUID, isAdmin bool) error {
	q := `UPDATE document_templates SET status = 'archived', updated_at = now()
		WHERE id = $1 AND dealer_id = $2 AND status = 'active'`
	args := []any{id, dealerID}
	if isAdmin {
		q = `UPDATE document_templates SET status = 'archived', updated_at = now()
			WHERE id = $1 AND status = 'active'`
		args = []any{id}
	}
	tag, err := d.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
