package cache

import (
	"cachex/cachex/tx"
	"context"
	"time"

	"gorm.io/gorm"
)

type PK interface {
	GetPK() string
	TableName() string
	PrimaryKey() string
}
type BaseModel[T PK] struct {
	// ===== Core =====
	Db    *gorm.DB        // Database connection (possibly sharded)
	table string          // Table name
	ctx   context.Context // Request context

	PrimaryKey string // Primary key field (e.g. "ID")
	shardKey   string // Sharding key for DB/table routing

	// ===== Cache Config =====
	Timeout          time.Duration // Base TTL for cache (with jitter)
	Seed             int           // Random jitter range (seconds)
	NullValueTimeout time.Duration // TTL for null cache (anti-penetration)
	ListValueTimeout time.Duration // TTL for list cache

	// ===== Infra =====
	cacheDB *CacheDB // Shared cache engine (Redis + L1 + SF + Router)
}

// Model initializes a BaseModel for a given entity.
//
// Behavior:
//   - Binds table name and primary key
//   - Inherits cache configuration
//
// 中文说明：
// 初始化模型操作入口：
//   - 绑定表名 + 主键
//   - 继承缓存配置
func NewModel[T PK](c *CacheDB) *BaseModel[T] {
	var t T

	return &BaseModel[T]{
		cacheDB:          c,
		Seed:             c.Seed,
		Timeout:          c.Timeout,
		ListValueTimeout: c.ListValueTimeout,
		NullValueTimeout: c.NullValueTimeout,
		table:            t.TableName(),
		PrimaryKey:       t.PrimaryKey(),
		ctx:              context.Background(),
	}
}

// WithContext returns a shallow copy of BaseModel with a new context.
//
// Note:
//   - Shares underlying resources (DB, cache, singleflight, L1)
//   - Safe for concurrent usage
//
// 中文说明：
//   - 共享底层资源（DB / Redis / L1 / singleflight）
//   - 支持并发安全
func (b *BaseModel[T]) WithContext(ctx context.Context) *BaseModel[T] {
	b.ctx = ctx
	return b
}
func (b *BaseModel[T]) SetTimeout(timeOut time.Duration) *BaseModel[T] {
	b.Timeout = timeOut
	return b
}
func (b *BaseModel[T]) SetSeed(seed int) *BaseModel[T] {
	b.Seed = seed
	return b
}

func (b *BaseModel[T]) SetNullValueTimeout(timeOut time.Duration) *BaseModel[T] {
	b.NullValueTimeout = timeOut
	return b
}
func (b *BaseModel[T]) SetListValueTimeout(timeOut time.Duration) *BaseModel[T] {
	b.ListValueTimeout = timeOut
	return b
}
func (b *BaseModel[T]) SetHashkey(key string) *BaseModel[T] {
	b.shardKey = key

	return b
}

// 获取DB连接
func (b *BaseModel[T]) getDB() {
	//  优先取事务
	if txCtx := tx.GetTx(b.ctx); txCtx != nil {
		b.Db = txCtx.DB
	} else {
		b.Db = b.cacheDB.DB
	}
	if b.shardKey != "" && b.cacheDB.Router != nil {
		db, table := b.cacheDB.Router.Route(b.ctx, b.shardKey, b.table)
		b.table = table
		if db != nil {
			b.Db = db
		}
	}
}
