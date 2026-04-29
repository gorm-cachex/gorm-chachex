package hook

import "context"

type BaseHook struct{}

func (BaseHook) BeforeQuery(ctx context.Context, key string)           {}
func (BaseHook) AfterQuery(ctx context.Context, key string, hit bool)  {}
func (BaseHook) AfterUpdate(ctx context.Context, key string)           {}
func (BaseHook) AfterListInvalidate(ctx context.Context, table string) {}
