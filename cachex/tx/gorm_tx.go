package tx

import (
	"context"

	"cachex/log"
	"gorm.io/gorm"
)

//func (b *BaseModel) AfterCommit() {
//	for _, fn := range b.TxManager.AfterCommit {
//		fn()
//	}
//}

type txKeyType struct{}

type TxContext struct {
	DB          *gorm.DB
	AfterCommit []func()
}

func withTx(ctx context.Context, tx *TxContext) context.Context {
	return context.WithValue(ctx, txKeyType{}, tx)
}

func GetTx(ctx context.Context) *TxContext {
	v := ctx.Value(txKeyType{})
	if v == nil {
		return nil
	}
	return v.(*TxContext)
}

type DefaultTxManager struct{}

func (m *DefaultTxManager) Begin(ctx context.Context, db *gorm.DB) (context.Context, error) {
	tx := db.Begin()
	if tx.Error != nil {
		return ctx, tx.Error
	}

	txCtx := &TxContext{
		DB: tx,
	}

	ctx = withTx(ctx, txCtx)

	return ctx, nil
}
func (m *DefaultTxManager) Commit(ctx context.Context) error {
	txCtx := GetTx(ctx)
	if txCtx == nil {
		return nil
	}

	if err := txCtx.DB.Commit().Error; err != nil {
		return err
	}

	// ⭐ commit 成功后执行；逐项 recover，记录后再 panic，
	// 保证既有"panic 冒泡"语义不变，同时可观测。
	for _, fn := range txCtx.AfterCommit {
		runAfterCommit(fn)
	}

	return nil
}

func runAfterCommit(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorw("after commit panic", "recover", r)
			panic(r)
		}
	}()
	fn()
}
func (m *DefaultTxManager) Rollback(ctx context.Context) error {
	txCtx := GetTx(ctx)
	if txCtx == nil {
		return nil
	}
	return txCtx.DB.Rollback().Error
}
func (m *DefaultTxManager) AfterCommit(ctx context.Context, fn func()) {
	txCtx := GetTx(ctx)
	if txCtx == nil {
		// 非事务：直接执行
		fn()
		return
	}

	txCtx.AfterCommit = append(txCtx.AfterCommit, fn)
}
func (m *DefaultTxManager) InTx(ctx context.Context) bool {
	return GetTx(ctx) != nil
}
