package cache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"cachex/cachex/cache"
)

func TestUnit_RedisCache_SetFailure_LogsKey(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rc := &cache.RedisCache{Client: redis.NewClient(&redis.Options{Addr: s.Addr()})}
	// 关闭 server 触发 Set 失败
	s.Close()

	buf, restore := withCapturedLog(t)
	defer restore()

	_, err = rc.Set(context.Background(), "users:pk:1", "x", time.Second)
	if err == nil {
		t.Fatalf("expected error after server close")
	}
	out := buf.String()
	if !strings.Contains(out, "redis set failed") || !strings.Contains(out, "key=users:pk:1") || !strings.Contains(out, "err=") {
		t.Fatalf("structured log missing: %s", out)
	}
}

