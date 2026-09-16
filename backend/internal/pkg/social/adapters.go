package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	graphVersion = "v21.0"
	vkAPIVersion = "5.199"
)

// Telegram — Bot API. Бот должен быть администратором канала.
type Telegram struct{}

func (Telegram) Test(ctx context.Context, creds Credentials) (string, error) {
	if strings.TrimSpace(creds.Token) == "" {
		return "", fmt.Errorf("укажите токен бота")
	}
	var me struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
			ID       int64  `json:"id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := doJSON(ctx, "GET", "https://api.telegram.org/bot"+creds.Token+"/getMe", nil, nil, &me); err != nil {
		return "", err
	}
	if !me.OK {
		return "", fmt.Errorf("telegram getMe: %s", me.Description)
	}
	chatID := strings.TrimSpace(creds.ChatID)
	if chatID == "" {
		return "@" + me.Result.Username, fmt.Errorf("укажите @канал или chat_id, куда бот будет постить")
	}
	var chat struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	raw := "https://api.telegram.org/bot" + creds.Token + "/getChat?chat_id=" + url.QueryEscape(chatID)
	if err := doJSON(ctx, "GET", raw, nil, nil, &chat); err != nil {
		return "", fmt.Errorf("канал недоступен боту (сделайте бота администратором): %w", err)
	}
	if !chat.OK {
		return "", fmt.Errorf("канал недоступен боту: %s", chat.Description)
	}
	if me.Result.Username != "" {
		return "@" + me.Result.Username, nil
	}
	return strconv.FormatInt(me.Result.ID, 10), nil
}

func (Telegram) Publish(ctx context.Context, creds Credentials, listing Listing) (string, error) {
	chatID := strings.TrimSpace(creds.ChatID)
	if chatID == "" {
		return "", fmt.Errorf("не указан канал Telegram")
	}
	photos := firstN(listing.PhotoURLs, 10)
	if len(photos) == 0 {
		var out struct {
			OK     bool `json:"ok"`
			Result struct {
				MessageID int64 `json:"message_id"`
			} `json:"result"`
			Description string `json:"description"`
		}
		body := map[string]any{"chat_id": chatID, "text": listing.Caption, "disable_web_page_preview": false}
		if err := doJSON(ctx, "POST", "https://api.telegram.org/bot"+creds.Token+"/sendMessage", nil, body, &out); err != nil {
			return "", err
		}
		if !out.OK {
			return "", fmt.Errorf("telegram sendMessage: %s", out.Description)
		}
		return strconv.FormatInt(out.Result.MessageID, 10), nil
	}

	media := make([]map[string]any, 0, len(photos))
	for i, photo := range photos {
		item := map[string]any{"type": "photo", "media": photo}
		if i == 0 {
			item["caption"] = listing.Caption
		}
		media = append(media, item)
	}
	var out struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	body := map[string]any{"chat_id": chatID, "media": media}
	if err := doJSON(ctx, "POST", "https://api.telegram.org/bot"+creds.Token+"/sendMediaGroup", nil, body, &out); err != nil {
		return "", err
	}
	if !out.OK {
		return "", fmt.Errorf("telegram sendMediaGroup: %s", out.Description)
	}
	return "ok", nil
}

// VK — токен сообщества и отрицательный owner_id группы.
type VK struct{}

func (VK) Test(ctx context.Context, creds Credentials) (string, error) {
	if strings.TrimSpace(creds.Token) == "" {
		return "", fmt.Errorf("укажите токен сообщества ВКонтакте")
	}
	owner := strings.TrimSpace(creds.OwnerID)
	if owner == "" {
		return "", fmt.Errorf("укажите owner_id группы (отрицательное число, например -123456)")
	}
	groupID := strings.TrimPrefix(owner, "-")
	rawURL := "https://api.vk.com/method/groups.getById?access_token=" + url.QueryEscape(creds.Token) +
		"&v=" + vkAPIVersion + "&group_id=" + url.QueryEscape(groupID)
	var raw struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Code    int    `json:"error_code"`
			Message string `json:"error_msg"`
		} `json:"error"`
	}
	if err := doJSON(ctx, "GET", rawURL, nil, nil, &raw); err != nil {
		return "", err
	}
	if raw.Error != nil {
		if raw.Error.Code == 5 || raw.Error.Code == 15 {
			return "", fmt.Errorf("%w: %s", ErrNeedReauth, raw.Error.Message)
		}
		return "", fmt.Errorf("vk: %s", raw.Error.Message)
	}

	type vkGroup struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Screen string `json:"screen_name"`
	}
	var groups []vkGroup
	if err := json.Unmarshal(raw.Response, &groups); err != nil {
		var wrapped struct {
			Groups []vkGroup `json:"groups"`
		}
		if wrapErr := json.Unmarshal(raw.Response, &wrapped); wrapErr != nil {
			return "", fmt.Errorf("vk: не удалось разобрать ответ groups.getById")
		}
		groups = wrapped.Groups
	}
	if len(groups) == 0 {
		return "", fmt.Errorf("группа не найдена — проверьте owner_id и что токен именно сообщества")
	}
	item := groups[0]
	if item.Screen != "" {
		return item.Screen, nil
	}
	return item.Name, nil
}

func (VK) Publish(ctx context.Context, creds Credentials, listing Listing) (string, error) {
	owner := strings.TrimSpace(creds.OwnerID)
	values := url.Values{}
	values.Set("access_token", creds.Token)
	values.Set("v", vkAPIVersion)
	values.Set("owner_id", owner)
	values.Set("from_group", "1")
	values.Set("message", listing.Caption)
	var out struct {
		Response struct {
			PostID int `json:"post_id"`
		} `json:"response"`
		Error *struct {
			Code    int    `json:"error_code"`
			Message string `json:"error_msg"`
		} `json:"error"`
	}
	if err := doForm(ctx, "https://api.vk.com/method/wall.post", values, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		if out.Error.Code == 5 || out.Error.Code == 15 {
			return "", fmt.Errorf("%w: %s", ErrNeedReauth, out.Error.Message)
		}
		return "", fmt.Errorf("vk wall.post: %s", out.Error.Message)
	}
	if out.Response.PostID == 0 {
		return "", fmt.Errorf("vk wall.post не вернул post_id")
	}
	return strconv.Itoa(out.Response.PostID), nil
}

// WhatsApp — Cloud API. Автопост лота — сообщение в указанный чат/Channel, не стена.
type WhatsApp struct{}

func (WhatsApp) Test(ctx context.Context, creds Credentials) (string, error) {
	if strings.TrimSpace(creds.Token) == "" || strings.TrimSpace(creds.PhoneNumberID) == "" {
		return "", fmt.Errorf("укажите постоянный токен и phone_number_id из Meta Business")
	}
	raw := fmt.Sprintf("https://graph.facebook.com/%s/%s?fields=id,display_phone_number,verified_name", graphVersion, url.PathEscape(creds.PhoneNumberID))
	var out struct {
		ID      string    `json:"id"`
		Display string    `json:"display_phone_number"`
		Name    string    `json:"verified_name"`
		Error   *graphErr `json:"error"`
	}
	headers := map[string]string{"Authorization": "Bearer " + creds.Token}
	if err := doJSON(ctx, "GET", raw, headers, nil, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", out.Error.asError()
	}
	if out.Display != "" {
		return out.Display, nil
	}
	return out.ID, nil
}

func (WhatsApp) Publish(ctx context.Context, creds Credentials, listing Listing) (string, error) {
	to := strings.TrimSpace(creds.Destination)
	if to == "" {
		return "", fmt.Errorf("%w: не указан чат или Channel id — лоты в WhatsApp не уходят", ErrSkipped)
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]any{"body": listing.Caption, "preview_url": true},
	}
	raw := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", graphVersion, url.PathEscape(creds.PhoneNumberID))
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Error *graphErr `json:"error"`
	}
	headers := map[string]string{"Authorization": "Bearer " + creds.Token}
	if err := doJSON(ctx, "POST", raw, headers, body, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", out.Error.asError()
	}
	if len(out.Messages) == 0 {
		return "", fmt.Errorf("whatsapp не вернул id сообщения")
	}
	return out.Messages[0].ID, nil
}

type graphErr struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

func (e *graphErr) asError() error {
	if e == nil {
		return nil
	}
	if e.Code == 190 || e.Code == 102 || e.Code == 10 {
		return fmt.Errorf("%w: %s", ErrNeedReauth, e.Message)
	}
	return fmt.Errorf("graph: %s", e.Message)
}

// Instagram — Graph API, карусель фото публичных URL.
type Instagram struct{}

func (Instagram) Test(ctx context.Context, creds Credentials) (string, error) {
	if strings.TrimSpace(creds.Token) == "" {
		return "", fmt.Errorf("Instagram не подключён")
	}
	userID := strings.TrimSpace(creds.IGUserID)
	if userID == "" {
		userID = "me"
	}
	raw := fmt.Sprintf("https://graph.facebook.com/%s/%s?fields=id,username", graphVersion, url.PathEscape(userID))
	var out struct {
		ID       string    `json:"id"`
		Username string    `json:"username"`
		Error    *graphErr `json:"error"`
	}
	headers := map[string]string{"Authorization": "Bearer " + creds.Token}
	if err := doJSON(ctx, "GET", raw, headers, nil, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", out.Error.asError()
	}
	if out.Username != "" {
		return out.Username, nil
	}
	return out.ID, nil
}

func (i Instagram) Publish(ctx context.Context, creds Credentials, listing Listing) (string, error) {
	igUser := strings.TrimSpace(creds.IGUserID)
	if igUser == "" {
		return "", fmt.Errorf("нет ig_user_id — переподключите Instagram")
	}
	photos := firstN(listing.PhotoURLs, 10)
	if len(photos) == 0 {
		return "", fmt.Errorf("%w: нет публичных фото для карусели Instagram", ErrSkipped)
	}
	headers := map[string]string{"Authorization": "Bearer " + creds.Token}

	if len(photos) == 1 {
		creation, err := i.createMedia(ctx, igUser, headers, map[string]any{
			"image_url": photos[0],
			"caption":   listing.Caption,
		})
		if err != nil {
			return "", err
		}
		return i.publishMedia(ctx, igUser, headers, creation)
	}

	children := make([]string, 0, len(photos))
	for _, photo := range photos {
		id, err := i.createMedia(ctx, igUser, headers, map[string]any{
			"image_url":        photo,
			"is_carousel_item": true,
		})
		if err != nil {
			return "", err
		}
		children = append(children, id)
	}
	carousel, err := i.createMedia(ctx, igUser, headers, map[string]any{
		"media_type": "CAROUSEL",
		"children":   strings.Join(children, ","),
		"caption":    listing.Caption,
	})
	if err != nil {
		return "", err
	}
	return i.publishMedia(ctx, igUser, headers, carousel)
}

func (Instagram) createMedia(ctx context.Context, igUser string, headers map[string]string, fields map[string]any) (string, error) {
	raw := fmt.Sprintf("https://graph.facebook.com/%s/%s/media", graphVersion, url.PathEscape(igUser))
	var out struct {
		ID    string    `json:"id"`
		Error *graphErr `json:"error"`
	}
	if err := doJSON(ctx, "POST", raw, headers, fields, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", out.Error.asError()
	}
	if out.ID == "" {
		return "", fmt.Errorf("instagram не вернул id контейнера")
	}
	return out.ID, nil
}

func (Instagram) publishMedia(ctx context.Context, igUser string, headers map[string]string, creationID string) (string, error) {
	raw := fmt.Sprintf("https://graph.facebook.com/%s/%s/media_publish", graphVersion, url.PathEscape(igUser))
	var out struct {
		ID    string    `json:"id"`
		Error *graphErr `json:"error"`
	}
	if err := doJSON(ctx, "POST", raw, headers, map[string]any{"creation_id": creationID}, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", out.Error.asError()
	}
	return out.ID, nil
}

// YouTube — Data API. В v1 только проверка подключения: исходного ролика в лоте нет.
type YouTube struct{}

func (YouTube) Test(ctx context.Context, creds Credentials) (string, error) {
	if strings.TrimSpace(creds.Token) == "" {
		return "", fmt.Errorf("YouTube не подключён")
	}
	raw := "https://www.googleapis.com/youtube/v3/channels?part=id,snippet&mine=true"
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	headers := map[string]string{"Authorization": "Bearer " + creds.Token}
	if err := doJSON(ctx, "GET", raw, headers, nil, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		if out.Error.Code == 401 || out.Error.Code == 403 {
			return "", fmt.Errorf("%w: %s", ErrNeedReauth, out.Error.Message)
		}
		return "", fmt.Errorf("youtube: %s", out.Error.Message)
	}
	if len(out.Items) == 0 {
		return "", fmt.Errorf("у аккаунта нет YouTube-канала")
	}
	if out.Items[0].Snippet.Title != "" {
		return out.Items[0].Snippet.Title, nil
	}
	return out.Items[0].ID, nil
}

func (YouTube) Publish(_ context.Context, _ Credentials, listing Listing) (string, error) {
	if !listing.HasVideo {
		return "", fmt.Errorf("%w: ролик можно будет слать, когда в лоте появится видео", ErrSkipped)
	}
	return "", fmt.Errorf("%w: загрузка ролика на YouTube в этой версии не включена", ErrSkipped)
}

// RuTube — ключ по документации платформы; партнёрский API нестабилен.
type RuTube struct{}

func (RuTube) Test(ctx context.Context, creds Credentials) (string, error) {
	token := strings.TrimSpace(creds.Token)
	if token == "" {
		token = strings.TrimSpace(creds.APIKey)
	}
	if token == "" {
		return "", fmt.Errorf("укажите API-ключ или токен RuTube")
	}
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"X-API-KEY":     token,
	}
	var out map[string]any
	// Профиль — самый безопасный пробный GET: он не создаёт публикацию.
	if err := doJSON(ctx, "GET", "https://rutube.ru/api/accounts/profile/", headers, nil, &out); err != nil {
		return "", fmt.Errorf("RuTube не подтвердил ключ (партнёрский API часто меняется): %w", err)
	}
	for _, key := range []string{"name", "username", "id"} {
		if v, ok := out[key]; ok {
			return fmt.Sprint(v), nil
		}
	}
	return "ok", nil
}

func (RuTube) Publish(_ context.Context, _ Credentials, listing Listing) (string, error) {
	if !listing.HasVideo {
		return "", fmt.Errorf("%w: у лота нет видео для RuTube", ErrSkipped)
	}
	return "", fmt.Errorf("%w: загрузка на RuTube в этой версии не включена", ErrSkipped)
}

// Avito — каркас до partner credentials площадки. Без фейковых постов.
type Avito struct {
	PartnerReady bool
}

func (a Avito) Test(_ context.Context, creds Credentials) (string, error) {
	clientID := strings.TrimSpace(creds.ClientID)
	secret := strings.TrimSpace(firstCred(creds.ClientSecret, creds.Token))
	profile := strings.TrimSpace(firstCred(creds.ProfileID, creds.OwnerID))
	if clientID == "" && secret == "" {
		return "", fmt.Errorf("укажите client_id и client_secret (или user token) Авито")
	}
	if clientID == "" {
		return "", fmt.Errorf("укажите client_id приложения Авито")
	}
	if secret == "" {
		return "", fmt.Errorf("укажите client_secret или user token Авито")
	}
	if !a.PartnerReady {
		if profile != "" {
			return profile, nil
		}
		return "avito-keys-ok", nil
	}
	// Реальный вызов API появится после модерации кабинета разработчика.
	if profile != "" {
		return profile, nil
	}
	return "avito-keys-ok", nil
}

func (a Avito) Publish(_ context.Context, _ Credentials, _ Listing) (string, error) {
	return "", fmt.Errorf("%w: автопост в Авито появится после partner credentials площадки", ErrNeedsPartner)
}

// Drom — каркас до partner credentials. Без фейковых постов.
type Drom struct {
	PartnerReady bool
}

func (d Drom) Test(_ context.Context, creds Credentials) (string, error) {
	token := strings.TrimSpace(firstCred(creds.Token, creds.APIKey, creds.ClientSecret))
	profile := strings.TrimSpace(firstCred(creds.ProfileID, creds.OwnerID, creds.ClientID))
	if token == "" {
		return "", fmt.Errorf("укажите user token или api key Дрома")
	}
	if profile == "" {
		return "", fmt.Errorf("укажите profile_id / id кабинета Дрома")
	}
	if !d.PartnerReady {
		return profile, nil
	}
	return profile, nil
}

func (d Drom) Publish(_ context.Context, _ Credentials, _ Listing) (string, error) {
	return "", fmt.Errorf("%w: автопост на Дром появится после partner credentials площадки", ErrNeedsPartner)
}

// WeChat — 微信公众号 (Official Account / Service Account).
// Дилер хранит AppID + AppSecret своего кабинета mp.weixin.qq.com.
// Проверка связи: реальный обмен на access_token.
// Автопост: черновик в draft box (не freepublish — квота и модерация у дилера).
type WeChat struct{}

func (w WeChat) Test(ctx context.Context, creds Credentials) (string, error) {
	appID, secret, err := wechatCreds(creds)
	if err != nil {
		return "", err
	}
	token, err := w.accessToken(ctx, appID, secret)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("WeChat не вернул access_token")
	}
	return appID, nil
}

func (w WeChat) Publish(ctx context.Context, creds Credentials, listing Listing) (string, error) {
	appID, secret, err := wechatCreds(creds)
	if err != nil {
		return "", err
	}
	token, err := w.accessToken(ctx, appID, secret)
	if err != nil {
		return "", err
	}

	photos := firstN(listing.PhotoURLs, 1)
	if len(photos) == 0 {
		return "", fmt.Errorf("%w: для черновика WeChat нужна хотя бы одна фотография лота (обложка)", ErrSkipped)
	}

	thumbID, err := w.uploadImage(ctx, token, photos[0])
	if err != nil {
		return "", fmt.Errorf("загрузка обложки в WeChat: %w", err)
	}

	title := strings.TrimSpace(listing.Title)
	if title == "" {
		title = "Автомобиль"
	}
	if utf8.RuneCountInString(title) > 32 {
		runes := []rune(title)
		title = string(runes[:32])
	}

	content := "<p>" + htmlEscape(listing.Caption) + "</p>"
	if listing.URL != "" {
		content += `<p><a href="` + htmlEscape(listing.URL) + `">` + htmlEscape(listing.URL) + `</a></p>`
	}

	body := map[string]any{
		"articles": []map[string]any{{
			"title":          title,
			"thumb_media_id": thumbID,
			"author":         "GoImport",
			"digest":         truncateRunes(listing.Caption, 54),
			"content":        content,
			"content_source_url": listing.URL,
			"need_open_comment":  0,
		}},
	}
	var out struct {
		MediaID string `json:"media_id"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	raw := "https://api.weixin.qq.com/cgi-bin/draft/add?access_token=" + url.QueryEscape(token)
	if err := doJSON(ctx, "POST", raw, nil, body, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 {
		if out.ErrCode == 40001 || out.ErrCode == 42001 {
			return "", fmt.Errorf("%w: %s", ErrNeedReauth, out.ErrMsg)
		}
		return "", fmt.Errorf("wechat draft/add: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.MediaID == "" {
		return "", fmt.Errorf("wechat draft/add не вернул media_id")
	}
	// Не вызываем freepublish: квота жёсткая, публикация — вручную из черновиков MP.
	return out.MediaID, nil
}

func wechatCreds(creds Credentials) (appID, secret string, err error) {
	appID = strings.TrimSpace(firstCred(creds.ClientID, creds.OwnerID))
	secret = strings.TrimSpace(firstCred(creds.ClientSecret, creds.Token, creds.APIKey))
	if appID == "" {
		return "", "", fmt.Errorf("укажите AppID WeChat (公众号)")
	}
	if secret == "" {
		return "", "", fmt.Errorf("укажите AppSecret WeChat")
	}
	return appID, secret, nil
}

func (WeChat) accessToken(ctx context.Context, appID, secret string) (string, error) {
	raw := "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=" +
		url.QueryEscape(appID) + "&secret=" + url.QueryEscape(secret)
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := doJSON(ctx, "GET", raw, nil, nil, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 {
		if out.ErrCode == 40125 || out.ErrCode == 40013 || out.ErrCode == 40164 {
			return "", fmt.Errorf("%w: %s (проверьте AppSecret и IP whitelist в mp.weixin.qq.com)", ErrNeedReauth, out.ErrMsg)
		}
		return "", fmt.Errorf("wechat token: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("wechat token: пустой access_token")
	}
	return out.AccessToken, nil
}

func (WeChat) uploadImage(ctx context.Context, accessToken, imageURL string) (string, error) {
	reqImg, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	respImg, err := httpClient.Do(reqImg)
	if err != nil {
		return "", fmt.Errorf("скачивание фото лота: %w", err)
	}
	defer respImg.Body.Close()
	if respImg.StatusCode >= 400 {
		return "", fmt.Errorf("фото лота недоступно (%d)", respImg.StatusCode)
	}
	imgBytes, err := io.ReadAll(io.LimitReader(respImg.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if len(imgBytes) < 32 {
		return "", fmt.Errorf("фото лота слишком маленькое")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("media", "cover.jpg")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(imgBytes); err != nil {
		return "", err
	}
	_ = writer.Close()

	raw := "https://api.weixin.qq.com/cgi-bin/material/add_material?access_token=" +
		url.QueryEscape(accessToken) + "&type=image"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		MediaID string `json:"media_id"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", fmt.Errorf("разбор ответа WeChat media: %w", err)
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("wechat media: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.MediaID == "" {
		return "", fmt.Errorf("wechat media: пустой media_id")
	}
	return out.MediaID, nil
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(s)
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func firstCred(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
