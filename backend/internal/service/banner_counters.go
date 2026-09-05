package service

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/autoimport/crm/internal/domain"
)

// BannerCounters — счётчики показов и кликов в Redis.
//
// Показ баннера случается на каждом открытии главной страницы. Писать
// UPDATE в базу на каждый показ означало бы конкуренцию за одну строку и
// раздувание журнала транзакций ради данных, точность которых до единицы
// никому не нужна. Счётчики копятся в Redis и переносятся в базу пачкой.
//
// При недоступности Redis показы просто не учитываются: реклама должна
// показываться даже тогда, когда статистику собрать нельзя.
type BannerCounters struct {
	client redis.UniversalClient
	prefix string
	log    *slog.Logger
}

func NewBannerCounters(client redis.UniversalClient, log *slog.Logger) *BannerCounters {
	return &BannerCounters{client: client, prefix: "banner", log: log}
}

func (c *BannerCounters) impressionsKey() string { return c.prefix + ":impressions" }
func (c *BannerCounters) clicksKey() string      { return c.prefix + ":clicks" }

// Таймаут на операции со счётчиками.
//
// Показ баннера не должен задерживать ответ страницы: если Redis отвечает
// дольше, счётчик теряется, и это приемлемая плата.
const counterTimeout = 300 * time.Millisecond

// RecordImpressions учитывает показ набора баннеров.
func (c *BannerCounters) RecordImpressions(ctx context.Context, banners []domain.Banner) {
	if c.client == nil || len(banners) == 0 {
		return
	}

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), counterTimeout)
	defer cancel()

	pipe := c.client.Pipeline()
	for index := range banners {
		pipe.HIncrBy(opCtx, c.impressionsKey(), banners[index].ID.String(), 1)
	}

	if _, err := pipe.Exec(opCtx); err != nil {
		c.log.DebugContext(ctx, "не удалось учесть показы баннеров", "error", err)
	}
}

// RecordClick учитывает переход по баннеру.
func (c *BannerCounters) RecordClick(ctx context.Context, bannerID uuid.UUID) {
	if c.client == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), counterTimeout)
	defer cancel()

	if err := c.client.HIncrBy(opCtx, c.clicksKey(), bannerID.String(), 1).Err(); err != nil {
		c.log.DebugContext(ctx, "не удалось учесть переход по баннеру", "error", err)
	}
}

// Merge добавляет накопленные в Redis значения к сохранённым в базе.
func (c *BannerCounters) Merge(ctx context.Context, banners []domain.Banner) {
	if c.client == nil || len(banners) == 0 {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, counterTimeout*3)
	defer cancel()

	impressions, err := c.client.HGetAll(opCtx, c.impressionsKey()).Result()
	if err != nil {
		return
	}
	clicks, err := c.client.HGetAll(opCtx, c.clicksKey()).Result()
	if err != nil {
		return
	}

	for index := range banners {
		id := banners[index].ID.String()
		banners[index].Impressions += parseCounter(impressions[id])
		banners[index].Clicks += parseCounter(clicks[id])
	}
}

func parseCounter(raw string) int64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// Drain забирает накопленные счётчики и очищает их.
//
// Чтение и удаление выполняются одной транзакцией Redis: между HGETALL и
// DEL иначе успевают попасть новые показы, и они пропадут безвозвратно.
func (c *BannerCounters) Drain(ctx context.Context) (impressions, clicks map[uuid.UUID]int64, err error) {
	if c.client == nil {
		return nil, nil, nil
	}

	pipe := c.client.TxPipeline()
	impressionsCmd := pipe.HGetAll(ctx, c.impressionsKey())
	clicksCmd := pipe.HGetAll(ctx, c.clicksKey())
	pipe.Del(ctx, c.impressionsKey(), c.clicksKey())

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, nil, err
	}

	return toCounterMap(impressionsCmd.Val()), toCounterMap(clicksCmd.Val()), nil
}

func toCounterMap(raw map[string]string) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(raw))
	for key, value := range raw {
		id, err := uuid.Parse(key)
		if err != nil {
			continue
		}
		if count := parseCounter(value); count > 0 {
			out[id] = count
		}
	}
	return out
}
