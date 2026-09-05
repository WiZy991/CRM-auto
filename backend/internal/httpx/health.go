package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

func notFoundError() error { return apierr.NotFound("Запрошенный ресурс") }
func methodNotAllowedError() error {
	return &apierr.Error{
		Status:  http.StatusMethodNotAllowed,
		Code:    apierr.CodeBadRequest,
		Message: "Метод не поддерживается для этого адреса",
	}
}

// notImplemented — заглушка для маршрутов, объявленных, но ещё не
// реализованных. Возвращает корректный код, а не панику.
func notImplemented(w http.ResponseWriter, r *http.Request) {
	Error(w, r, &apierr.Error{
		Status:  http.StatusNotImplemented,
		Code:    "not_implemented",
		Message: "Этот раздел ещё не реализован",
	})
}

// Pinger — проверяемая зависимость.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler отвечает на проверки состояния.
//
// Разделение на liveness и readiness принципиально: /healthz говорит
// «процесс жив» и не должен зависеть от базы, иначе кратковременная
// недоступность PostgreSQL приведёт к перезапуску контейнеров вместо
// ожидания. /readyz говорит «готов принимать трафик» и проверяет
// зависимости.
type HealthHandler struct {
	version   string
	db        Pinger
	redis     Pinger
	startedAt time.Time
}

func NewHealthHandler(version string, db, redis Pinger) *HealthHandler {
	return &HealthHandler{version: version, db: db, redis: redis, startedAt: time.Now()}
}

// Live — GET /healthz
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        h.version,
		"uptime_seconds": int(time.Since(h.startedAt).Seconds()),
	})
}

// Ready — GET /readyz
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	// Короткий таймаут: проверка готовности не должна сама подвешивать
	// систему мониторинга.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{}
	ready := true

	if h.db != nil {
		if err := h.db.Ping(ctx); err != nil {
			checks["postgres"] = "unavailable"
			ready = false
		} else {
			checks["postgres"] = "ok"
		}
	}

	if h.redis != nil {
		if err := h.redis.Ping(ctx); err != nil {
			// Redis нужен для ограничения частоты и списка отозванных
			// сессий, но приложение переживает его отказ в режиме
			// «пропускать», поэтому это предупреждение, а не отказ.
			checks["redis"] = "degraded"
		} else {
			checks["redis"] = "ok"
		}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	JSON(w, status, map[string]any{
		"status":  state,
		"version": h.version,
		"checks":  checks,
	})
}
