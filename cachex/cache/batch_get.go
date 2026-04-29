package cache

import (
	"cachex/json"
	"cachex/log"
	"fmt"
	"math/rand"
	"time"
)

// batchGetByPK 批量按主键加载实体：先走 Redis 批量，缺失部分回源 DB 并补写缓存。
//
// 中文说明：
//   - 优先 MGet 命中
//   - 缺失部分 IN 查询 DB
//   - DB 命中实体回写缓存；DB 未命中则写入 __null__ 防穿透
//   - 最终按入参 pks 顺序返回
func (b *BaseModel[T]) batchGetByPK(pks []string) ([]T, error) {

	if b.cacheDB.TxManager.InTx(b.ctx) {
		return b.batchGetByPKFromDB(pks)
	}

	ctx := b.ctx

	resultMap := make(map[string]T, len(pks))
	var missing []string

	// ========= 1. Redis =========
	for i := 0; i < len(pks); i += b.cacheDB.Limits.MaxRedisBatch {

		end := min(i+b.cacheDB.Limits.MaxRedisBatch, len(pks))

		keys := make([]string, 0, end-i)
		for _, pk := range pks[i:end] {
			keys = append(keys, b.cacheDB.PkKey(pk, b.table))
		}

		values, err := b.cacheDB.Cache.MGet(ctx, keys...)
		if err != nil {
			log.Errorw("redis mget failed", "op", "batchGetByPK", "n", len(keys), "err", err.Error())
			missing = append(missing, pks[i:end]...)
			continue
		}

		var hit, miss int
		for idx, val := range values {
			pk := pks[i+idx]

			if val == nil {
				missing = append(missing, pk)
				miss++
				continue
			}

			str, ok := val.(string)
			if !ok || str == "__null__" {
				// __null__ 视为命中（穿透防御），不计入回源
				hit++
				continue
			}

			var entity T
			if err := json.CJSON.Unmarshal([]byte(str), &entity); err != nil {
				log.Errorw("entity unmarshal failed", "op", "batchGetByPK", "pk", pk, "err", err.Error())
				missing = append(missing, pk)
				miss++
				continue
			}

			resultMap[pk] = entity
			hit++
		}
		log.Debugw("batch redis", "op", "batchGetByPK", "batch", len(keys), "hit", hit, "miss", miss)
	}

	// ========= 2. DB =========
	if len(missing) > 0 {
		dbList, err := b.batchGetByPKFromDB(missing)
		if err != nil {
			log.Errorw("batch db failed", "op", "batchGetByPK", "n", len(missing), "err", err.Error())
			return nil, err
		}

		found := make(map[string]struct{}, len(dbList))

		for _, item := range dbList {
			pk := item.GetPK()
			found[pk] = struct{}{}
			resultMap[pk] = item

			bytes, _ := json.CJSON.Marshal(item)

			_, _ = b.cacheDB.Cache.Set(
				ctx,
				b.cacheDB.PkKey(pk, b.table),
				bytes,
				b.Timeout+time.Duration(rand.Intn(b.Seed))*time.Second,
			)
		}

		// null cache
		for _, pk := range missing {
			if _, ok := found[pk]; !ok {
				_, _ = b.cacheDB.Cache.Set(
					ctx,
					b.cacheDB.PkKey(pk, b.table),
					"__null__",
					b.NullValueTimeout,
				)
			}
		}
	}

	// ========= 3. 顺序 =========
	out := make([]T, 0, len(pks))
	for _, pk := range pks {
		if v, ok := resultMap[pk]; ok {
			out = append(out, v)
		}
	}

	return out, nil
}

// batchGetByPKFromDB 直接通过 IN 查询从 DB 批量加载实体，按 MaxDBIn 切片避免超长 SQL。
func (b *BaseModel[T]) batchGetByPKFromDB(pks []string) ([]T, error) {

	ctx := b.ctx

	// pk -> entity
	resultMap := make(map[string]T, len(pks))

	// 分批查询
	for i := 0; i < len(pks); i += b.cacheDB.Limits.MaxDBIn {

		end := i + b.cacheDB.Limits.MaxDBIn
		if end > len(pks) {
			end = len(pks)
		}

		sub := pks[i:end]

		var list []T

		start := time.Now()
		err := b.Db.WithContext(ctx).
			Table(b.table).
			Where(fmt.Sprintf("%s IN ?", b.PrimaryKey), sub).
			Find(&list).Error
		if err != nil {
			log.Errorw("batch db query failed", "op", "batchGetByPKFromDB", "n", len(sub), "err", err.Error())
			return nil, err
		}
		log.Debugw("batch db", "op", "batchGetByPKFromDB", "in", len(sub), "n", len(list), "elapsed_ms", time.Since(start).Milliseconds())

		for _, item := range list {
			resultMap[item.GetPK()] = item
		}
	}

	// 按顺序组装
	out := make([]T, 0, len(pks))
	for _, pk := range pks {
		if v, ok := resultMap[pk]; ok {
			out = append(out, v)
		}
	}

	return out, nil
}

