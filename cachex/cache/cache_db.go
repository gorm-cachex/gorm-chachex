package cache

import (
	"cachex/cachex/hook"
	"cachex/cachex/router"
	"cachex/cachex/tx"
	"cachex/log"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// CacheDB is the core entry of cachex.
//
// It combines:
// - L1 in-memory cache
// - Redis distributed cache
// - DB fallback (GORM)
// - singleflight (anti-breakdown)
// - sharding router
// - transaction manager
//
// CacheDB 是 cachex 的核心入口，整合了：
// - 本地缓存（L1）
// - Redis 分布式缓存
// - 数据库回源（GORM）
// - singleflight 防击穿
// - 分库分表路由
// - 事务管理
type CacheDB struct {
	// ===== Core =====
	DB *gorm.DB
	// 主数据库连接（默认连接）
	// Primary database connection (default DB)

	sf *singleflight.Group
	// 单飞机制（防止缓存击穿）
	// Singleflight group (prevents cache breakdown)

	Cache Cache
	// 分布式缓存（通常是 Redis）
	// Distributed cache (usually Redis)

	Router router.Router
	// 分库分表路由（可插拔）
	// Sharding router (pluggable DB/table routing)

	TxManager tx.TxManager
	// 事务管理器（支持 AfterCommit 等扩展）
	// Transaction manager (supports AfterCommit hooks, etc.)

	Hooks []hook.Hook
	// 生命周期 Hook（监控 / 日志 / tracing）
	// Lifecycle hooks (metrics / logging / tracing)

	// ===== L1 Cache =====
	EnableL1Cache bool
	// 是否启用本地缓存（默认 false）
	// Enable local (in-memory) cache

	l1 *L1Cache
	// 本地缓存实例（进程内缓存）
	// In-memory L1 cache instance

	L1ValueTTL time.Duration
	// 本地缓存（有值）TTL（推荐 1~3 秒）
	// TTL for L1 cache (value), recommended 1~3 seconds

	L1NullTTL time.Duration
	// 本地缓存（空值）TTL（推荐 300ms~1s）
	// TTL for L1 cache (null), recommended 300ms~1s

	// ===== Cache TTL Config =====
	Timeout time.Duration
	// Redis缓存TTL（带随机抖动）
	// Base TTL for Redis cache (with jitter)

	Seed int
	// TTL随机抖动范围（秒）
	// Random jitter range (seconds) for cache expiration

	NullValueTimeout time.Duration
	// Redis空值缓存TTL（防止缓存穿透）
	// TTL for null values in Redis (anti-penetration)

	ListValueTimeout time.Duration
	// 列表缓存TTL（分页/列表查询）
	// TTL for list cache (pagination queries)

	// ===== Limits / Protection =====
	Limits Limits
	// 查询限制（防止滥用/攻击）
	// Query limits (protect against abuse / heavy queries)
}

func NewCacheDb(
	c *CacheDB) *CacheDB {
	if c.Timeout == 0 {
		c.Timeout = time.Minute * 10
	}
	if c.ListValueTimeout == 0 {
		c.ListValueTimeout = time.Minute * 10
	}
	if c.NullValueTimeout == 0 {
		c.NullValueTimeout = time.Minute * 5
	}
	if c.Limits.MaxConditions == 0 {
		c.Limits.MaxConditions = defaultLimits.MaxConditions
	}
	if c.Limits.MaxDBIn == 0 {
		c.Limits.MaxDBIn = defaultLimits.MaxDBIn
	}
	if c.Limits.MaxPageSize == 0 {
		c.Limits.MaxPageSize = defaultLimits.MaxPageSize
	}
	if c.Limits.MaxRedisBatch == 0 {
		c.Limits.MaxRedisBatch = defaultLimits.MaxRedisBatch
	}
	if c.Seed == 0 {
		c.Seed = 500
	}
	if c.EnableL1Cache {
		c.l1 = &L1Cache{}
	}
	if c.L1NullTTL == 0 {
		c.L1NullTTL = 500 * time.Millisecond
	}
	if c.L1ValueTTL == 0 {
		c.L1ValueTTL = time.Second * 2
	}
	c.sf = &singleflight.Group{}
	return c
}

func (c *CacheDB) UseHook(h hook.Hook) {
	c.Hooks = append(c.Hooks, h)
}
func (c *CacheDB) ListVersionKey(table string) string {
	return fmt.Sprintf("%s:list:version", table)
}

func (c *CacheDB) getListVersion(ctx context.Context, table string) int64 {
	v, err := c.Cache.Get(ctx, c.ListVersionKey(table)).Int64()
	if errors.Is(err, redis.Nil) {
		// 首次初始化：版本号永久存活（ttl=0 即不过期），由 Insert/Update 触发 Incr 更新
		_, _ = c.Cache.Set(ctx, c.ListVersionKey(table), 1, 0)
		log.Debugw("list version init", "table", table)
		return 1
	}
	if err != nil {
		log.Warnw("list version fallback to 1", "table", table, "err", err.Error())
		return 1
	}
	return v
}
func (c *CacheDB) CallBeforeQuery(ctx context.Context, key string) {
	// 自动从 ctx 提取 req_id，绑定到当前 goroutine，便于贯穿日志追踪。
	_ = log.BindCtx(ctx)
	for _, h := range c.Hooks {
		h.BeforeQuery(ctx, key)
	}
}
func (c *CacheDB) CallAfterQuery(ctx context.Context, key string, hit bool) {
	for _, h := range c.Hooks {
		h.AfterQuery(ctx, key, hit)
	}
}
func (c *CacheDB) CallAfterUpdate(ctx context.Context, key string) {
	for _, h := range c.Hooks {
		h.AfterUpdate(ctx, key)
	}
}
func (c *CacheDB) CallAfterListInvalidate(ctx context.Context, table string) {
	for _, h := range c.Hooks {
		h.AfterListInvalidate(ctx, table)
	}
}
