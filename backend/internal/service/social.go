package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/money"
	"github.com/autoimport/crm/internal/pkg/security"
	pkgSocial "github.com/autoimport/crm/internal/pkg/social"
	"github.com/autoimport/crm/internal/store"
)

const (
	oauthStateTTL  = 10 * time.Minute
	oauthStatePref = "social:oauth:"
	outboxBatch    = 20
)

// Social — самообслуживание каналов дилера и очередь автопостинга.
type Social struct {
	accounts *store.SocialAccounts
	cars     *store.Cars
	notify   *store.Notifications
	cipher   *security.Cipher
	cfg      config.Social
	public   string
	storage  string
	rdb      *redis.Client
	log      *slog.Logger
	adapters map[domain.SocialNetwork]pkgSocial.Adapter
}

func NewSocial(
	accounts *store.SocialAccounts,
	cars *store.Cars,
	notify *store.Notifications,
	cipher *security.Cipher,
	cfg config.Social,
	publicURL, storageBase string,
	rdb *redis.Client,
	log *slog.Logger,
) *Social {
	return &Social{
		accounts: accounts,
		cars:     cars,
		notify:   notify,
		cipher:   cipher,
		cfg:      cfg,
		public:   strings.TrimRight(publicURL, "/"),
		storage:  strings.TrimRight(storageBase, "/"),
		rdb:      rdb,
		log:      log,
		adapters: map[domain.SocialNetwork]pkgSocial.Adapter{
			domain.NetworkTelegram:  pkgSocial.Telegram{},
			domain.NetworkVK:        pkgSocial.VK{},
			domain.NetworkWhatsApp:  pkgSocial.WhatsApp{},
			domain.NetworkInstagram: pkgSocial.Instagram{},
			domain.NetworkYouTube:   pkgSocial.YouTube{},
			domain.NetworkRuTube:    pkgSocial.RuTube{},
		},
	}
}

// ChannelView — канал без секретов, для кабинета.
type ChannelView struct {
	Network           string `json:"network"`
	Title             string `json:"title"`
	AuthKind          string `json:"auth_kind"`
	Status            string `json:"status"`
	ExternalID        string `json:"external_id,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	AutoPost          bool   `json:"auto_post"`
	TokenMask         string `json:"token_mask,omitempty"`
	ChatID            string `json:"chat_id,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	PhoneNumberID     string `json:"phone_number_id,omitempty"`
	BusinessAccountID string `json:"business_account_id,omitempty"`
	Destination       string `json:"destination,omitempty"`
	PlatformReady     bool   `json:"platform_ready"`
	PlatformHint      string `json:"platform_hint,omitempty"`
	PublishHint       string `json:"publish_hint,omitempty"`
}

// ChannelPut — поля, которые дилер сохраняет сам.
type ChannelPut struct {
	Network           domain.SocialNetwork
	Token             string
	APIKey            string
	ChatID            string
	OwnerID           string
	PhoneNumberID     string
	BusinessAccountID string
	Destination       string
	AutoPost          *bool
	Disconnect        bool
}

// PlatformStatus — какие приложения площадки уже заведены.
type PlatformStatus struct {
	VK     bool `json:"vk"`
	Meta   bool `json:"meta"`
	Google bool `json:"google"`
}

// ListChannels отдаёт все шесть сетей, даже если ключей ещё нет.
func (s *Social) ListChannels(ctx context.Context, dealerID uuid.UUID) ([]ChannelView, PlatformStatus, error) {
	rows, err := s.accounts.ListByDealer(ctx, dealerID)
	if err != nil {
		return nil, PlatformStatus{}, apierr.Internal(err)
	}
	byNet := make(map[domain.SocialNetwork]store.SocialAccountRow, len(rows))
	for _, row := range rows {
		byNet[row.Network] = row
	}

	views := make([]ChannelView, 0, len(domain.SocialNetworkOrder))
	for _, network := range domain.SocialNetworkOrder {
		view := s.emptyView(network)
		if row, ok := byNet[network]; ok {
			filled, err := s.viewFromRow(row)
			if err != nil {
				return nil, PlatformStatus{}, apierr.Internal(err)
			}
			view = filled
		}
		views = append(views, view)
	}
	return views, PlatformStatus{
		VK:     s.cfg.VKReady(),
		Meta:   s.cfg.MetaReady(),
		Google: s.cfg.GoogleReady(),
	}, nil
}

