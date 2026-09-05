package httpx

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/service"
)

// BannerHandler — обработчики рекламных баннеров.
type BannerHandler struct {
	banners *service.Banners
}

func NewBannerHandler(banners *service.Banners) *BannerHandler {
	return &BannerHandler{banners: banners}
}

var allowedPlacements = placementValues()

func placementValues() []string {
	out := make([]string, 0, len(domain.PlacementOrder))
	for _, placement := range domain.PlacementOrder {
		out = append(out, string(placement))
	}
	return out
}

// --- Представления ----------------------------------------------------------

// bannerPublicResponse — то, что видит посетитель площадки.
//
// Счётчики и служебные поля сюда не попадают: показатели рекламы —
// коммерческая информация дилера, и отдавать её всем подряд незачем.
type bannerPublicResponse struct {
	ID             uuid.UUID `json:"id"`
	Title          string    `json:"title"`
	Subtitle       string    `json:"subtitle,omitempty"`
	ImageURL       string    `json:"image_url"`
	ImageMobileURL string    `json:"image_mobile_url,omitempty"`
	CTALabel       string    `json:"cta_label"`
	// Href ведёт через обработчик перехода, чтобы засчитать клик.
	Href string `json:"href"`
}

type bannerResponse struct {
	ID        uuid.UUID `json:"id"`
	DealerID  uuid.UUID `json:"dealer_id"`
	Placement string    `json:"placement"`
	// PlacementTitle и StatusTitle приходят с сервера: подписи мест показа
	// и состояний должны совпадать в кабинете дилера и в админке.
	PlacementTitle string `json:"placement_title"`
	Status         string `json:"status"`
	StatusTitle    string `json:"status_title"`

	Title          string `json:"title"`
	Subtitle       string `json:"subtitle,omitempty"`
	ImageURL       string `json:"image_url"`
	ImageMobileURL string `json:"image_mobile_url,omitempty"`
	TargetURL      string `json:"target_url"`
	CTALabel       string `json:"cta_label"`

	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Weight   int       `json:"weight"`

	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	CTR         float64 `json:"ctr"`

	RejectReason string `json:"reject_reason,omitempty"`
	IsShowable   bool   `json:"is_showable"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toBannerResponse(banner *domain.Banner) bannerResponse {
	return bannerResponse{
		ID:             banner.ID,
		DealerID:       banner.DealerID,
		Placement:      string(banner.Placement),
		PlacementTitle: banner.Placement.Title(),
		Status:         string(banner.Status),
		StatusTitle:    banner.Status.Title(),

		Title:          banner.Title,
		Subtitle:       banner.Subtitle,
		ImageURL:       banner.ImageURL,
		ImageMobileURL: banner.ImageMobileURL,
		TargetURL:      banner.Href,
		CTALabel:       banner.CTALabel,

		StartsAt: banner.StartsAt,
		EndsAt:   banner.EndsAt,
		Weight:   banner.Weight,

		Impressions: banner.Impressions,
		Clicks:      banner.Clicks,
		CTR:         banner.CTR(),

		RejectReason: banner.RejectReason,
		IsShowable:   banner.IsShowable(time.Now()),

		CreatedAt: banner.CreatedAt,
		UpdatedAt: banner.UpdatedAt,
	}
}

func toBannerList(banners []domain.Banner) []bannerResponse {
	out := make([]bannerResponse, 0, len(banners))
	for index := range banners {
		out = append(out, toBannerResponse(&banners[index]))
	}
	return out
}

// --- Обработчики ------------------------------------------------------------

// Active — GET /api/v1/banners
func (h *BannerHandler) Active(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	placement := q.Enum("placement", allowedPlacements...)
	limit := 3
	if value := q.Int("limit", 1, 10); value != nil {
		limit = *value
	}
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}
	if placement == "" {
		Error(w, r, apierr.BadRequest("Укажите место показа"))
		return
	}

	banners, err := h.banners.Active(r.Context(), placement, limit)
	if err != nil {
		Error(w, r, err)
		return
	}

	items := make([]bannerPublicResponse, 0, len(banners))
	for index := range banners {
		banner := banners[index]
		items = append(items, bannerPublicResponse{
			ID:             banner.ID,
			Title:          banner.Title,
			Subtitle:       banner.Subtitle,
			ImageURL:       banner.ImageURL,
			ImageMobileURL: banner.ImageMobileURL,
			CTALabel:       banner.CTALabel,
			Href:           "/api/v1/banners/" + banner.ID.String() + "/click",
		})
	}

	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// Click — GET /api/v1/banners/{id}/click
//
// Переход выполняется редиректом, а не ссылкой напрямую: так засчитывается
// клик, а целевой адрес проверяется на действующий баннер.
func (h *BannerHandler) Click(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	target, err := h.banners.RegisterClick(r.Context(), bannerID)
	if err != nil {
		Error(w, r, err)
		return
	}

	// Переход на внешний сайт не должен передавать адрес нашей страницы
	// и доступ к window.opener.
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// Mine — GET /api/v1/dealer/banners
func (h *BannerHandler) Mine(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())

	banners, stats, err := h.banners.ListForDealer(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"items":        toBannerList(banners),
		"stats":        stats,
		"dictionaries": domain.BannerDictionaries(),
	})
}

type bannerBody struct {
	Placement string `json:"placement"`

	Title          string `json:"title"`
	Subtitle       string `json:"subtitle"`
	ImageURL       string `json:"image_url"`
	ImageMobileURL string `json:"image_mobile_url"`
	TargetURL      string `json:"target_url"`
	CTALabel       string `json:"cta_label"`

	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Weight   int       `json:"weight"`
}

func (b bannerBody) toForm() service.BannerForm {
	return service.BannerForm{
		Placement:      b.Placement,
		Title:          b.Title,
		Subtitle:       b.Subtitle,
		ImageURL:       b.ImageURL,
		ImageMobileURL: b.ImageMobileURL,
		Href:           b.TargetURL,
		CTALabel:       b.CTALabel,
		StartsAt:       b.StartsAt,
		EndsAt:         b.EndsAt,
		Weight:         b.Weight,
	}
}

// Create — POST /api/v1/dealer/banners
func (h *BannerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body bannerBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	banner, err := h.banners.Create(r.Context(), actor.UserID, body.toForm())
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{"banner": toBannerResponse(banner)})
}

// Update — PUT /api/v1/dealer/banners/{id}
func (h *BannerHandler) Update(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body bannerBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	banner, err := h.banners.Update(r.Context(), bannerID, viewerFrom(r), body.toForm())
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"banner": toBannerResponse(banner)})
}

// Submit — POST /api/v1/dealer/banners/{id}/submit
func (h *BannerHandler) Submit(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	banner, err := h.banners.Submit(r.Context(), bannerID, actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"banner": toBannerResponse(banner)})
}

type bannerPauseBody struct {
	Paused bool `json:"paused"`
}

// SetPaused — PATCH /api/v1/dealer/banners/{id}/pause
func (h *BannerHandler) SetPaused(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body bannerPauseBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	banner, err := h.banners.SetPaused(r.Context(), bannerID, actor.UserID, body.Paused)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"banner": toBannerResponse(banner)})
}

// Delete — DELETE /api/v1/dealer/banners/{id}
func (h *BannerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	if err := h.banners.Delete(r.Context(), bannerID, viewerFrom(r)); err != nil {
		Error(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PendingModeration — GET /api/v1/admin/banners
func (h *BannerHandler) PendingModeration(w http.ResponseWriter, r *http.Request) {
	q := NewQuery(r)
	limit := 50
	if value := q.Int("limit", 1, 200); value != nil {
		limit = *value
	}
	if err := q.Err(); err != nil {
		Error(w, r, err)
		return
	}

	banners, err := h.banners.PendingModeration(r.Context(), limit)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"items": toBannerList(banners)})
}

type moderateBannerBody struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

// Moderate — POST /api/v1/admin/banners/{id}/moderate
func (h *BannerHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	bannerID, err := UUIDParam(r, "id")
	if err != nil {
		Error(w, r, err)
		return
	}

	var body moderateBannerBody
	if err := DecodeJSON(w, r, &body); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	banner, err := h.banners.Moderate(r.Context(), bannerID, actor.UserID, body.Approve, body.Reason)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusOK, map[string]any{"banner": toBannerResponse(banner)})
}
