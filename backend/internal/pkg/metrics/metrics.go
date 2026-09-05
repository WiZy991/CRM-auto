// Package metrics отдаёт метрики в текстовом формате Prometheus.
//
// Реализация своя, без клиентской библиотеки: нужен фиксированный набор
// счётчиков и одна гистограмма, а официальный клиент тянет за собой protobuf
// и десяток транзитивных зависимостей. Формат экспозиции текстовый и
// стабильный, поэтому написать его напрямую дешевле, чем поддерживать
// лишние зависимости.
//
// Метрики нужны не для красоты: без доли 4xx/5xx, задержек и счётчика
// срабатываний rate-limit атаку невозможно отличить от роста популярности.
package metrics

import (
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry хранит все метрики приложения.
type Registry struct {
	mu sync.RWMutex

	requestsTotal   map[string]*atomic.Int64
	requestDuration *histogram

	rateLimitHits   map[string]*atomic.Int64
	bansTotal       atomic.Int64
	suspiciousTotal map[string]*atomic.Int64

	authEvents map[string]*atomic.Int64

	dbPoolStats func() map[string]int64

	buildInfo map[string]string
	startedAt time.Time
}

// Границы бакетов подобраны под ожидаемые времена ответа API: от быстрых
// чтений из кеша до тяжёлых отчётов воронки.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func New(version string) *Registry {
	return &Registry{
		requestsTotal:   make(map[string]*atomic.Int64),
		requestDuration: newHistogram(durationBuckets),
		rateLimitHits:   make(map[string]*atomic.Int64),
		suspiciousTotal: make(map[string]*atomic.Int64),
		authEvents:      make(map[string]*atomic.Int64),
		buildInfo:       map[string]string{"version": version, "go_version": runtimeVersion()},
		startedAt:       time.Now(),
	}
}

// SetDBPoolSource подключает статистику пула соединений.
func (r *Registry) SetDBPoolSource(fn func() map[string]int64) {
	r.mu.Lock()
	r.dbPoolStats = fn
	r.mu.Unlock()
}