func (s *Social) emptyView(network domain.SocialNetwork) ChannelView {
	view := ChannelView{
		Network:       string(network),
		Title:         network.Title(),
		AuthKind:      network.AuthKind(),
		Status:        string(domain.SocialDisconnected),
		AutoPost:      true,
		PlatformReady: true,
		PublishHint:   publishHint(network),
	}
	switch network {
	case domain.NetworkInstagram:
		view.PlatformReady = s.cfg.MetaReady()
		if !view.PlatformReady {
			view.PlatformHint = "площадка ещё не подключила Meta"
		}
	case domain.NetworkYouTube:
		view.PlatformReady = s.cfg.GoogleReady()
		if !view.PlatformReady {
			view.PlatformHint = "площадка ещё не подключила Google"
		}
	case domain.NetworkWhatsApp:
		view.PlatformReady = true
		view.PublishHint = "WhatsApp — не лента. Лоты уходят сообщением в указанный чат или Channel."
	}
	return view
}

func publishHint(network domain.SocialNetwork) string {
	switch network {
	case domain.NetworkYouTube:
		return "Ролик можно будет слать, когда в лоте появится видео."
	case domain.NetworkRuTube:
		return "Автозагрузка ролика в этой версии не включена — только проверка ключа."
	case domain.NetworkWhatsApp:
		return "Автопост — сообщение в чат или Channel, не пост на стену."
	default:
		return ""
	}
}

func (s *Social) viewFromRow(row store.SocialAccountRow) (ChannelView, error) {
	view := s.emptyView(row.Network)
	view.Status = string(row.Status)
	view.ExternalID = row.ExternalID
	view.LastError = row.LastError
	view.AutoPost = row.AutoPost
	if len(row.Credentials) == 0 {
		return view, nil
	}
	creds, err := s.decrypt(row.Credentials)
	if err != nil {
		return ChannelView{}, err
	}
	view.TokenMask = pkgSocial.SecretMask(firstNonEmpty(creds.Token, creds.APIKey))
	view.ChatID = creds.ChatID
	view.OwnerID = creds.OwnerID
	view.PhoneNumberID = creds.PhoneNumberID
	view.BusinessAccountID = creds.BusinessAccountID
	view.Destination = creds.Destination
	return view, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SaveChannel сохраняет ключи дилера. Пустой token не затирает уже лежащий.
func (s *Social) SaveChannel(ctx context.Context, dealerID uuid.UUID, input ChannelPut) (ChannelView, error) {
	if !input.Network.Valid() {
		return ChannelView{}, apierr.Validation(map[string]string{"network": "неизвестная сеть"})
	}
	if input.Network.AuthKind() == "oauth" && strings.TrimSpace(input.Token) != "" {
		return ChannelView{}, apierr.Validation(map[string]string{
			"token": "для этой сети используйте кнопку «Подключить», а не вставку токена",
		})
	}

	existing, err := s.accounts.ByDealerNetwork(ctx, dealerID, input.Network)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ChannelView{}, apierr.Internal(err)
	}
	found := err == nil

	if input.Disconnect {
		if !found {
			return s.emptyView(input.Network), nil
		}
		if err := s.accounts.Clear(ctx, dealerID, input.Network); err != nil {
			return ChannelView{}, apierr.Internal(err)
		}
		return s.emptyView(input.Network), nil
	}

	creds := pkgSocial.Credentials{}
	if found && len(existing.Credentials) > 0 {
		decoded, decErr := s.decrypt(existing.Credentials)
		if decErr != nil {
			return ChannelView{}, apierr.Internal(decErr)
		}
		creds = decoded
	}

	if t := strings.TrimSpace(input.Token); t != "" {
		creds.Token = t
	}
	if k := strings.TrimSpace(input.APIKey); k != "" {
		creds.APIKey = k
		if creds.Token == "" {
			creds.Token = k
		}
	}
	creds.ChatID = strings.TrimSpace(input.ChatID)
	creds.OwnerID = strings.TrimSpace(input.OwnerID)
	creds.PhoneNumberID = strings.TrimSpace(input.PhoneNumberID)
	creds.BusinessAccountID = strings.TrimSpace(input.BusinessAccountID)
	creds.Destination = strings.TrimSpace(input.Destination)

	blob, err := s.encrypt(creds)
	if err != nil {
		return ChannelView{}, apierr.Internal(err)
	}

	autoPost := true
	status := domain.SocialDisconnected
	lastError := ""
	externalID := ""
	if found {
		autoPost = existing.AutoPost
		status = existing.Status
		lastError = existing.LastError
		externalID = existing.ExternalID
	}
	if input.AutoPost != nil {
		autoPost = *input.AutoPost
	}
	if len(blob) > 0 && status == domain.SocialDisconnected {
		status = domain.SocialConnected
		lastError = ""
	}

	row, err := s.accounts.Upsert(ctx, store.SocialAccountRow{
		DealerID:    dealerID,
		Network:     input.Network,
		Credentials: blob,
		ExternalID:  externalID,
		Status:      status,
		LastError:   lastError,
		AutoPost:    autoPost,
	})
	if err != nil {
		return ChannelView{}, apierr.Internal(err)
	}
	view, err := s.viewFromRow(row)
	if err != nil {
		return ChannelView{}, apierr.Internal(err)
	}
	return view, nil
}

