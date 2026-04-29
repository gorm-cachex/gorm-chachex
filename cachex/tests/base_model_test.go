//go:build integration

package tests

import (
	"cachex/cachex/cache"
	"cachex/cachex/tx"
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	// 初始化 DB / Redis / CacheDB
	initTestEnv()

	os.Exit(m.Run())
}

type User struct {
	ID     int64 `gorm:"primaryKey"`
	Name   string
	Email  string `gorm:"unique"`
	Status string
}

func (u User) GetPK() string {
	return strconv.FormatInt(u.ID, 10)
}
func (User) TableName() string  { return "users" }
func (User) PrimaryKey() string { return "id" }

var (
	db      *gorm.DB
	cacheDB *cache.CacheDB
	ctx           = context.Background()
	userID  int64 = 1
)

func initTestEnv() {
	// ===== DB =====
	dsn := "root:123.comM@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	// ===== Redis =====
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",

		PoolSize:     100,
		MinIdleConns: 10,
	})

	// ===== cachex =====

	cacheDB = cache.NewCacheDb(&cache.CacheDB{
		DB:    db,
		Cache: &cache.RedisCache{Client: rdb},
		//	Router:    initHashRing(db),
		TxManager:     &tx.DefaultTxManager{},
		Timeout:       time.Minute * 10,
		Seed:          500,
		EnableL1Cache: true,
	})

	// 清缓存
	_ = rdb.FlushDB(ctx).Err()
}

func Test_GetByPK_CacheHit(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	// 第一次访问（写入缓存）
	user, err := userModel.GetByPK(1)
	require.NoError(t, err)
	require.NotNil(t, user)

	// 第二次访问（缓存命中）
	start := time.Now()
	user2, err := userModel.GetByPK(1)
	require.NoError(t, err)
	require.NotNil(t, user2)
	require.Equal(t, user.ID, user2.ID)

	t.Log("cache hit cost:", time.Since(start))
	require.Less(t, time.Since(start).Microseconds(), int64(1000))
}
func Test_GetByUnique_CacheHit(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	// 假设 email 是唯一键
	user, err := userModel.GetByUnique("email", "test@example.com")
	require.NoError(t, err)
	require.NotNil(t, user)

	// 再访问，走缓存
	start := time.Now()
	user2, err := userModel.GetByUnique("email", "test@example.com")
	require.NoError(t, err)
	require.Equal(t, user.ID, user2.ID)

	t.Log("unique cache hit cost:", time.Since(start))
}
func Test_GetByConditions_CacheHit(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	conds := []cache.CacheCondition{
		{Field: "status", Value: "active"},
	}

	user, err := userModel.GetByConditions(conds)
	require.NoError(t, err)
	require.NotNil(t, user)

	start := time.Now()
	user2, err := userModel.GetByConditions(conds)
	require.NoError(t, err)
	require.Equal(t, user.ID, user2.ID)

	t.Log("conditions cache hit cost:", time.Since(start))
}
func Test_GetList_CacheHit(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	conds := []cache.CacheCondition{
		{Field: "status", Op: "=", Value: "active"},
	}
	page, pageSize := 1, 10

	users, total, err := userModel.GetList(conds, page, pageSize, nil)
	require.NoError(t, err)
	require.NotEmpty(t, users)
	require.GreaterOrEqual(t, total, int64(len(users)))

	// 再次访问，走缓存
	start := time.Now()
	users2, total2, err := userModel.GetList(conds, page, pageSize, nil)
	require.NoError(t, err)
	require.Equal(t, total, total2)
	require.Equal(t, len(users), len(users2))

	t.Log("list cache hit cost:", time.Since(start))
}