func (r *Registry) counter(store map[string]*atomic.Int64, key string) *atomic.Int64 {
	r.mu.RLock()
	c, ok := store[key]
	r.mu.RUnlock()
	if ok {
		return c
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := store[key]; ok {
		return c
	}
	c = &atomic.Int64{}
	store[key] = c
	return c
}

// ObserveRequest фиксирует завершённый HTTP-запрос.
//
// В качестве метки берётся шаблон маршрута (/api/v1/cars/{id}), а не сырой
// путь: иначе каждый идентификатор создавал бы отдельную серию и метрики
// разрослись бы до неработоспособного состояния.
func (r *Registry) ObserveRequest(method, routePattern string, status int, duration time.Duration) {
	if routePattern == "" {
		routePattern = "unmatched"
	}
	key := method + "|" + routePattern + "|" + strconv.Itoa(status)
	r.counter(r.requestsTotal, key).Add(1)
	r.requestDuration.observe(duration.Seconds())
}

// ObserveRateLimit фиксирует срабатывание ограничения частоты.
func (r *Registry) ObserveRateLimit(rule string) {
	r.counter(r.rateLimitHits, rule).Add(1)
}

// ObserveSuspicious фиксирует подозрительное событие.
func (r *Registry) ObserveSuspicious(kind string) {
	r.counter(r.suspiciousTotal, kind).Add(1)
}

// ObserveBan фиксирует блокировку адреса.
func (r *Registry) ObserveBan() { r.bansTotal.Add(1) }

// ObserveAuthEvent фиксирует событие аутентификации: login_success,
// login_failed, refresh_reuse, register и так далее.
func (r *Registry) ObserveAuthEvent(event string) {
	r.counter(r.authEvents, event).Add(1)
}

// Handler отдаёт метрики. Доступ к нему закрыт на уровне nginx и
// middleware — экспозиция раскрывает внутреннее устройство сервиса.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		var b strings.Builder
		b.Grow(4096)

		r.mu.RLock()
		buildInfo := maps.Clone(r.buildInfo)
		requests := snapshot(r.requestsTotal)
		limits := snapshot(r.rateLimitHits)
		suspicious := snapshot(r.suspiciousTotal)
		authEvents := snapshot(r.authEvents)
		poolFn := r.dbPoolStats
		r.mu.RUnlock()

		writeHelp(&b, "autoimport_build_info", "Версия сборки сервиса.", "gauge")
		labels := make([]string, 0, len(buildInfo))
		for k, v := range buildInfo {
			labels = append(labels, fmt.Sprintf("%s=%q", k, v))
		}
		sort.Strings(labels)
		fmt.Fprintf(&b, "autoimport_build_info{%s} 1\n", strings.Join(labels, ","))

		writeHelp(&b, "autoimport_uptime_seconds", "Время работы процесса в секундах.", "gauge")
		fmt.Fprintf(&b, "autoimport_uptime_seconds %.0f\n", time.Since(r.startedAt).Seconds())

		writeHelp(&b, "autoimport_http_requests_total", "Число обработанных HTTP-запросов.", "counter")
		for _, key := range sortedKeys(requests) {
			parts := strings.SplitN(key, "|", 3)
			if len(parts) != 3 {
				continue
			}
			fmt.Fprintf(&b, "autoimport_http_requests_total{method=%q,route=%q,status=%q} %d\n",
				parts[0], parts[1], parts[2], requests[key])
		}

		r.requestDuration.write(&b, "autoimport_http_request_duration_seconds",
			"Распределение времени обработки HTTP-запросов.")

		writeHelp(&b, "autoimport_ratelimit_hits_total", "Срабатывания ограничения частоты запросов.", "counter")
		for _, key := range sortedKeys(limits) {
			fmt.Fprintf(&b, "autoimport_ratelimit_hits_total{rule=%q} %d\n", key, limits[key])
		}

		writeHelp(&b, "autoimport_suspicious_events_total", "Подозрительные запросы по типам.", "counter")
		for _, key := range sortedKeys(suspicious) {
			fmt.Fprintf(&b, "autoimport_suspicious_events_total{kind=%q} %d\n", key, suspicious[key])
		}

		writeHelp(&b, "autoimport_ip_bans_total", "Число блокировок адресов.", "counter")
		fmt.Fprintf(&b, "autoimport_ip_bans_total %d\n", r.bansTotal.Load())

		writeHelp(&b, "autoimport_auth_events_total", "События аутентификации по типам.", "counter")
		for _, key := range sortedKeys(authEvents) {
			fmt.Fprintf(&b, "autoimport_auth_events_total{event=%q} %d\n", key, authEvents[key])
		}

		if poolFn != nil {
			writeHelp(&b, "autoimport_db_pool", "Состояние пула соединений с PostgreSQL.", "gauge")
			stats := poolFn()
			for _, key := range sortedKeys(stats) {
				fmt.Fprintf(&b, "autoimport_db_pool{stat=%q} %d\n", key, stats[key])
			}
		}

		_, _ = w.Write([]byte(b.String()))
	}
}

func writeHelp(b *strings.Builder, name, help, typ string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func snapshot(store map[string]*atomic.Int64) map[string]int64 {
	out := make(map[string]int64, len(store))
	for k, v := range store {
		out[k] = v.Load()
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Гистограмма -----------------------------------------------------------

type histogram struct {
	bounds []float64
	counts []atomic.Int64
	sum    atomic.Uint64 // хранится как биты float64
	total  atomic.Int64
}

func newHistogram(bounds []float64) *histogram {
	return &histogram{
		bounds: bounds,
		counts: make([]atomic.Int64, len(bounds)+1),
	}
}

func (h *histogram) observe(v float64) {
	idx := sort.SearchFloat64s(h.bounds, v)
	h.counts[idx].Add(1)
	h.total.Add(1)

	// Сумма накапливается через CAS: атомарного сложения для float в Go нет.
	for {
		old := h.sum.Load()
		updated := float64bits(bitsFloat64(old) + v)
		if h.sum.CompareAndSwap(old, updated) {
			return
		}
	}
}

func (h *histogram) write(b *strings.Builder, name, help string) {
	writeHelp(b, name, help, "histogram")

	var cumulative int64
	for i, bound := range h.bounds {
		cumulative += h.counts[i].Load()
		fmt.Fprintf(b, "%s_bucket{le=\"%g\"} %d\n", name, bound, cumulative)
	}
	cumulative += h.counts[len(h.bounds)].Load()
	fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %d\n", name, cumulative)
	fmt.Fprintf(b, "%s_sum %g\n", name, bitsFloat64(h.sum.Load()))
	fmt.Fprintf(b, "%s_count %d\n", name, h.total.Load())
}