// TestConnection вызывает адаптер и пишет статус канала.
func (s *Social) TestConnection(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork) (ChannelView, error) {
	if !network.Valid() {
		return ChannelView{}, apierr.Validation(map[string]string{"network": "неизвестная сеть"})
	}
	row, err := s.accounts.ByDealerNetwork(ctx, dealerID, network)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ChannelView{}, apierr.Validation(map[string]string{"network": "сначала сохраните ключ или подключите аккаунт"})
		}
		return ChannelView{}, apierr.Internal(err)
	}
	creds, err := s.decrypt(row.Credentials)
	if err != nil {
		return ChannelView{}, apierr.Internal(err)
	}
	creds, err = s.refreshIfNeeded(ctx, network, creds, dealerID)
	if err != nil {
		_ = s.markReauth(ctx, dealerID, network, err.Error())
		return ChannelView{}, apierr.BadRequest(err.Error())
	}

	adapter := s.adapters[network]
	externalID, testErr := adapter.Test(ctx, creds)
	if testErr != nil {
		status := domain.SocialError
		if errors.Is(testErr, pkgSocial.ErrNeedReauth) {
			status = domain.SocialNeedsReauth
			s.notifyReauth(ctx, dealerID, network)
		}
		_ = s.accounts.SetStatus(ctx, dealerID, network, status, testErr.Error(), "")
		return ChannelView{}, apierr.BadRequest(testErr.Error())
	}

	if err := s.accounts.SetStatus(ctx, dealerID, network, domain.SocialConnected, "", externalID); err != nil {
		return ChannelView{}, apierr.Internal(err)
	}
	row.Status = domain.SocialConnected
	row.LastError = ""
	row.ExternalID = externalID
	view, err := s.viewFromRow(row)
	if err != nil {
		return ChannelView{}, apierr.Internal(err)
	}
	return view, nil
}

func (s *Social) markReauth(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork, message string) error {
	s.notifyReauth(ctx, dealerID, network)
	return s.accounts.SetStatus(ctx, dealerID, network, domain.SocialNeedsReauth, message, "")
}

func (s *Social) notifyReauth(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork) {
	if s.notify == nil {
		return
	}
	_ = s.notify.Create(ctx, store.CreateNotificationParams{
		UserID:  dealerID,
		Kind:    store.NotifySecurityAlert,
		Title:   "Обновите ключ канала " + network.Title(),
		Body:    "Связь с " + network.Title() + " пропала. Откройте «Каналы» и подключите аккаунт заново — это можно сделать без поддержки площадки.",
		Link:    "/app/channels",
		Payload: map[string]any{"network": string(network)},
	})
}

