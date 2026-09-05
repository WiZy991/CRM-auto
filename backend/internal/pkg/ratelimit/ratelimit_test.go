package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestLimiterBlocksAfterBurst(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:16380"
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis недоступен: " + err.Error())
	}

	lim := New(rdb, "test-rl")
	rule := Rule{Name: "burst", Limit: 2, Window: time.Minute, Burst: 2}
	identity := uuid.NewString()

	first, err := lim.Allow(ctx, rule, identity)
	if err != nil {
		t.Fatalf("первый запрос: %v", err)
	}
	second, err := lim.Allow(ctx, rule, identity)
	if err != nil {
		t.Fatalf("второй запрос: %v", err)
	}
	third, err := lim.Allow(ctx, rule, identity)
	if err != nil {
		t.Fatalf("третий запрос: %v", err)
	}

	if !first.Allowed || !second.Allowed {
		t.Fatalf("первые два запроса должны пройти, получено %v %v", first, second)
	}
	if third.Allowed {
		t.Fatal("третий запрос сверх burst должен быть отклонён")
	}
}
