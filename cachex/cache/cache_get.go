package cache

import (
	"cachex/json"
	"cachex/log"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// errString 把 error 转换为字符串字段值，nil 时返回空串，避免 fmt 输出 "<nil>"。
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetByPK retrieves a record by primary key.
//
// ⚡ Performance:
//   - L1 cache (~us)
//   - Redis (~ms)
//   - DB fallback (~ms)
//
// 🛡️ Protection:
//   - Cache breakdown → singleflight
//   - Cache penetration → null cache
//   - Cache avalanche → jitter TTL
//
// 🧠 Design:
//   - Multi-level cache
//   - Eventual consistency
//   - L1 自适应：当 EnableL1Cache=false 时 l1 为 nil，
//     L1Cache 的所有方法均带 nil-receiver 守卫，等价跳过。
//
// 中文：
// 主键查询接口，具备高性能 + 高并发保护能力
func (b *BaseModel[T]) GetByPK(pk any) (*T, error) {
	b.getDB()
	key := b.cacheDB.PkKey(pk, b.table)
	b.cacheDB.CallBeforeQuery(b.ctx, key)
	log.Debugw("get by pk start", "op", "GetByPK", "table", b.table, "pk", pk, "key", key)

	// ===== L1 缓存 =====
	if val, ok := b.cacheDB.l1.Get(key); ok {
		if val == "__null__" {
			log.Debugw("cache hit", "op", "GetByPK", "key", key, "hit_layer", "l1", "null", true)
			return nil, gorm.ErrRecordNotFound
		}
		t := val.(*T)
		log.Debugw("cache hit", "op", "GetByPK", "key", key, "hit_layer", "l1")
		return t, nil
	}

	// ===== Redis =====
	if !b.cacheDB.TxManager.InTx(b.ctx) {
		val, err := b.cacheDB.Cache.Get(b.ctx, key).Result()
		if err == nil {
			log.Debugw("cache hit", "op", "GetByPK", "key", key, "hit_layer", "redis")
			b.cacheDB.CallAfterQuery(b.ctx, key, true)
			if val == "__null__" {
				b.cacheDB.l1.Set(key, "__null__", b.cacheDB.L1NullTTL)
				return nil, gorm.ErrRecordNotFound
			}
			var t T
			_ = json.CJSON.Unmarshal([]byte(val), &t)
			b.cacheDB.l1.Set(key, &t, b.cacheDB.L1ValueTTL)
			return &t, nil
		}
		if !errors.Is(err, redis.Nil) {
			log.Errorw("redis get failed", "op", "GetByPK", "key", key, "err", err.Error())
			return nil, err
		}
		log.Debugw("cache miss", "op", "GetByPK", "key", key, "layer", "redis")
	}

	// ===== singleflight =====
	log.Debugw("singleflight begin", "op", "GetByPK", "key", key)
	v, err, _ := b.cacheDB.sf.Do(key, func() (interface{}, error) {
		// 再双检 Redis
		if !b.cacheDB.TxManager.InTx(b.ctx) {
			val, err := b.cacheDB.Cache.Get(b.ctx, key).Result()
			if err == nil {
				if val == "__null__" {
					return nil, gorm.ErrRecordNotFound
				}
				var t T
				_ = json.CJSON.Unmarshal([]byte(val), &t)
				return &t, nil
			}
		}

		// ===== DB 查询 =====
		b.cacheDB.CallAfterQuery(b.ctx, key, false)
		// NOTE: 历史遗留——曾用于穿透抑制，singleflight 已能合并并发，
		// 这里不再 sleep。保留注释以便回溯。
		// time.Sleep(1 * time.Millisecond)

		var t T
		dbStart := time.Now()
		err := b.Db.WithContext(b.ctx).
			Table(b.table).
			Where(fmt.Sprintf("%s = ?", b.PrimaryKey), pk).
			First(&t).Error
		log.Infow("db fallback done", "op", "GetByPK", "key", key, "elapsed_ms", time.Since(dbStart).Milliseconds(), "err", errString(err))
		if err != nil {
			if !b.cacheDB.TxManager.InTx(b.ctx) && errors.Is(err, gorm.ErrRecordNotFound) {
				_, _ = b.cacheDB.Cache.Set(b.ctx, key, "__null__", b.NullValueTimeout)
				b.cacheDB.l1.Set(key, "__null__", b.cacheDB.L1NullTTL)
				log.Debugw("null cache set", "op", "GetByPK", "key", key, "ttl_ms", b.NullValueTimeout.Milliseconds())
				b.cacheDB.CallAfterUpdate(b.ctx, key)
				return nil, err
			}
			log.Errorw("db query failed", "op", "GetByPK", "key", key, "err", err.Error())
			return nil, err
		}

		// 写缓存
		if !b.cacheDB.TxManager.InTx(b.ctx) {
			bytes, _ := json.CJSON.Marshal(&t)
			ttl := b.Timeout + time.Duration(rand.Intn(b.Seed))*time.Second
			_, _ = b.cacheDB.Cache.Set(
				b.ctx,
				key,
				bytes,
				ttl,
			)
			b.cacheDB.l1.Set(key, &t, b.cacheDB.L1ValueTTL)
			log.Debugw("entity cache set", "op", "GetByPK", "key", key, "ttl_ms", ttl.Milliseconds())
			b.cacheDB.CallAfterUpdate(b.ctx, key)
		}
		return &t, nil
	})

	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Errorw("get by pk failed", "op", "GetByPK", "key", key, "err", err.Error())
		}
		return nil, err
	}
	if v == nil {
		log.Debugw("get by pk not found", "op", "GetByPK", "key", key)
		return nil, gorm.ErrRecordNotFound
	}

	return v.(*T), nil
}

