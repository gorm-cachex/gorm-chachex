package cache

import (
	errors2 "cachex/cachex/errors"
	"cachex/log"
	"fmt"
)

// Update modifies a record by primary key with optional conditions.
//
// Features:
//   - Supports optimistic locking
//   - Invalidates PK cache and list cache
//
// 中文说明：
// 更新数据（基于主键）：
//   - 支持乐观锁
//   - 清除主键缓存 + 列表缓存
//
// 特性：
//   - 条件更新
//   - 事务延迟失效（AfterCommit）
func (b *BaseModel[T]) Update(
	pk any,
	updates map[string]interface{},
	opt *UpdateOption,
) (*UpdateResult, error) {
	var err error
	b.getDB()

	hasVersion := opt != nil && opt.ExpectedVersion != nil
	log.Debugw("update start", "op", "Update", "table", b.table, "pk", pk, "fields_n", len(updates), "has_version", hasVersion)

	q := b.Db.WithContext(b.ctx).Table(b.table).Where(fmt.Sprintf("%s = ?", b.PrimaryKey), pk)

	// 乐观锁,
	if opt != nil && opt.ExpectedVersion != nil {
		q = q.Where("version = ?", *opt.ExpectedVersion)
		updates["version"] = *opt.ExpectedVersion + 1
	}

	// 附加条件（可选）
	if opt != nil && opt.Conds != nil {
		q, err = ApplyConds(q, opt.Conds)
		if err != nil {
			log.Errorw("apply conds failed", "op", "Update", "err", err.Error())
			return &UpdateResult{Updated: false}, err
		}
	}

	res := q.Updates(updates)
	if res.Error != nil {
		log.Errorw("update db failed", "op", "Update", "table", b.table, "pk", pk, "err", res.Error.Error())
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		log.Warnw("update no rows affected", "op", "Update", "table", b.table, "pk", pk)
		return &UpdateResult{Updated: false}, errors2.RowsAffected
	}

	invalidate := func() {
		pkKey := b.cacheDB.PkKey(pk, b.table)
		b.cacheDB.Cache.Del(b.ctx, pkKey)
		// L1 同步失效，避免本进程仍读到旧值
		b.cacheDB.l1.Delete(pkKey)
		b.cacheDB.CallAfterUpdate(b.ctx, pkKey)
		log.Infow("cache invalidate", "op", "Update", "table", b.table, "pk", pk, "reason", "update")

		_, _ = b.cacheDB.Cache.Incr(
			b.ctx,
			b.cacheDB.ListVersionKey(b.table),
		)
		if opt != nil && opt.Conds != nil {
			b.delCondCache(b.ctx, opt.Conds, b.table)
		}
		b.cacheDB.CallAfterListInvalidate(b.ctx, b.table)

	}
	// 事务执行过程中不操作缓存,事务执行完了，外部调用

	b.cacheDB.TxManager.AfterCommit(b.ctx, invalidate)

	return &UpdateResult{
		RowsAffected: res.RowsAffected,
		Updated:      true,
	}, nil
}

// UpdateByPKWithTx updates a record inside a transaction.
//
// Behavior:
//   - Executes update in DB
//   - Defers cache invalidation until commit
//
// 中文说明：
// 事务内更新数据：
//   - 更新 DB
//   - 提交后再清理缓存（避免脏读）
func (b *BaseModel[T]) UpdateByPKWithTx(

	pk any,
	updates interface{},
) error {
	b.getDB()

	if err := b.Db.Table(b.table).
		Where(fmt.Sprintf("%s = ?", b.PrimaryKey), pk).
		Updates(updates).Error; err != nil {
		log.Errorw("update db failed", "op", "UpdateByPKWithTx", "table", b.table, "pk", pk, "err", err.Error())
		return err
	}

	// 延迟到事务提交后执行
	b.cacheDB.TxManager.AfterCommit(b.ctx, func() {
		pkKey := b.cacheDB.PkKey(pk, b.table)
		b.cacheDB.Cache.Del(b.ctx, pkKey)
		b.cacheDB.l1.Delete(pkKey)
		_, _ = b.cacheDB.Cache.Incr(b.ctx, b.cacheDB.ListVersionKey(b.table))
		b.cacheDB.CallAfterUpdate(b.ctx, b.cacheDB.ListVersionKey(b.table))
		log.Infow("cache invalidate", "op", "UpdateByPKWithTx", "table", b.table, "pk", pk, "reason", "tx update")
	})

	return nil
}
