package httpx

import (
	"net/http"
	"strings"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/service"
)

// SocialHandler — самообслуживание каналов дилера.
type SocialHandler struct {
	social *service.Social
}

func NewSocialHandler(social *service.Social) *SocialHandler {
	return &SocialHandler{social: social}
}

func parseNetworkParam(r *http.Request) (domain.SocialNetwork, error) {
	raw := strings.ToLower(strings.TrimSpace(chiURLParam(r, "network")))
	network, ok := domain.ParseSocialNetwork(raw)
	if !ok {
		return "", apierr.BadRequest("Неизвестная социальная сеть")
	}
	return network, nil
}

// List — GET /api/v1/dealer/channels
func (h *SocialHandler) List(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())
	items, platform, err := h.social.ListChannels(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items, "platform": platform})
}

// Save — PUT /api/v1/dealer/channels
func (h *SocialHandler) Save(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Network           string `json:"network"`
		Token             string `json:"token"`
		APIKey            string `json:"api_key"`
		ChatID            string `json:"chat_id"`
		OwnerID           string `json:"owner_id"`
		PhoneNumberID     string `json:"phone_number_id"`
		BusinessAccountID string `json:"business_account_id"`
		Destination       string `json:"destination"`
		AutoPost          *bool  `json:"auto_post"`
		Disconnect        bool   `json:"disconnect"`
	}
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}
	network, ok := domain.ParseSocialNetwork(strings.ToLower(strings.TrimSpace(req.Network)))
	if !ok {
		Error(w, r, apierr.Validation(map[string]string{"network": "неизвестная сеть"}))
		return
	}

	actor := ActorFrom(r.Context())
	view, err := h.social.SaveChannel(r.Context(), actor.UserID, service.ChannelPut{
		Network:           network,
		Token:             req.Token,
		APIKey:            req.APIKey,
		ChatID:            req.ChatID,
		OwnerID:           req.OwnerID,
		PhoneNumberID:     req.PhoneNumberID,
		BusinessAccountID: req.BusinessAccountID,
		Destination:       req.Destination,
		AutoPost:          req.AutoPost,
		Disconnect:        req.Disconnect,
	})
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"channel": view})
}

// Test — POST /api/v1/dealer/channels/{network}/test
func (h *SocialHandler) Test(w http.ResponseWriter, r *http.Request) {
	network, err := parseNetworkParam(r)
	if err != nil {
		Error(w, r, err)
		return
	}
	actor := ActorFrom(r.Context())
	view, err := h.social.TestConnection(r.Context(), actor.UserID, network)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"channel": view, "ok": true})
}

// OAuthStart — GET /api/v1/dealer/channels/{network}/oauth/start
func (h *SocialHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	network, err := parseNetworkParam(r)
	if err != nil {
		Error(w, r, err)
		return
	}
	actor := ActorFrom(r.Context())
	target, err := h.social.OAuthStartURL(r.Context(), actor.UserID, network)
	if err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"url": target})
}

// OAuthCallback — GET /api/v1/integrations/oauth/callback
func (h *SocialHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	target, err := h.social.OAuthCallback(r.Context(), q.Get("state"), q.Get("code"), q.Get("error"))
	if err != nil {
		Error(w, r, err)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}
