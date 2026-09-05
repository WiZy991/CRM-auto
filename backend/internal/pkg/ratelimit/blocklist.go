package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Blocklist — адаптивная блокировка адресов.
//
// Логика: каждое подозрительное событие (401, 403, 404 на несуществующий
// маршрут, срабатывание лимита, попытка обращения к сканерному пути)
// увеличивает счётчик адреса в скользящем окне. При превышении порога адрес
// блокируется, и длительность блокировки удваивается с каждым новым
// нарушением — от нескольких минут до суток.
//
// Смысл именно в эскалации: единичная ошибка пароля не должна ничего
// блокировать, а методичный перебор становится всё дороже с каждой волной.
type Blocklist struct {
	rdb    redis.UniversalClient
	prefix string

	threshold    int
	window       time.Duration
	baseDuration time.Duration
	maxDuration  time.Duration
}

// BlocklistConfig — параметры адаптивной блокировки.
type BlocklistConfig struct {
	Threshold    int
	Window       time.Duration
	BaseDuration time.Duration
	MaxDuration  time.Duration
}

func NewBlocklist(rdb redis.UniversalClient, prefix string, cfg BlocklistConfig) *Blocklist {
	if prefix == "" {
		prefix = "ban"
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 25
	}
	if cfg.Window <= 0 {
		cfg.Window = 5 * time.Minute
	}
	if cfg.BaseDuration <= 0 {
		cfg.BaseDuration = 5 * time.Minute
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = 24 * time.Hour
	}
	return &Blocklist{
		rdb:          rdb,
		prefix:       prefix,
		threshold:    cfg.Threshold,
		window:       cfg.Window,
		baseDuration: cfg.BaseDuration,
		maxDuration:  cfg.MaxDuration,
	}
}

func (b *Blocklist) banKey(ip string) string    { return fmt.Sprintf("%s:ip:%s", b.prefix, ip) }
func (b *Blocklist) scoreKey(ip string) string  { return fmt.Sprintf("%s:score:%s", b.prefix, ip) }
func (b *Blocklist) strikeKey(ip string) string { return fmt.Sprintf("%s:strikes:%s", b.prefix, ip) }

// IsBanned проверяет адрес. Вызывается на каждый входящий запрос, поэтому
// это единственная операция — GET по ключу.
//
// При недоступности Redis запрос пропускается: ошибка кеша не должна
// закрывать платформу целиком.
func (b *Blocklist) IsBanned(ctx context.Context, ip string) (bool, time.Duration) {
	ttl, err := b.rdb.TTL(ctx, b.banKey(ip)).Result()
	if err != nil || ttl <= 0 {
		return false, 0
	}
	return true, ttl
}

// registerScript увеличивает счётчик подозрительных событий и, при
// превышении порога, ставит блокировку с растущей длительностью.
//
// KEYS[1] — счётчик событий, KEYS[2] — ключ бана, KEYS[3] — счётчик нарушений
// ARGV[1] — вес события, ARGV[2] — порог, ARGV[3] — окно (сек),
// ARGV[4] — базовая длительность (сек), ARGV[5] — максимум (сек)
//
// Возвращает {забанен (1/0), длительность бана в секундах, текущий счёт}
var registerScript = redis.NewScript(`
local weight    = tonumber(ARGV[1])
local threshold = tonumber(ARGV[2])
local window    = tonumber(ARGV[3])
local base      = tonumber(ARGV[4])
local maxdur    = tonumber(ARGV[5])

local score = redis.call('INCRBY', KEYS[1], weight)
if score == weight then
    redis.call('EXPIRE', KEYS[1], window)
end

if score < threshold then
    return {0, 0, score}
end

-- Порог превышен: счётчик обнуляется, число нарушений растёт.
redis.call('DEL', KEYS[1])
local strikes = redis.call('INCR', KEYS[3])
redis.call('EXPIRE', KEYS[3], 86400)

-- Экспоненциальный рост: base * 2^(strikes-1), но не выше maxdur.
local duration = base * math.pow(2, math.min(strikes - 1, 20))
if duration > maxdur then
    duration = maxdur
end
duration = math.floor(duration)

redis.call('SET', KEYS[2], strikes, 'EX', duration)
return {1, duration, score}
`)

// Verdict — результат регистрации подозрительного события.
type Verdict struct {
	Banned   bool
	Duration time.Duration
	Score    int
}

// RegisterSuspicious отмечает подозрительное событие. Вес позволяет
// оценивать события по-разному: обращение к /wp-admin опаснее одиночной
// ошибки пароля, поэтому набирает порог быстрее.
func (b *Blocklist) RegisterSuspicious(ctx context.Context, ip string, weight int) (Verdict, error) {
	if weight <= 0 {
		weight = 1
	}

	res, err := registerScript.Run(ctx, b.rdb,
		[]string{b.scoreKey(ip), b.banKey(ip), b.strikeKey(ip)},
		weight, b.threshold, int(b.window.Seconds()),
		int(b.baseDuration.Seconds()), int(b.maxDuration.Seconds()),
	).Slice()
	if err != nil {
		return Verdict{}, fmt.Errorf("регистрация подозрительного события: %w", err)
	}

	verdict := Verdict{}
	if len(res) >= 3 {
		banned, _ := res[0].(int64)
		duration, _ := res[1].(int64)
		score, _ := res[2].(int64)

		verdict.Banned = banned == 1
		verdict.Duration = time.Duration(duration) * time.Second
		verdict.Score = int(score)
	}
	return verdict, nil
}

// Ban блокирует адрес принудительно — из админки или по решению
// эксплуатации.
func (b *Blocklist) Ban(ctx context.Context, ip string, duration time.Duration) error {
	if duration <= 0 {
		duration = b.baseDuration
	}
	return b.rdb.Set(ctx, b.banKey(ip), "manual", duration).Err()
}

// Unban снимает блокировку и сбрасывает историю нарушений.
func (b *Blocklist) Unban(ctx context.Context, ip string) error {
	return b.rdb.Del(ctx, b.banKey(ip), b.scoreKey(ip), b.strikeKey(ip)).Err()
}
