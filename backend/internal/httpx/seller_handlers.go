package httpx

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/service"
	"github.com/autoimport/crm/internal/store"
)

// SellerHandler — обработчики базы зарубежных продавцов.
type SellerHandler struct {
	sellers *service.Sellers
}

func NewSellerHandler(sellers *service.Sellers) *SellerHandler {
	return &SellerHandler{sellers: sellers}
}

var (
	allowedSellerKinds = sellerKindValues()
	allowedSellerSorts = []string{"rating", "name", "new", "experience"}
)

func sellerKindValues() []string {
	out := make([]string, 0, len(domain.SellerKindOrder))
	for _, kind := range domain.SellerKindOrder {
		out = append(out, string(kind))
	}
	return out
}

// --- Представления ----------------------------------------------------------

type sellerResponse struct {
	ID uuid.UUID `json:"id"`

	Country      string `json:"country"`
	CountryTitle string `json:"country_title"`
	Kind         string `json:"kind"`
	KindTitle    string `json:"kind_title"`

	Name        string `json:"name"`
	NameLocal   string `json:"name_local,omitempty"`
	DisplayName string `json:"display_name"`
	Region      string `json:"region"`
	City        string `json:"city,omitempty"`
	Address     string `json:"address,omitempty"`

	Brands      []string `json:"brands"`
	Description string   `json:"description,omitempty"`
	Website     string   `json:"website,omitempty"`
	LogoURL     string   `json:"logo_url,omitempty"`

	// Contacts приходят пустыми, если запрашивающий не имеет на них права.
	Contacts map[string]string `json:"contacts"`

	MinOrderQty           int  `json:"min_order_qty"`
	ExportExperienceYears *int `json:"export_experience_years,omitempty"`

	RatingAvg   float64 `json:"rating_avg"`
	RatingCount int     `json:"rating_count"`

	IsVerified bool       `json:"is_verified"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	IsActive   bool       `json:"is_active"`

	CanEdit bool `json:"can_edit"`

	// Note и IsTrusted — приватная отметка дилера об этом поставщике.
	Note      string `json:"note,omitempty"`
	IsTrusted bool   `json:"is_trusted,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

func toSellerResponse(view service.SellerView) sellerResponse {
	seller := view.Seller

	brands := seller.Brands
	if brands == nil {
		brands = []string{}
	}
	contacts := view.Contacts
	if contacts == nil {
		contacts = map[string]string{}
	}

	response := sellerResponse{
		ID:           seller.ID,
		Country:      string(seller.Country),
		CountryTitle: seller.Country.Title(),
		Kind:         string(seller.Kind),
		KindTitle:    seller.Kind.Title(),

		Name:        seller.Name,
		NameLocal:   seller.NameLocal,
		DisplayName: seller.DisplayName(),
		Region:      seller.Region,
		City:        seller.City,
		Address:     seller.Address,

		Brands:      brands,
		Description: seller.Description,
		Website:     seller.Website,
		LogoURL:     seller.LogoURL,
		Contacts:    contacts,

		MinOrderQty:           seller.MinOrderQty,
		ExportExperienceYears: seller.ExportExperienceYears,

		RatingAvg:   seller.RatingAvg,
		RatingCount: seller.RatingCount,

		IsVerified: seller.IsVerified(),
		VerifiedAt: seller.VerifiedAt,
		IsActive:   seller.IsActive,

		CanEdit:   view.CanEdit,
		CreatedAt: seller.CreatedAt,
	}

	if view.Link != nil {
		response.Note = view.Link.Note
		response.IsTrusted = view.Link.IsTrusted
	}
	return response
}

func toSellerList(views []service.SellerView) []sellerResponse {
	out := make([]sellerResponse, 0, len(views))
	for _, view := range views {
		out = append(out, toSellerResponse(view))
	}
	return out
}

// --- Обработчики ------------------------------------------------------------

// List — GET /api/v1/sellers
func (h *SellerHandler) List(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)

	filter := store.SellerFilter{
		Countries: q.EnumList("country", 2, allowedOrigins...),
		Kinds:     q.EnumList("kind", 5, allowedSellerKinds...),
		Regions:   q.StringList("region", 20, 120),
		Brands:    q.StringList("brand", 20, 60),
		Search:    q.String("q", 200),
		Sort:      q.Enum("sort", allowedSellerSorts...),
	}
	if verified := q.Bool("verified"); verified != nil {
		filter.VerifiedOnly = *verified
	}
	if inactive := q.Bool("inactive"); inactive != nil {
		filter.IncludeInactive = *inactive
	}
	filter.Limit, filter.Offset = readOffsetPage(q, 24, 100)

	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	views, total, err := h.sellers.List(r.Context(), filter, viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"items": toSellerList(views),
		"total": total,
	})
}