// EnqueuePublishedCar ставит лот в outbox по всем каналам с auto_post.
func (s *Social) EnqueuePublishedCar(ctx context.Context, dealerID, carID uuid.UUID) error {
	if dealerID == uuid.Nil {
		return nil
	}
	accounts, err := s.accounts.AutoPostNetworks(ctx, dealerID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if account.Network == domain.NetworkYouTube || account.Network == domain.NetworkRuTube {
			// В v1 нет исходного видео в лоте — в очередь не кладём, чтобы
			// не засорять outbox пропусками.
			continue
		}
		if err := s.accounts.Enqueue(ctx, dealerID, carID, account.Network); err != nil {
			s.log.Error("не удалось поставить публикацию в очередь",
				slog.String("network", string(account.Network)),
				slog.String("car_id", carID.String()),
				slog.String("error", err.Error()))
		}
	}
	return nil
}

// EnqueueManual — кнопка «Опубликовать» на лоте.
func (s *Social) EnqueueManual(ctx context.Context, dealerID, carID uuid.UUID) error {
	car, err := s.cars.ByID(ctx, carID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Объявление")
		}
		return apierr.Internal(err)
	}
	if car.DealerID == nil || *car.DealerID != dealerID {
		return apierr.NotFound("Объявление")
	}
	if car.Status != domain.CarActive {
		return apierr.Conflict("Публиковать в каналы можно только лот в продаже")
	}
	if err := s.EnqueuePublishedCar(ctx, dealerID, carID); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// ProcessOutbox забирает pending и вызывает адаптеры. Ошибка одной сети
// не останавливает остальные.
func (s *Social) ProcessOutbox(ctx context.Context) {
	items, err := s.accounts.ClaimPending(ctx, outboxBatch)
	if err != nil {
		s.log.Error("очередь публикаций", slog.String("error", err.Error()))
		return
	}
	for _, item := range items {
		s.processOne(ctx, item)
	}
}

func (s *Social) processOne(ctx context.Context, item store.SocialOutboxRow) {
	account, err := s.accounts.ByDealerNetwork(ctx, item.DealerID, item.Network)
	if err != nil {
		_ = s.accounts.FinishOutbox(ctx, item.ID, "skipped", "", "канал отключён")
		return
	}
	creds, err := s.decrypt(account.Credentials)
	if err != nil {
		_ = s.accounts.RetryOrFail(ctx, item, "не удалось прочитать ключ")
		return
	}
	creds, err = s.refreshIfNeeded(ctx, item.Network, creds, item.DealerID)
	if err != nil {
		_ = s.markReauth(ctx, item.DealerID, item.Network, err.Error())
		_ = s.accounts.FinishOutbox(ctx, item.ID, "failed", "", err.Error())
		return
	}

	car, err := s.cars.ByID(ctx, item.CarID)
	if err != nil {
		_ = s.accounts.FinishOutbox(ctx, item.ID, "skipped", "", "лот не найден")
		return
	}
	listing := s.listingFromCar(car)
	adapter := s.adapters[item.Network]
	postID, pubErr := adapter.Publish(ctx, creds, listing)
	if pubErr != nil {
		if errors.Is(pubErr, pkgSocial.ErrSkipped) {
			_ = s.accounts.FinishOutbox(ctx, item.ID, "skipped", "", pubErr.Error())
			return
		}
		if errors.Is(pubErr, pkgSocial.ErrNeedReauth) {
			_ = s.markReauth(ctx, item.DealerID, item.Network, pubErr.Error())
			_ = s.accounts.FinishOutbox(ctx, item.ID, "failed", "", pubErr.Error())
			return
		}
		s.log.Warn("публикация в сеть не удалась",
			slog.String("network", string(item.Network)),
			slog.String("car_id", item.CarID.String()),
			slog.String("error", pubErr.Error()))
		_ = s.accounts.RetryOrFail(ctx, item, pubErr.Error())
		return
	}
	_ = s.accounts.FinishOutbox(ctx, item.ID, "sent", postID, "")
	_ = s.accounts.SetStatus(ctx, item.DealerID, item.Network, domain.SocialConnected, "", account.ExternalID)
}

