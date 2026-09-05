package httpx

import (
	"net/http"
	"strings"

	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// DealerHandler — публичный каталог дилеров и карточка владельца.
type DealerHandler struct {
	dealers *service.PublicDealers
}

func NewDealerHandler(dealers *service.PublicDealers) *DealerHandler {
	return &DealerHandler{dealers: dealers}
}

// List — GET /api/v1/dealers
func (h *DealerHandler) List(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	city := q.String("city", 80)
	limit := 24
	offset := 0
	if value := q.Int("limit", 1, 50); value != nil {
		limit = *value
	}
	if value := q.Int("offset", 0, 10_000); value != nil {
		offset = *value
	}
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	items, total, err := h.dealers.List(r.Context(), city, limit, offset)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// Get — GET /api/v1/dealers/{slug}
func (h *DealerHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chiURLParam(r, "slug")))
	item, err := h.dealers.BySlug(r.Context(), slug)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"dealer": item})
}

// Cars — GET /api/v1/dealers/{slug}/cars
func (h *DealerHandler) Cars(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chiURLParam(r, "slug")))
	actor := ActorFrom(r.Context())
	page, err := h.dealers.CarsBySlug(r.Context(), slug, actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	items := make([]carListItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toCarListItem(item))
	}
	JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": page.NextCursor,
		"total":       page.Total,
	})
}

// Mine — GET /api/v1/dealers/me
func (h *DealerHandler) Mine(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())
	profile, err := h.dealers.Mine(r.Context(), actor.UserID, actor.Role)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"dealer": profile})
}

type dealerWriteBody struct {
	Slug          string   `json:"slug"`
	CompanyName   string   `json:"company_name"`
	LegalName     string   `json:"legal_name"`
	INN           string   `json:"inn"`
	City          string   `json:"city"`
	Address       string   `json:"address"`
	Description   string   `json:"description"`
	Website       string   `json:"website"`
	LogoURL       string   `json:"logo_url"`
	CoverURL      string   `json:"cover_url"`
	Services      []string `json:"services"`
	WorkCountries []string `json:"work_countries"`
}

// SaveMine — PUT /api/v1/dealers/me
func (h *DealerHandler) SaveMine(w http.ResponseWriter, r *http.Request) {
	var body dealerWriteBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	profile, err := h.dealers.SaveMine(r.Context(), actor.UserID, actor.Role, store.DealerWrite{
		Slug:          body.Slug,
		CompanyName:   body.CompanyName,
		LegalName:     body.LegalName,
		INN:           body.INN,
		City:          body.City,
		Address:       body.Address,
		Description:   body.Description,
		Website:       body.Website,
		LogoURL:       body.LogoURL,
		CoverURL:      body.CoverURL,
		Services:      body.Services,
		WorkCountries: body.WorkCountries,
	})
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"dealer": profile})
}

func toPublicReview(rev store.Review) map[string]any {
	out := map[string]any{
		"id":          rev.ID,
		"rating":      rev.Rating,
		"text":        rev.Text,
		"author_name": rev.AuthorName,
		"created_at":  rev.CreatedAt,
	}
	if rev.DealerReply != "" {
		out["dealer_reply"] = rev.DealerReply
		out["replied_at"] = rev.RepliedAt
	}
	return out
}

// Reviews — GET /api/v1/dealers/{slug}/reviews
func (h *DealerHandler) Reviews(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chiURLParam(r, "slug")))
	items, err := h.dealers.ReviewsBySlug(r.Context(), slug)
	if err != nil {
		Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toPublicReview(item))
	}
	JSON(w, http.StatusOK, map[string]any{"items": out})
}

// MineReviews — GET /api/v1/dealer/reviews
func (h *DealerHandler) MineReviews(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())
	items, err := h.dealers.MineReviews(r.Context(), actor.UserID, actor.Role)
	if err != nil {
		Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toPublicReview(item))
	}
	JSON(w, http.StatusOK, map[string]any{"items": out})
}

type reviewReplyBody struct {
	Reply string `json:"reply"`
}

// ReplyReview — POST /api/v1/dealer/reviews/{id}/reply
func (h *DealerHandler) ReplyReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}
	var body reviewReplyBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}
	actor := ActorFrom(r.Context())
	rev, err := h.dealers.ReplyReview(r.Context(), reviewID, actor.UserID, actor.Role, body.Reply)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"review": toPublicReview(*rev)})
}
