package tx

import (
	"context"

	"gorm.io/gorm"
)

type TxManager interface {
	Begin(ctx context.Context, db *gorm.DB) (context.Context, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error

	AfterCommit(ctx context.Context, fn func())
	InTx(ctx context.Context) bool
}
