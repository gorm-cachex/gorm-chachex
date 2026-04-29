package cache_test

// 共享测试夹具：构造 CacheDB（基于 sqlmock + miniredis），定义 User 模型。

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cachex/cachex/cache"
	"cachex/cachex/cache/mocks"
	"cachex/cachex/tx"
)

type User struct {
	ID     int64 `gorm:"primaryKey;column:id"`
	Name   string
	Email  string
	Status string
}

func (u User) GetPK() string       { return strconv.FormatInt(u.ID, 10) }
func (User) TableName() string     { return "users" }
func (User) PrimaryKey() string    { return "id" }

type fixture struct {
	cdb     *cache.CacheDB
	mock    sqlmock.Sqlmock
	mc      *mocks.MockCache
	rawSQL  *sql.DB
	teardown func()
}

func newFixture(t *testing.T, useNoTx bool, enableL1 bool) *fixture {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual), sqlmock.MonitorPingsOption(false))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: mockDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}

	mc := mocks.New(t)

	var txm tx.TxManager
	if useNoTx {
		txm = mocks.NoTxManager{}
	} else {
		txm = &tx.DefaultTxManager{}
	}

	cdb := cache.NewCacheDb(&cache.CacheDB{
		DB:               gdb,
		Cache:            mc,
		TxManager:        txm,
		Timeout:          time.Minute,
		Seed:             1,
		NullValueTimeout: time.Second,
		ListValueTimeout: time.Minute,
		EnableL1Cache:    enableL1,
	})

	return &fixture{
		cdb:     cdb,
		mock:    mock,
		mc:      mc,
		rawSQL:  mockDB,
		teardown: func() {
			_ = mockDB.Close()
			mc.Close()
		},
	}
}

func (f *fixture) ctx() context.Context { return context.Background() }

