package cache

import (
	"cachex/json"
	"cachex/log"
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// GetList retrieves a paginated list with caching support.
//
// Flow:
//
//	ListKey → PK list → batch load entities
//
// Strategy:
//   - Cache stores PK list + total count
//   - Entities are loaded via batch PK queries
//
// 中文说明：
// 分页查询接口，支持缓存：
//
//	ListKey → 主键列表 → 批量加载实体
//
// 特性：
//   - 列表缓存（PK数组）
//   - 批量加载（减少 DB / Redis 次数）
//   - 支持排序、分页
func (b *BaseModel[T]) GetList(
	conds []CacheCondition,
	page int,
	pageSize int,
	order OrderClause,
) ([]T, int64, error) {

	// ===== 1. 参数校验 =====
	if err := b.cacheDB.ValidateConditions(conds); err != nil {
		log.Errorw("validate conditions failed", "op", "GetList", "err", err.Error())
		return nil, 0, err
	}
	if err := b.cacheDB.ValidatePageSize(pageSize); err != nil {
		log.Errorw("validate pagesize failed", "op", "GetList", "size", pageSize, "err", err.Error())
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}

	b.getDB()

	log.Debugw("get list start", "op", "GetList", "table", b.table, "page", page, "size", pageSize, "conds_n", len(conds), "order", order.String())

	// ===== 2. 事务中：完全走 DB =====
	if b.cacheDB.TxManager.InTx(b.ctx) {
		return b.getListFromDB(conds, page, pageSize, order)
	}

	// ===== 3. 是否可缓存 =====
	if !IsCacheableConds(conds) {
		log.Debugw("list cache bypass", "op", "GetList", "reason", "non-eq cond")
		return b.getListFromDB(conds, page, pageSize, order)
	}

	// ===== 4. 构造 List Cache Key =====
	listKey := b.cacheDB.ListKey(
		b.ctx,
		b.table,
		conds,
		order.String(),
		page,
		pageSize,
	)

	b.cacheDB.CallBeforeQuery(b.ctx, listKey)

	// ===== 5. 读 Redis =====
	val, err := b.cacheDB.Cache.Get(b.ctx, listKey).Result()
	if err == nil {

		log.Debugw("list cache hit", "op", "GetList", "key", listKey, "hit_layer", "redis")
		b.cacheDB.CallAfterQuery(b.ctx, listKey, true)

		var cv ListCacheValue
		if json.CJSON.Unmarshal([]byte(val), &cv) == nil {

			//  核心：通过 PK 批量加载
			list, err := b.batchGetByPK(cv.PKs)
			if err != nil {
				log.Errorw("batch load failed", "op", "GetList", "err", err.Error())
			}
			return list, cv.Total, err
		}

		log.Warnw("list cache corrupted", "op", "GetList", "key", listKey)
	}

	if err != nil && !errors.Is(err, redis.Nil) {
		log.Errorw("redis get failed", "op", "GetList", "key", listKey, "err", err.Error())
	}

	// ===== 6. DB 查 PK 列表 =====
	pks, total, err := b.getPKListFromDB(
		conds,
		page,
		pageSize,
		order,
	)
	if err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Infow("list empty", "op", "GetList")
			return []T{}, 0, nil
		}
		log.Errorw("get pk list failed", "op", "GetList", "err", err.Error())
		return nil, 0, err
	}

	if len(pks) == 0 {
		return []T{}, total, nil
	}

	// ===== 7. 写 List Cache =====
	cacheVal := ListCacheValue{
		PKs:   pks,
		Total: total,
	}

	if bytes, err := json.CJSON.Marshal(cacheVal); err == nil {
		ttl := b.ListValueTimeout + time.Duration(rand.Intn(b.Seed))*time.Second
		_, _ = b.cacheDB.Cache.Set(
			b.ctx,
			listKey,
			bytes,
			ttl,
		)
		log.Debugw("list cache set", "op", "GetList", "key", listKey, "pk_n", len(pks), "ttl_ms", ttl.Milliseconds())
		b.cacheDB.CallAfterUpdate(b.ctx, listKey)
	}

	// ===== 8. 批量加载实体 =====
	list, err := b.batchGetByPK(pks)
	if err != nil {
		log.Errorw("batch load failed", "op", "GetList", "err", err.Error())
	}
	return list, total, err
}

// delCondCache 失效组合条件键（更新时调用）。
func (b *BaseModel[T]) delCondCache(
	ctx context.Context,
	conds []CacheCondition,
	table string,
) {
	key := b.cacheDB.CompositeKey(conds, table)
	b.cacheDB.Cache.Del(ctx, key)
	b.cacheDB.CallAfterUpdate(b.ctx, key)
}
