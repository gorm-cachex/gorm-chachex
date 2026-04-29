package router

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type Router interface {
	Route(ctx context.Context, key string, table string) (*gorm.DB, string)
}

type ModRouter struct {
	DB *gorm.DB
}

func (r *ModRouter) Route(ctx context.Context, key string, table string) (*gorm.DB, string) {
	_ = ctx
	id, _ := strconv.Atoi(key)
	suffix := id % 64
	return r.DB, fmt.Sprintf("%s_%02d", table, suffix)
}

type TimeRouter struct {
	DB *gorm.DB
}

func (r *TimeRouter) Route(ctx context.Context, key string, table string) (*gorm.DB, string) {
	_ = ctx
	t, _ := time.Parse("200601", key)
	return r.DB, fmt.Sprintf("%s_%d%02d", table, t.Year(), t.Month())
}

// 编译期接口断言：确保 ModRouter / TimeRouter 永远满足 Router 接口。
var (
	_ Router = (*ModRouter)(nil)
	_ Router = (*TimeRouter)(nil)
)

