package test

import (
	"cachex/cachex/hook"
	"cachex/cachex/tx"
	"cachex/log"
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"cachex/cachex/cache"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type User struct {
	ID     int64 `gorm:"primaryKey"`
	Name   string
	Email  string `gorm:"unique"`
	Status string
}

func (User) TableName() string  { return "users" }
func (User) PrimaryKey() string { return "id" }
func (u User) GetPK() string {
	return strconv.FormatInt(u.ID, 10)
}

var (
	db      *gorm.DB
	cacheDB *cache.CacheDB
	ctx           = context.Background()
	userID  int64 = 1
)

func init() {
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
		EnableL1Cache: false,
	})
	cacheDB.UseHook(&hook.LogHook{})
	// ===== 初始化数据 =====
	//db.AutoMigrate(&User{})
	//db.Exec("DELETE FROM users")
	//
	//u := User{ID: userID, Name: "Alice", Email: "gege", Status: "active"}
	//db.Create(&u)

	// 清缓存
	_ = rdb.FlushDB(ctx).Err()
}

func Test_Update_InvalidateCache(t *testing.T) {
	log.SetReqId("log")
	ctx := context.Background()

	_, err := cache.NewModel[User](cacheDB).WithContext(ctx).
		Update(1, map[string]interface{}{
			"name": "updated",
		}, nil)

	require.NoError(t, err)

	var u *User
	u, err = cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(1)
	require.NoError(t, err)

	require.Equal(t, "updated", u.Name)
}
func Test_CachePenetration(t *testing.T) {
	log.SetReqId("log")

	ctx := context.Background()

	// 第一次：DB miss
	_, err := cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(99999)
	require.Error(t, err)

	// 第二次：应该命中 __null__
	start := time.Now()
	_, err = cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(99999)

	t.Log("null cache latency:", time.Since(start))
}
func Test_CacheBreakdown(t *testing.T) {
	log.SetReqId("log")
	ctx := context.Background()

	var wg sync.WaitGroup
	concurrency := 100

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(1)
		}()
	}

	wg.Wait()

	t.Log("cost:", time.Since(start))
}
