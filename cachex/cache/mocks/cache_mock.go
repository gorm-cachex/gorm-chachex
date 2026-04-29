package mocks

import (
	"context"
	"errors"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"cachex/cachex/cache"
)

// ErrInjected 用于注入失败路径的错误。
var ErrInjected = errors.New("injected error")

// MockCache 通过 miniredis 提供"接近真实"的 Redis 行为，
// 同时允许通过 ErrOnSet/ErrOnGet 注入失败用于测试错误分支。
type MockCache struct {
	Server *miniredis.Miniredis
	Client *redis.Client

	ErrOnGet  error
	ErrOnSet  error
	ErrOnIncr error
	ErrOnMGet error
}

type tHelper interface {
	Helper()
	Fatalf(format string, args ...any)
}

// New 构造一个内嵌 miniredis 的 MockCache。Close 由调用方负责。
func New(t tHelper) *MockCache {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	return &MockCache{
		Server: s,
		Client: redis.NewClient(&redis.Options{Addr: s.Addr()}),
	}
}

func (m *MockCache) Close() {
	_ = m.Client.Close()
	m.Server.Close()
}

func (m *MockCache) Get(ctx context.Context, key string) *redis.StringCmd {
	if m.ErrOnGet != nil {
		cmd := redis.NewStringCmd(ctx)
		cmd.SetErr(m.ErrOnGet)
		return cmd
	}
	return m.Client.Get(ctx, key)
}

func (m *MockCache) Set(ctx context.Context, key string, val any, ttl time.Duration) (string, error) {
	if m.ErrOnSet != nil {
		return "", m.ErrOnSet
	}
	return m.Client.Set(ctx, key, val, ttl).Result()
}

func (m *MockCache) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return m.Client.Del(ctx, keys...)
}

func (m *MockCache) Incr(ctx context.Context, key string) (int64, error) {
	if m.ErrOnIncr != nil {
		return 0, m.ErrOnIncr
	}
	return m.Client.Incr(ctx, key).Result()
}

func (m *MockCache) MGet(ctx context.Context, keys ...string) ([]any, error) {
	if m.ErrOnMGet != nil {
		return nil, m.ErrOnMGet
	}
	return m.Client.MGet(ctx, keys...).Result()
}

// 编译期断言
var _ cache.Cache = (*MockCache)(nil)

