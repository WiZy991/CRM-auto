// Package ratelimit реализует ограничение частоты запросов и адаптивную
// блокировку адресов на Redis.
//
// Почему Redis, а не память процесса: лимит должен быть общим для всех
// экземпляров API. Счётчик в памяти означает, что при трёх процессах
// атакующий получает тройной бюджет, а после перезапуска — новый.
//
// Алгоритм — token bucket, выполняемый одним Lua-скриптом на стороне Redis.
// Это даёт атомарность (между чтением и записью никто не встрянет), один
// сетевой вызов на проверку и естественную поддержку всплесков: клиент может
// израсходовать накопленные токены сразу, но средняя скорость останется
// в пределах лимита.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript расходует один токен из ведра.
//
// KEYS[1] — ключ ведра
// ARGV[1] — ёмкость (максимум накопленных токенов)
// ARGV[2] — скорость восстановления, токенов в секунду
// ARGV[3] — текущее время в миллисекундах
// ARGV[4] — TTL ключа в секундах
//
// Возвращает: {разрешено (1/0), остаток токенов, ждать миллисекунд}
var tokenBucketScript = redis.NewScript(`
local capacity   = tonumber(ARGV[1])
local rate       = tonumber(ARGV[2])
local now_ms     = tonumber(ARGV[3])
local ttl        = tonumber(ARGV[4])

local bucket = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(bucket[1])
local ts     = tonumber(bucket[2])

if tokens == nil then
    tokens = capacity
    ts = now_ms
end

-- Восстановление токенов за прошедшее время.
local elapsed = math.max(0, now_ms - ts) / 1000.0
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
local retry_ms = 0

if tokens >= 1 then
    tokens = tokens - 1
    allowed = 1
else
    -- Сколько ждать до появления одного токена.
    retry_ms = math.ceil((1 - tokens) / rate * 1000)
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'ts', now_ms)
redis.call('EXPIRE', KEYS[1], ttl)

return {allowed, math.floor(tokens), retry_ms}
`)

// Rule описывает лимит: сколько запросов за какой период.
type Rule struct {
	// Name используется в ключе Redis и в метриках.
	Name string
	// Limit — число запросов за Window.
	Limit int
	// Window — период, за который действует Limit.
	Window time.Duration
	// Burst — максимальный мгновенный всплеск. Ноль означает Limit.
	Burst int
}

func (r Rule) capacity() float64 {
	if r.Burst > 0 {
		return float64(r.Burst)
	}
	return float64(r.Limit)
}

func (r Rule) ratePerSecond() float64 {
	if r.Window <= 0 || r.Limit <= 0 {
		return 0
	}
	return float64(r.Limit) / r.Window.Seconds()
}

func (r Rule) ttlSeconds() int {
	// Ключ живёт заметно дольше окна: иначе клиент сбрасывал бы лимит,
	// просто выждав окно без запросов, и всплески считались бы заново.
	ttl := int(math.Ceil(r.Window.Seconds() * 2))
	if ttl < 60 {
		ttl = 60
	}
	return ttl
}

// Decision — результат проверки лимита.
type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	Rule       string
}

// Limiter проверяет лимиты и ведёт список заблокированных адресов.
type Limiter struct {
	rdb    redis.UniversalClient
	prefix string

	// failOpen определяет поведение при недоступности Redis.
	//
	// Выбран режим «пропускать»: иначе сбой Redis превращается в полную
	// недоступность платформы, то есть в отказ обслуживания собственными
	// руками. Событие пишется в лог с уровнем error и попадает в метрики,
	// а на рубеже nginx лимиты продолжают работать независимо от Redis.
	failOpen bool
}

func New(rdb redis.UniversalClient, prefix string) *Limiter {
	if prefix == "" {
		prefix = "rl"
	}
	return &Limiter{rdb: rdb, prefix: prefix, failOpen: true}
}

// ErrLimiterUnavailable возвращается, когда Redis недоступен и режим
// failOpen отключён.
var ErrLimiterUnavailable = errors.New("сервис ограничения частоты недоступен")

// Allow расходует один токен для пары (правило, идентификатор клиента).
func (l *Limiter) Allow(ctx context.Context, rule Rule, identity string) (Decision, error) {
	if rule.Limit <= 0 || rule.Window <= 0 {
		return Decision{Allowed: true, Rule: rule.Name}, nil
	}

	key := fmt.Sprintf("%s:%s:%s", l.prefix, rule.Name, identity)
	now := time.Now().UnixMilli()

	res, err := tokenBucketScript.Run(ctx, l.rdb, []string{key},
		rule.capacity(), rule.ratePerSecond(), now, rule.ttlSeconds()).Slice()
	if err != nil {
		if l.failOpen {
			return Decision{Allowed: true, Rule: rule.Name}, fmt.Errorf("redis недоступен: %w", err)
		}
		return Decision{Allowed: false, Rule: rule.Name}, ErrLimiterUnavailable
	}

	decision := Decision{Rule: rule.Name}
	if len(res) >= 3 {
		allowed, _ := res[0].(int64)
		remaining, _ := res[1].(int64)
		retryMS, _ := res[2].(int64)

		decision.Allowed = allowed == 1
		decision.Remaining = int(remaining)
		decision.RetryAfter = time.Duration(retryMS) * time.Millisecond
	}

	// Клиенту всегда сообщается хотя бы секунда ожидания: Retry-After: 0
	// провоцирует немедленный повтор и лишнюю нагрузку.
	if !decision.Allowed && decision.RetryAfter < time.Second {
		decision.RetryAfter = time.Second
	}
	return decision, nil
}

// Reset снимает лимит. Используется после успешного входа: правильный
// пароль обнуляет бюджет неудачных попыток для этого адреса.
func (l *Limiter) Reset(ctx context.Context, rule Rule, identity string) error {
	key := fmt.Sprintf("%s:%s:%s", l.prefix, rule.Name, identity)
	return l.rdb.Del(ctx, key).Err()
}
