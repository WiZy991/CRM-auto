package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

const (
	idempotencyTTL     = 24 * time.Hour
	idempotencyLockTTL = 45 * time.Second
)

// Idempotency повторяет сохранённый ответ для POST/PUT/PATCH с заголовком
// Idempotency-Key. Без ключа запрос идёт как обычно: клиент, который не
// умеет повторять мутации, не обязан слать заголовок.
func (g *Guard) Idempotency() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if g.Redis == nil || !isIdempotentMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			raw := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !idempotencyKeyOK(raw) {
				Error(w, r, apierr.BadRequest("Некорректный Idempotency-Key"))
				return
			}

			actor := ActorFrom(r.Context())
			scope := ClientIPString(r.Context())
			if !actor.IsZero() {
				scope = actor.UserID.String()
			}
			sum := sha256.Sum256([]byte(raw))
			key := "idem:" + scope + ":" + hex.EncodeToString(sum[:])
			lockKey := key + ":lock"

			ctx := r.Context()
			cached, err := g.Redis.Get(ctx, key).Bytes()
			if err == nil && len(cached) > 0 {
				writeCachedIdempotent(w, cached)
				return
			}
			if err != nil && err != redis.Nil {
				LoggerFrom(ctx).Error("идемпотентность: redis недоступен", "error", err.Error())
				next.ServeHTTP(w, r)
				return
			}

			ok, err := g.Redis.SetNX(ctx, lockKey, "1", idempotencyLockTTL).Result()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if !ok {
				Error(w, r, apierr.Conflict("Повторный запрос ещё обрабатывается"))
				return
			}
			defer func() { _ = g.Redis.Del(context.WithoutCancel(ctx), lockKey).Err() }()

			capture := &idempotentWriter{ResponseWriter: w, buf: bytes.NewBuffer(nil)}
			next.ServeHTTP(capture, r)

			status := capture.status
			if status == 0 {
				status = http.StatusOK
			}
			if status >= 200 && status < 300 {
				payload, _ := json.Marshal(idempotentRecord{
					Status:      status,
					ContentType: w.Header().Get("Content-Type"),
					Body:        capture.buf.Bytes(),
				})
				_ = g.Redis.Set(ctx, key, payload, idempotencyTTL).Err()
			}
			capture.flush()
		})
	}
}

func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func idempotencyKeyOK(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if r > unicode.MaxASCII {
			return false
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

type idempotentRecord struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

func writeCachedIdempotent(w http.ResponseWriter, raw []byte) {
	var rec idempotentRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rec.ContentType != "" {
		w.Header().Set("Content-Type", rec.ContentType)
	}
	w.Header().Set("Idempotent-Replay", "true")
	w.WriteHeader(rec.Status)
	_, _ = w.Write(rec.Body)
}

type idempotentWriter struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
	wrote  bool
}

func (i *idempotentWriter) WriteHeader(status int) {
	i.status = status
}

func (i *idempotentWriter) Write(p []byte) (int, error) {
	return i.buf.Write(p)
}

func (i *idempotentWriter) flush() {
	if i.wrote {
		return
	}
	i.wrote = true
	status := i.status
	if status == 0 {
		status = http.StatusOK
	}
	i.ResponseWriter.WriteHeader(status)
	_, _ = i.ResponseWriter.Write(i.buf.Bytes())
}