func (s *Social) listingFromCar(car *domain.Car) pkgSocial.Listing {
	pageURL := s.public + "/catalog/" + car.ID.String()
	photos := make([]string, 0, len(car.Photos))
	for _, photo := range car.Photos {
		if abs := s.absoluteURL(photo.URL); abs != "" {
			photos = append(photos, abs)
		}
	}
	price := money.Format(car.PriceMinor, car.Currency)
	if car.PriceRubMinor > 0 {
		price = money.FormatRub(car.PriceRubMinor)
	}
	return pkgSocial.Listing{
		Title:     car.DisplayName(),
		Caption:   pkgSocial.BuildCaption(car.Brand, car.Model, car.Year, price, pageURL),
		URL:       pageURL,
		PhotoURLs: photos,
		HasVideo:  false,
	}
}

func (s *Social) absoluteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		base := s.storage
		if strings.HasPrefix(raw, "/uploads") || strings.HasPrefix(raw, "/files") {
			base = s.public
		}
		if base == "" {
			base = s.public
		}
		return strings.TrimRight(base, "/") + raw
	}
	return s.public + "/" + raw
}

func (s *Social) encrypt(creds pkgSocial.Credentials) ([]byte, error) {
	encoded, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	if string(encoded) == "{}" {
		return nil, nil
	}
	return s.cipher.Encrypt(string(encoded))
}

func (s *Social) decrypt(blob []byte) (pkgSocial.Credentials, error) {
	if len(blob) == 0 {
		return pkgSocial.Credentials{}, nil
	}
	plain, err := s.cipher.Decrypt(blob)
	if err != nil {
		return pkgSocial.Credentials{}, err
	}
	var creds pkgSocial.Credentials
	if err := json.Unmarshal([]byte(plain), &creds); err != nil {
		return pkgSocial.Credentials{}, err
	}
	return creds, nil
}

func (s *Social) refreshIfNeeded(ctx context.Context, network domain.SocialNetwork, creds pkgSocial.Credentials, dealerID uuid.UUID) (pkgSocial.Credentials, error) {
	if network != domain.NetworkYouTube || creds.RefreshToken == "" || !s.cfg.GoogleReady() {
		return creds, nil
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.GoogleClientID)
	values.Set("client_secret", s.cfg.GoogleClientSecret)
	values.Set("refresh_token", creds.RefreshToken)
	values.Set("grant_type", "refresh_token")
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := pkgSocialDoForm(ctx, "https://oauth2.googleapis.com/token", values, &out); err != nil {
		return creds, fmt.Errorf("%w: %s", pkgSocial.ErrNeedReauth, err.Error())
	}
	if out.Error != "" {
		return creds, fmt.Errorf("%w: %s", pkgSocial.ErrNeedReauth, firstNonEmpty(out.ErrorDesc, out.Error))
	}
	if out.AccessToken == "" {
		return creds, nil
	}
	creds.Token = out.AccessToken
	if blob, encErr := s.encrypt(creds); encErr == nil {
		_ = s.accounts.UpdateCredentials(ctx, dealerID, network, blob)
	}
	return creds, nil
}

// --- OAuth ------------------------------------------------------------------

