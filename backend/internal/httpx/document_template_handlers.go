package httpx

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/service"
)

// ListDocumentTemplates — GET /api/v1/dealer/document-templates
func (h *DealHandler) ListDocumentTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.pipeline.ListDocumentTemplates(r.Context(), viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"fields": service.DocumentFieldCatalog(),
	})
}

// UploadDocumentTemplate — POST /api/v1/dealer/document-templates (multipart)
func (h *DealHandler) UploadDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	const maxMem = 16 << 20
	if err := r.ParseMultipartForm(maxMem); err != nil {
		Error(w, r, apierr.BadRequest("Не удалось прочитать форму"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Error(w, r, apierr.BadRequest("Прикрепите файл .docx"))
		return
	}
	defer file.Close()
	name := strings.ToLower(header.Filename)
	if !strings.HasSuffix(name, ".docx") {
		Error(w, r, apierr.BadRequest("Нужен файл с расширением .docx"))
		return
	}
	payload, err := io.ReadAll(io.LimitReader(file, 12<<20+1))
	if err != nil {
		Error(w, r, apierr.BadRequest("Не удалось прочитать файл"))
		return
	}
	view, err := h.pipeline.UploadDocumentTemplate(
		r.Context(),
		viewerFrom(r),
		r.FormValue("title"),
		r.FormValue("kind"),
		r.FormValue("stage"),
		payload,
	)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"template": view})
}

// UpdateDocumentTemplate — PUT /api/v1/dealer/document-templates/{id}
func (h *DealHandler) UpdateDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, r, apierr.BadRequest("Некорректный id шаблона"))
		return
	}
	var body struct {
		Title    string            `json:"title"`
		Kind     string            `json:"kind"`
		Stage    string            `json:"stage"`
		FieldMap map[string]string `json:"field_map"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}
	view, err := h.pipeline.UpdateDocumentTemplate(
		r.Context(), viewerFrom(r), id, body.Title, body.Kind, body.Stage, body.FieldMap,
	)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"template": view})
}

// DeleteDocumentTemplate — DELETE /api/v1/dealer/document-templates/{id}
func (h *DealHandler) DeleteDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, r, apierr.BadRequest("Некорректный id шаблона"))
		return
	}
	if err := h.pipeline.DeleteDocumentTemplate(r.Context(), viewerFrom(r), id); err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// GenerateFromTemplate — POST /api/v1/deals/{id}/documents/from-template
func (h *DealHandler) GenerateFromTemplate(w http.ResponseWriter, r *http.Request) {
	dealID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, r, apierr.BadRequest("Некорректный id сделки"))
		return
	}
	var body struct {
		TemplateID      string `json:"template_id"`
		VisibleToClient *bool  `json:"visible_to_client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, r, apierr.BadRequest("Некорректный JSON"))
		return
	}
	templateID, err := uuid.Parse(strings.TrimSpace(body.TemplateID))
	if err != nil {
		Error(w, r, apierr.BadRequest("Укажите template_id"))
		return
	}
	visible := true
	if body.VisibleToClient != nil {
		visible = *body.VisibleToClient
	}
	doc, warnings, err := h.pipeline.GenerateFromTemplate(
		r.Context(), dealID, templateID, viewerFrom(r), visible,
	)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{
		"document": documentResponse{
			ID:              doc.ID,
			Kind:            string(doc.Kind),
			Title:           doc.Title,
			URL:             "/api/v1/documents/" + doc.ID.String(),
			MimeType:        doc.MimeType,
			Bytes:           doc.Bytes,
			SizeLabel:       formatBytes(doc.Bytes),
			VisibleToClient: doc.VisibleToClient,
			CreatedAt:       doc.CreatedAt,
		},
		"warnings": warnings,
	})
}
