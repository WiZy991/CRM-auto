package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/docs"
	"github.com/autoimport/crm/internal/store"
)

// DocumentTemplateView — шаблон без сырого файла.
type DocumentTemplateView struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Kind         string            `json:"kind"`
	KindTitle    string            `json:"kind_title"`
	Stage        string            `json:"stage,omitempty"`
	Placeholders []string          `json:"placeholders"`
	FieldMap     map[string]string `json:"field_map"`
	Bytes        int               `json:"bytes"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func templateView(t *store.DocumentTemplate) DocumentTemplateView {
	view := DocumentTemplateView{
		ID:           t.ID.String(),
		Title:        t.Title,
		Kind:         string(t.Kind),
		KindTitle:    t.Kind.Title(),
		Placeholders: t.Placeholders,
		FieldMap:     t.FieldMap,
		Bytes:        t.Bytes,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
	if view.FieldMap == nil {
		view.FieldMap = map[string]string{}
	}
	if view.Placeholders == nil {
		view.Placeholders = []string{}
	}
	if t.Stage != nil {
		view.Stage = string(*t.Stage)
	}
	return view
}

// ListDocumentTemplates — шаблоны дилера.
func (p *Pipeline) ListDocumentTemplates(ctx context.Context, viewer Viewer) ([]DocumentTemplateView, error) {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Шаблоны документов доступны дилеру")
	}
	if p.templates == nil {
		return nil, apierr.Unavailable("Шаблоны недоступны")
	}
	items, err := p.templates.ListByDealer(ctx, viewer.UserID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	out := make([]DocumentTemplateView, 0, len(items))
	for i := range items {
		out = append(out, templateView(&items[i]))
	}
	return out, nil
}

// UploadDocumentTemplate сохраняет DOCX, извлекает маркеры и автомап.
func (p *Pipeline) UploadDocumentTemplate(
	ctx context.Context,
	viewer Viewer,
	title string,
	kindRaw string,
	stageRaw string,
	docx []byte,
) (DocumentTemplateView, error) {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return DocumentTemplateView{}, apierr.Forbidden("Только дилер загружает шаблоны")
	}
	if p.templates == nil || p.disk == nil {
		return DocumentTemplateView{}, apierr.Unavailable("Хранилище шаблонов недоступно")
	}
	if len(docx) < 64 || len(docx) > 12<<20 {
		return DocumentTemplateView{}, apierr.BadRequest("Файл DOCX должен быть от 1 КБ до 12 МБ")
	}
	// ZIP local header signature.
	if len(docx) < 4 || docx[0] != 'P' || docx[1] != 'K' {
		return DocumentTemplateView{}, apierr.BadRequest("Нужен файл .docx (не скан и не PDF)")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Шаблон документа"
	}
	if utf8.RuneCountInString(title) > 200 {
		return DocumentTemplateView{}, apierr.BadRequest("Слишком длинное название")
	}

	text, err := docs.ReadDocxText(docx)
	if err != nil {
		return DocumentTemplateView{}, apierr.BadRequest("Не удалось прочитать DOCX: " + err.Error())
	}
	markers := docs.ExtractMarkers(text)
	fieldMap := docs.AutoMap(markers)

	saved, err := p.disk.SavePrivate("templates", "docx", docx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		return DocumentTemplateView{}, apierr.Internal(err)
	}

	kind := store.DocumentKind(normalizeEnum(kindRaw))
	if !kind.Valid() {
		kind = store.DocOther
	}
	var stage *domain.Stage
	if s := normalizeEnum(stageRaw); s != "" {
		st := domain.Stage(s)
		if st.Valid() {
			stage = &st
		}
	}

	tpl, err := p.templates.Create(ctx, store.CreateTemplateParams{
		DealerID:     viewer.UserID,
		Title:        title,
		Kind:         kind,
		Stage:        stage,
		StorageKey:   saved.RelPath,
		Bytes:        saved.Bytes,
		FieldMap:     fieldMap,
		Placeholders: markers,
	})
	if err != nil {
		_ = os.Remove(mustResolve(p, saved.RelPath))
		return DocumentTemplateView{}, apierr.Internal(err)
	}
	p.recordAudit(ctx, viewer.UserID, "document_template.upload", tpl.ID.String(),
		map[string]any{"markers": len(markers)})
	return templateView(tpl), nil
}

func mustResolve(p *Pipeline, key string) string {
	if p.disk == nil {
		return ""
	}
	full, _ := p.disk.Resolve(key)
	return full
}

// UpdateDocumentTemplate правит title/kind/stage/field_map.
func (p *Pipeline) UpdateDocumentTemplate(
	ctx context.Context,
	viewer Viewer,
	templateID uuid.UUID,
	title, kindRaw, stageRaw string,
	fieldMap map[string]string,
) (DocumentTemplateView, error) {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return DocumentTemplateView{}, apierr.Forbidden("Только дилер")
	}
	if p.templates == nil {
		return DocumentTemplateView{}, apierr.Unavailable("Шаблоны недоступны")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return DocumentTemplateView{}, apierr.BadRequest("Укажите название")
	}
	kind := store.DocumentKind(normalizeEnum(kindRaw))
	if !kind.Valid() {
		kind = store.DocOther
	}
	var stage *domain.Stage
	if s := normalizeEnum(stageRaw); s != "" {
		st := domain.Stage(s)
		if st.Valid() {
			stage = &st
		}
	}
	if fieldMap == nil {
		fieldMap = map[string]string{}
	}
	for marker, key := range fieldMap {
		if key != "" && docs.FieldTitle(key) == "" {
			return DocumentTemplateView{}, apierr.Validation(map[string]string{
				marker: "неизвестный ключ поля: " + key,
			})
		}
	}
	tpl, err := p.templates.UpdateMap(ctx, store.UpdateMapParams{
		ID:       templateID,
		DealerID: viewer.UserID,
		IsAdmin:  viewer.Role == domain.RoleAdmin,
		Title:    title,
		Kind:     kind,
		Stage:    stage,
		FieldMap: fieldMap,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return DocumentTemplateView{}, apierr.NotFound("Шаблон")
		}
		return DocumentTemplateView{}, apierr.Internal(err)
	}
	return templateView(tpl), nil
}

// DeleteDocumentTemplate архивирует шаблон.
func (p *Pipeline) DeleteDocumentTemplate(ctx context.Context, viewer Viewer, templateID uuid.UUID) error {
	if viewer.Role != domain.RoleDealer && viewer.Role != domain.RoleAdmin {
		return apierr.Forbidden("Только дилер")
	}
	if p.templates == nil {
		return apierr.Unavailable("Шаблоны недоступны")
	}
	if err := p.templates.Archive(ctx, templateID, viewer.UserID, viewer.Role == domain.RoleAdmin); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Шаблон")
		}
		return apierr.Internal(err)
	}
	return nil
}

// GenerateFromTemplate заполняет DOCX данными сделки и кладёт в deal_documents.
func (p *Pipeline) GenerateFromTemplate(
	ctx context.Context,
	dealID, templateID uuid.UUID,
	viewer Viewer,
	visibleToClient bool,
) (*store.Document, []string, error) {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return nil, nil, err
	}
	if p.templates == nil || p.disk == nil {
		return nil, nil, apierr.Unavailable("Хранилище недоступно")
	}

	tpl, err := p.templates.ByID(ctx, templateID, viewer.UserID, viewer.Role == domain.RoleAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, apierr.NotFound("Шаблон")
		}
		return nil, nil, apierr.Internal(err)
	}

	payload, err := p.docPayload(ctx, dealID, viewer, tpl.Kind)
	if err != nil {
		return nil, nil, err
	}
	values := docs.ValuesFromPayload(payload)

	raw, err := p.disk.Read(tpl.StorageKey)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	filled, err := docs.FillDocx(raw, tpl.FieldMap, values)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	saved, err := p.disk.SavePrivate("docs", "docx", filled,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}

	title := fmt.Sprintf("%s, сделка № %s", tpl.Title, payload.DealNumber)
	doc, err := p.comms.AddDocument(ctx, store.AddDocumentParams{
		DealID:          dealID,
		Kind:            tpl.Kind,
		Title:           title,
		FileURL:         saved.RelPath,
		MimeType:        saved.MimeType,
		Bytes:           saved.Bytes,
		UploadedBy:      viewer.UserID,
		VisibleToClient: visibleToClient,
		IsAdmin:         viewer.Role == domain.RoleAdmin,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, apierr.NotFound("Сделка")
		}
		return nil, nil, apierr.Internal(err)
	}
	p.recordAudit(ctx, viewer.UserID, "deal.document.template", dealID.String(),
		map[string]any{"template_id": templateID.String()})
	return doc, payload.Warnings, nil
}

// DocumentFieldCatalog — справочник ключей для UI маппинга.
func DocumentFieldCatalog() []docs.DictionaryField {
	return docs.FieldCatalog()
}