// OAuthStartURL строит адрес авторизации Meta/Google.
func (s *Social) OAuthStartURL(ctx context.Context, dealerID uuid.UUID, network domain.SocialNetwork) (string, error) {
	if !network.Valid() {
		return "", apierr.Validation(map[string]string{"network": "неизвестная сеть"})
	}
	if network != domain.NetworkInstagram && network != domain.NetworkYouTube {
		return "", apierr.BadRequest("для этой сети OAuth не используется — вставьте ключ")
	}
	if s.rdb == nil {
		return "", apierr.Unavailable("OAuth временно недоступен: нет Redis")
	}

	state, err := security.RandomToken(24)
	if err != nil {
		return "", apierr.Internal(err)
	}
	payload := dealerID.String() + "|" + string(network)
	if err := s.rdb.Set(ctx, oauthStatePref+state, payload, oauthStateTTL).Err(); err != nil {
		return "", apierr.Unavailable("OAuth временно недоступен")
	}

	redirect := s.cfg.OAuthRedirectURI
	switch network {
	case domain.NetworkInstagram:
		if !s.cfg.MetaReady() {
			return "", apierr.BadRequest("площадка ещё не подключила Meta")
		}
		q := url.Values{}
		q.Set("client_id", s.cfg.MetaAppID)
		q.Set("redirect_uri", redirect)
		q.Set("state", state)
		q.Set("response_type", "code")
		q.Set("scope", "instagram_basic,instagram_content_publish,pages_show_list,pages_read_engagement,business_management")
		return "https://www.facebook.com/" + "v21.0" + "/dialog/oauth?" + q.Encode(), nil
	case domain.NetworkYouTube:
		if !s.cfg.GoogleReady() {
			return "", apierr.BadRequest("площадка ещё не подключила Google")
		}
		q := url.Values{}
		q.Set("client_id", s.cfg.GoogleClientID)
		q.Set("redirect_uri", redirect)
		q.Set("state", state)
		q.Set("response_type", "code")
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
		q.Set("scope", "https://www.googleapis.com/auth/youtube.readonly https://www.googleapis.com/auth/youtube.upload")
		return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode(), nil
	default:
		return "", apierr.BadRequest("неподдерживаемая сеть")
	}
}

// OAuthCallback привязывает токен к дилеру из state.
func (s *Social) OAuthCallback(ctx context.Context, state, code, oauthErr string) (string, error) {
	fail := s.public + "/app/channels?oauth=error"
	okURL := func(network domain.SocialNetwork) string {
		return s.public + "/app/channels?connected=" + string(network)
	}
	if oauthErr != "" {
		return fail, nil
	}
	if state == "" || code == "" {
		return fail, nil
	}
	if s.rdb == nil {
		return fail, nil
	}
	raw, err := s.rdb.GetDel(ctx, oauthStatePref+state).Result()
	if err != nil {
		return fail, nil
	}
	dealerID, network, ok := parseOAuthState(raw)
	if !ok {
		return fail, nil
	}

	switch network {
	case domain.NetworkInstagram:
		if err := s.finishInstagram(ctx, dealerID, code); err != nil {
			s.log.Error("OAuth Instagram", slog.String("error", err.Error()))
			return s.public + "/app/channels?oauth=error&network=instagram", nil
		}
	case domain.NetworkYouTube:
		if err := s.finishYouTube(ctx, dealerID, code); err != nil {
			s.log.Error("OAuth YouTube", slog.String("error", err.Error()))
			return s.public + "/app/channels?oauth=error&network=youtube", nil
		}
	default:
		return fail, nil
	}
	return okURL(network), nil
}

func parseOAuthState(raw string) (uuid.UUID, domain.SocialNetwork, bool) {
	dealerRaw, networkRaw, found := strings.Cut(raw, "|")
	if !found {
		return uuid.Nil, "", false
	}
	dealerID, err := uuid.Parse(dealerRaw)
	if err != nil {
		return uuid.Nil, "", false
	}
	network, ok := domain.ParseSocialNetwork(networkRaw)
	return dealerID, network, ok
}

