package social

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
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