// GetByUnique retrieves a record by a unique index.
//
// Flow:
//
//	UniqueKey → PK → Entity
//
// 中文说明：
// 通过唯一索引查询数据：UniqueKey → 主键 → 实体。
func (b *BaseModel[T]) GetByUnique(field string, value any) (*T, error) {
	b.getDB()
	uk := b.cacheDB.UkKey(field, value, b.table)

	// ===== 事务中直接查 DB =====
	if b.cacheDB.TxManager.InTx(b.ctx) {
		var pk sql.NullString
		err := b.Db.WithContext(b.ctx).
			Table(b.table).
			Select(b.PrimaryKey).
			Where(fmt.Sprintf("%s = ?", field), value).
			Scan(&pk).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Errorw("uk db query failed", "op", "GetByUnique", "uk", uk, "err", err.Error())
				return nil, err
			}
			return nil, gorm.ErrRecordNotFound
		}
		return b.GetByPK(pk.String)
	}

	b.cacheDB.CallBeforeQuery(b.ctx, uk)
	log.Debugw("get by uk start", "op", "GetByUnique", "table", b.table, "uk", uk)

	// ===== 1. Redis：UK → PK =====
	id, err := b.cacheDB.Cache.Get(b.ctx, uk).Result()
	if err == nil {
		b.cacheDB.CallAfterQuery(b.ctx, uk, true)
		log.Debugw("cache hit", "op", "GetByUnique", "key", uk, "hit_layer", "redis")
		if id == "__null__" {
			return nil, gorm.ErrRecordNotFound
		}
		return b.GetByPK(id)
	}
	if !errors.Is(err, redis.Nil) {
		log.Errorw("redis get failed", "op", "GetByUnique", "key", uk, "err", err.Error())
		return nil, err
	}
	b.cacheDB.CallAfterQuery(b.ctx, uk, false)
	log.Debugw("cache miss", "op", "GetByUnique", "key", uk, "layer", "redis")

	// ===== 2.DB 查 PK =====
	var pk sql.NullString
	dbStart := time.Now()
	err = b.Db.WithContext(b.ctx).
		Table(b.table).
		Select(b.PrimaryKey).
		Where(fmt.Sprintf("%s = ?", field), value).
		Scan(&pk).Error
	log.Infow("db fallback done", "op", "GetByUnique", "key", uk, "elapsed_ms", time.Since(dbStart).Milliseconds(), "err", errString(err))
	if err != nil {
		_, _ = b.cacheDB.Cache.Set(b.ctx, uk, "__null__", b.NullValueTimeout)
		return nil, err
	}
	if !pk.Valid {
		_, _ = b.cacheDB.Cache.Set(b.ctx, uk, "__null__", b.NullValueTimeout)
		log.Debugw("null cache set", "op", "GetByUnique", "key", uk, "ttl_ms", b.NullValueTimeout.Milliseconds())
		return nil, gorm.ErrRecordNotFound
	}

	// ===== 3.缓存 UK → PK =====
	ttl := b.Timeout + time.Duration(rand.Intn(b.Seed))*time.Second
	_, _ = b.cacheDB.Cache.Set(
		b.ctx,
		uk,
		pk.String,
		ttl,
	)
	log.Debugw("uk cache set", "op", "GetByUnique", "key", uk, "ttl_ms", ttl.Milliseconds())
	b.cacheDB.CallAfterUpdate(b.ctx, uk)

	// ===== 4.走 PK 缓存 =====
	return b.GetByPK(pk.String)
}

