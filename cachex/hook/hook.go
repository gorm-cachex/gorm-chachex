package hook

import "context"

type Hook interface {
	BeforeQuery(ctx context.Context, key string)
	AfterQuery(ctx context.Context, key string, hit bool)
	AfterUpdate(ctx context.Context, key string)
	// 列表失效（版本变化）
	AfterListInvalidate(ctx context.Context, table string)
}
type FuncHook struct {
	BeforeQueryFn         func(ctx context.Context, key string)
	AfterQueryFn          func(ctx context.Context, key string, hit bool)
	AfterUpdateFn         func(ctx context.Context, key string)
	AfterListInvalidateFn func(ctx context.Context, table string)
}

func (f FuncHook) BeforeQuery(ctx context.Context, key string) {
	if f.BeforeQueryFn != nil {
		f.BeforeQueryFn(ctx, key)
	}
}

func (f FuncHook) AfterQuery(ctx context.Context, key string, hit bool) {
	if f.AfterQueryFn != nil {
		f.AfterQueryFn(ctx, key, hit)
	}
}

func (f FuncHook) AfterUpdate(ctx context.Context, key string) {
	if f.AfterUpdateFn != nil {
		f.AfterUpdateFn(ctx, key)
	}
}
func (f FuncHook) AfterListInvalidate(ctx context.Context, table string) {
	if f.AfterListInvalidateFn != nil {
		f.AfterListInvalidateFn(ctx, table)
	}
}
