package cache

import "cachex/log"

// Insert creates a new record and invalidates related caches.
//
// Behavior:
//   - Writes to DB
//   - Increments list version to invalidate list cache
//
// 中文说明：
// 插入数据，并触发缓存失效：
//   - 写数据库
//   - 更新列表版本（失效列表缓存）
func (b *BaseModel[T]) Insert(value interface{}) error {
	b.getDB()

	db := b.Db.WithContext(b.ctx).Table(b.table)

	// 1.执行 DB Insert
	if err := db.Create(value).Error; err != nil {
		log.Errorw("insert db failed", "op", "Insert", "table", b.table, "err", err.Error())
		return err
	}

	// 2.事务内：延迟 list version 失效

	b.cacheDB.TxManager.AfterCommit(b.ctx, func() {
		_, _ = b.cacheDB.Cache.Incr(b.ctx, b.cacheDB.ListVersionKey(b.table))
		b.cacheDB.CallAfterListInvalidate(b.ctx, b.table)
		log.Infow("list version bump", "op", "Insert", "table", b.table)
	})
	return nil

}