func (s *Social) finishInstagram(ctx context.Context, dealerID uuid.UUID, code string) error {
	tokenURL := "https://graph.facebook.com/v21.0/oauth/access_token"
	q := url.Values{}
	q.Set("client_id", s.cfg.MetaAppID)
	q.Set("client_secret", s.cfg.MetaAppSecret)
	q.Set("redirect_uri", s.cfg.OAuthRedirectURI)
	q.Set("code", code)
	var short struct {
		AccessToken string `json:"access_token"`
	}
	if err := pkgSocialDoJSON(ctx, "GET", tokenURL+"?"+q.Encode(), nil, nil, &short); err != nil {
		return err
	}
	if short.AccessToken == "" {
		return errors.New("Meta не вернула access_token")
	}

	longQ := url.Values{}
	longQ.Set("grant_type", "fb_exchange_token")
	longQ.Set("client_id", s.cfg.MetaAppID)
	longQ.Set("client_secret", s.cfg.MetaAppSecret)
	longQ.Set("fb_exchange_token", short.AccessToken)
	var longLived struct {
		AccessToken string `json:"access_token"`
	}
	_ = pkgSocialDoJSON(ctx, "GET", tokenURL+"?"+longQ.Encode(), nil, nil, &longLived)
	token := firstNonEmpty(longLived.AccessToken, short.AccessToken)

	headers := map[string]string{"Authorization": "Bearer " + token}
	var pages struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := pkgSocialDoJSON(ctx, "GET", "https://graph.facebook.com/v21.0/me/accounts", headers, nil, &pages); err != nil {
		return err
	}
	if len(pages.Data) == 0 {
		return errors.New("нет Facebook Page — Instagram должен быть Professional, привязанный к странице")
	}

	var igUserID, pageID string
	for _, page := range pages.Data {
		var pageInfo struct {
			Instagram struct {
				ID string `json:"id"`
			} `json:"instagram_business_account"`
		}
		raw := "https://graph.facebook.com/v21.0/" + page.ID + "?fields=instagram_business_account"
		if err := pkgSocialDoJSON(ctx, "GET", raw, headers, nil, &pageInfo); err != nil {
			continue
		}
		if pageInfo.Instagram.ID != "" {
			igUserID = pageInfo.Instagram.ID
			pageID = page.ID
			break
		}
	}
	if igUserID == "" {
		return errors.New("у страницы нет Instagram Business / Creator")
	}

	blob, err := s.encrypt(pkgSocial.Credentials{
		Token:    token,
		PageID:   pageID,
		IGUserID: igUserID,
	})
	if err != nil {
		return err
	}
	_, err = s.accounts.Upsert(ctx, store.SocialAccountRow{
		DealerID:    dealerID,
		Network:     domain.NetworkInstagram,
		Credentials: blob,
		ExternalID:  igUserID,
		Status:      domain.SocialConnected,
		AutoPost:    true,
	})
	return err
}

func (s *Social) finishYouTube(ctx context.Context, dealerID uuid.UUID, code string) error {
	values := url.Values{}
	values.Set("client_id", s.cfg.GoogleClientID)
	values.Set("client_secret", s.cfg.GoogleClientSecret)
	values.Set("redirect_uri", s.cfg.OAuthRedirectURI)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := pkgSocialDoForm(ctx, "https://oauth2.googleapis.com/token", values, &tokens); err != nil {
		return err
	}
	if tokens.Error != "" {
		return fmt.Errorf("%s", firstNonEmpty(tokens.ErrorDesc, tokens.Error))
	}
	if tokens.AccessToken == "" {
		return errors.New("Google не вернула access_token")
	}

	external := "youtube"
	adapter := pkgSocial.YouTube{}
	if id, err := adapter.Test(ctx, pkgSocial.Credentials{Token: tokens.AccessToken}); err == nil {
		external = id
	}

	blob, err := s.encrypt(pkgSocial.Credentials{
		Token:        tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	})
	if err != nil {
		return err
	}
	_, err = s.accounts.Upsert(ctx, store.SocialAccountRow{
		DealerID:    dealerID,
		Network:     domain.NetworkYouTube,
		Credentials: blob,
		ExternalID:  external,
		Status:      domain.SocialConnected,
		AutoPost:    true,
	})
	return err
}

// Обёртки, чтобы OAuth в service не импортировал неэкспортированные helpers.
// Реальные doJSON/doForm живут в pkg/social — здесь дублировать нельзя,
// поэтому экспортируем тонкие функции ниже в oauth_http.go.
func pkgSocialDoJSON(ctx context.Context, method, rawURL string, headers map[string]string, body any, dest any) error {
	return pkgSocial.DoJSON(ctx, method, rawURL, headers, body, dest)
}

func pkgSocialDoForm(ctx context.Context, rawURL string, values url.Values, dest any) error {
	return pkgSocial.DoForm(ctx, rawURL, values, dest)
}
