package cache

import (
	"cachex/log"
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) (string, error)
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Incr(ctx context.Context, key string) (int64, error)
	MGet(ctx context.Context, keys ...string) ([]interface{}, error)
}
type RedisCache struct {
	Client *redis.Client
}

func (r *RedisCache) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return r.Client.Del(ctx, keys...)

}
func (r *RedisCache) Get(ctx context.Context, key string) *redis.StringCmd {
	return r.Client.Get(ctx, key)

}
func (r *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) (string, error) {

	val, err := r.Client.Set(ctx, key, value, expiration).Result()
	if err != nil {
		log.Errorw("redis set failed", "key", key, "ttl", expiration.String(), "err", err.Error())
		return "", err
	}
	return val, nil
}

func (r *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	v, err := r.Client.Incr(ctx, key).Result()
	if err != nil {
		log.Errorw("redis incr failed", "key", key, "err", err.Error())
	}
	return v, err
}

func (r *RedisCache) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	v, err := r.Client.MGet(ctx, keys...).Result()
	if err != nil {
		log.Errorw("redis mget failed", "n", len(keys), "err", err.Error())
	}
	return v, err
}
