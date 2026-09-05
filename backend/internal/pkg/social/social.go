package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrNeedReauth — токен отозван или просрочен; дилер обновляет ключ сам.
var ErrNeedReauth = errors.New("нужно обновить ключ канала")

// ErrSkipped — публиковать нечего или канал не лента (нет видео, нет чата).
var ErrSkipped = errors.New("публикация пропущена")

// Credentials — расшифрованные поля ключа. Пустые значения не сериализуются
// в ответ API; наружу уходит только маска.
type Credentials struct {
	Token             string `json:"token,omitempty"`
	RefreshToken      string `json:"refresh_token,omitempty"`
	ChatID            string `json:"chat_id,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	PhoneNumberID     string `json:"phone_number_id,omitempty"`
	BusinessAccountID string `json:"business_account_id,omitempty"`
	Destination       string `json:"destination,omitempty"`
	APIKey            string `json:"api_key,omitempty"`
	PageID            string `json:"page_id,omitempty"`
	IGUserID          string `json:"ig_user_id,omitempty"`
}

// SecretMask показывает хвост токена, чтобы дилер узнал «тот ли ключ».
func SecretMask(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return "••••"
	}
	return "…" + string(runes[len(runes)-4:])
}

// Listing — данные лота для поста.
type Listing struct {
	Title     string
	Caption   string
	URL       string
	PhotoURLs []string
	HasVideo  bool
}

// BuildCaption собирает текст поста из карточки лота.
func BuildCaption(brand, model string, year int, priceLabel, pageURL string) string {
	parts := []string{strings.TrimSpace(fmt.Sprintf("%s %s %d", brand, model, year))}
	if priceLabel != "" {
		parts = append(parts, priceLabel)
	}
	if pageURL != "" {
		parts = append(parts, pageURL)
	}
	text := strings.Join(parts, "\n")
	if utf8.RuneCountInString(text) > 3500 {
		runes := []rune(text)
		text = string(runes[:3500])
	}
	return text
}

// Adapter проверяет связь и публикует лот.
type Adapter interface {
	Test(ctx context.Context, creds Credentials) (externalID string, err error)
	Publish(ctx context.Context, creds Credentials, listing Listing) (postID string, err error)
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// DoJSON выполняет JSON-запрос к внешней сети. Нужен OAuth-обмену в сервисе.
func DoJSON(ctx context.Context, method, rawURL string, headers map[string]string, body any, dest any) error {
	return doJSON(ctx, method, rawURL, headers, body, dest)
}

// DoForm выполняет form-urlencoded запрос (обмен кода Google/VK).
func DoForm(ctx context.Context, rawURL string, values url.Values, dest any) error {
	return doForm(ctx, rawURL, values, dest)
}

func doJSON(ctx context.Context, method, rawURL string, headers map[string]string, body any, dest any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("сериализация запроса: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("запрос к сети: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("чтение ответа сети: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %s", ErrNeedReauth, truncateBody(payload))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ответ %d: %s", resp.StatusCode, truncateBody(payload))
	}
	if dest == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, dest); err != nil {
		return fmt.Errorf("разбор ответа сети: %w", err)
	}
	return nil
}

func doForm(ctx context.Context, rawURL string, values url.Values, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("запрос к сети: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("чтение ответа сети: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %s", ErrNeedReauth, truncateBody(payload))
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ответ %d: %s", resp.StatusCode, truncateBody(payload))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(payload, dest)
}

func truncateBody(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 280 {
		return text[:280]
	}
	if text == "" {
		return "пустой ответ"
	}
	return text
}

func firstN(urls []string, n int) []string {
	if len(urls) <= n {
		return urls
	}
	return urls[:n]
}