// GetByConditions retrieves a record using equality-based conditions.
//
// 中文说明：通过组合条件查询（仅支持等值查询缓存）：条件 → 主键 → 实体。
func (b *BaseModel[T]) GetByConditions(conds []CacheCondition) (*T, error) {
	if err := b.cacheDB.ValidateConditions(conds); err != nil {
		log.Errorw("validate conditions failed", "op", "GetByConditions", "err", err.Error())
		return nil, err
	}

	b.getDB()
	ck := b.cacheDB.CompositeKey(conds, b.table)

	// ===== 事务中：直接查 DB =====
	if b.cacheDB.TxManager.InTx(b.ctx) {
		pkStr, err := b.getPKByConditions(conds)
		if err != nil {
			return nil, err
		}
		if pkStr == "" {
			return nil, gorm.ErrRecordNotFound
		}
		return b.GetByPK(pkStr)
	}

	b.cacheDB.CallBeforeQuery(b.ctx, ck)
	log.Debugw("get by conds start", "op", "GetByConditions", "table", b.table, "key", ck, "conds_n", len(conds))

	// ===== 1. Redis：条件 → PK =====
	id, err := b.cacheDB.Cache.Get(b.ctx, ck).Result()
	if err == nil {
		b.cacheDB.CallAfterQuery(b.ctx, ck, true)
		log.Debugw("cache hit", "op", "GetByConditions", "key", ck, "hit_layer", "redis")
		if id == "__null__" {
			return nil, gorm.ErrRecordNotFound
		}
		return b.GetByPK(id)
	}
	if !errors.Is(err, redis.Nil) {
		log.Errorw("redis get failed", "op", "GetByConditions", "key", ck, "err", err.Error())
		return nil, err
	}
	b.cacheDB.CallAfterQuery(b.ctx, ck, false)
	log.Debugw("cache miss", "op", "GetByConditions", "key", ck, "layer", "redis")

	// ===== 2. DB 查 PK =====
	// pkStr=="" 与 err==gorm.ErrRecordNotFound 在当前实现中等价，
	// 二者都会在下方写入 __null__ 防穿透。
	pkStr, err := b.getPKByConditions(conds)
	if err != nil {
		_, _ = b.cacheDB.Cache.Set(b.ctx, ck, "__null__", b.NullValueTimeout)
		log.Debugw("null cache set", "op", "GetByConditions", "key", ck, "ttl_ms", b.NullValueTimeout.Milliseconds())
		return nil, err
	}
	if pkStr == "" {
		_, _ = b.cacheDB.Cache.Set(b.ctx, ck, "__null__", b.NullValueTimeout)
		log.Debugw("null cache set", "op", "GetByConditions", "key", ck, "ttl_ms", b.NullValueTimeout.Milliseconds())
		return nil, gorm.ErrRecordNotFound
	}

	// ===== 3. 缓存 条件 → PK =====
	ttl := b.Timeout + time.Duration(rand.Intn(b.Seed))*time.Second
	_, _ = b.cacheDB.Cache.Set(
		b.ctx,
		ck,
		pkStr,
		ttl,
	)
	log.Debugw("ck cache set", "op", "GetByConditions", "key", ck, "ttl_ms", ttl.Milliseconds())

	// ===== 4. 走 PK 缓存 =====
	return b.GetByPK(pkStr)
}

