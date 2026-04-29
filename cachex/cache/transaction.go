package cache

import (
	"cachex/log"
	"context"
)

// Transaction executes a function within a managed transaction.
//
// Features:
//   - Context propagation
//   - AfterCommit hooks supported
//
// 中文说明：
// 事务执行入口：
//   - 自动管理 Begin / Commit / Rollback
//   - 支持 AfterCommit 回调

func (c *CacheDB) Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	release := log.BindCtx(ctx)
	defer release()

	log.Debugw("tx begin", "op", "Transaction")
	ctx, err := c.TxManager.Begin(ctx, c.DB)
	if err != nil {
		log.Errorw("tx begin failed", "op", "Transaction", "err", err.Error())
		return err
	}

	if err := fn(ctx); err != nil {
		log.Warnw("tx rollback", "op", "Transaction", "err", err.Error())
		_ = c.TxManager.Rollback(ctx)
		return err
	}

	if cerr := c.TxManager.Commit(ctx); cerr != nil {
		log.Errorw("tx commit failed", "op", "Transaction", "err", cerr.Error())
		return cerr
	}
	log.Debugw("tx commit", "op", "Transaction")
	return nil
}
