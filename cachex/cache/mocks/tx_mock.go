package mocks

import (
	"context"

	"gorm.io/gorm"

	"cachex/cachex/tx"
)

// NoTxManager 在所有路径上视为"非事务"，AfterCommit 立即执行。
// 用于单元测试时绕过事务语义。
type NoTxManager struct{}

func (NoTxManager) Begin(ctx context.Context, db *gorm.DB) (context.Context, error) {
	return ctx, nil
}
func (NoTxManager) Commit(ctx context.Context) error              { return nil }
func (NoTxManager) Rollback(ctx context.Context) error            { return nil }
func (NoTxManager) AfterCommit(ctx context.Context, fn func())    { fn() }
func (NoTxManager) InTx(ctx context.Context) bool                 { return false }

var _ tx.TxManager = NoTxManager{}