// Facets — GET /api/v1/sellers/facets
func (h *SellerHandler) Facets(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	countries := q.EnumList("country", 2, allowedOrigins...)
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	facets, err := h.sellers.Facets(r.Context(), countries)
	if err != nil {
		Error(w, r, err)
		return
	}

	facets["sorts"] = []map[string]string{
		{"value": "rating", "title": "Проверенные и с высоким рейтингом"},
		{"value": "name", "title": "По названию"},
		{"value": "new", "title": "Сначала новые"},
		{"value": "experience", "title": "По опыту экспорта"},
	}
	JSON(w, http.StatusOK, facets)
}

// Mine — GET /api/v1/sellers/my
func (h *SellerHandler) Mine(w http.ResponseWriter, r *http.Request) {
	views, err := h.sellers.ListMine(r.Context(), viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"items": toSellerList(views)})
}

// Get — GET /api/v1/sellers/{id}
func (h *SellerHandler) Get(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	view, err := h.sellers.Get(r.Context(), sellerID, viewerFrom(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"seller": toSellerResponse(*view)})
}

type sellerBody struct {
	Country string `json:"country"`
	Kind    string `json:"kind"`

	Name      string `json:"name"`
	NameLocal string `json:"name_local"`
	Region    string `json:"region"`
	City      string `json:"city"`
	Address   string `json:"address"`

	Brands      []string          `json:"brands"`
	Description string            `json:"description"`
	Website     string            `json:"website"`
	Contacts    map[string]string `json:"contacts"`
	LogoURL     string            `json:"logo_url"`

	MinOrderQty           int  `json:"min_order_qty"`
	ExportExperienceYears *int `json:"export_experience_years"`
}

func (b sellerBody) toForm() service.SellerForm {
	return service.SellerForm{
		Country:               b.Country,
		Kind:                  b.Kind,
		Name:                  b.Name,
		NameLocal:             b.NameLocal,
		Region:                b.Region,
		City:                  b.City,
		Address:               b.Address,
		Brands:                b.Brands,
		Description:           b.Description,
		Website:               b.Website,
		Contacts:              b.Contacts,
		LogoURL:               b.LogoURL,
		MinOrderQty:           b.MinOrderQty,
		ExportExperienceYears: b.ExportExperienceYears,
	}
}

// Create — POST /api/v1/sellers
func (h *SellerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body sellerBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	seller, err := h.sellers.Create(r.Context(), viewerFrom(r), body.toForm())
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{
		"seller": toSellerResponse(service.SellerView{
			Seller: seller, Contacts: seller.Contacts, CanEdit: true,
		}),
	})
}

// Update — PUT /api/v1/sellers/{id}
func (h *SellerHandler) Update(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body sellerBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	seller, err := h.sellers.Update(r.Context(), sellerID, viewerFrom(r), body.toForm())
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"seller": toSellerResponse(service.SellerView{
			Seller: seller, Contacts: seller.Contacts, CanEdit: true,
		}),
	})
}

type sellerActiveBody struct {
	Active bool `json:"active"`
}

// SetActive — PATCH /api/v1/sellers/{id}/active
func (h *SellerHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body sellerActiveBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	if err := h.sellers.SetActive(r.Context(), sellerID, viewerFrom(r), body.Active); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"active": body.Active})
}

type sellerVerifyBody struct {
	Verified bool `json:"verified"`
}

// SetVerified — PATCH /api/v1/admin/sellers/{id}/verify
func (h *SellerHandler) SetVerified(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body sellerVerifyBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	if err := h.sellers.SetVerified(r.Context(), sellerID, viewerFrom(r), body.Verified); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"verified": body.Verified})
}

type sellerNoteBody struct {
	Note      string `json:"note"`
	IsTrusted bool   `json:"is_trusted"`
}

// SaveNote — PUT /api/v1/sellers/{id}/note
func (h *SellerHandler) SaveNote(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body sellerNoteBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.sellers.SaveLink(r.Context(), sellerID, actor.UserID, body.Note, body.IsTrusted); err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"note": body.Note, "is_trusted": body.IsTrusted})
}

// DeleteNote — DELETE /api/v1/sellers/{id}/note
func (h *SellerHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	sellerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.sellers.DeleteLink(r.Context(), sellerID, actor.UserID); err != nil {
		Error(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
