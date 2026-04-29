package benchmark

import (
	"cachex/cachex/tx"
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"cachex/cachex/cache"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type User struct {
	Id   int64 `gorm:"primaryKey"`
	Name string
}

func (User) TableName() string  { return "users" }
func (User) PrimaryKey() string { return "id" }
func (u User) GetPK() string {
	return strconv.FormatInt(u.Id, 10)
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
		EnableL1Cache: true,
	})
	// ===== 初始化数据 =====
	//db.AutoMigrate(&User{})
	//db.Exec("DELETE FROM users")
	//
	//u := User{ID: userID, Name: "Alice", Email: "gege", Status: "active"}
	//db.Create(&u)
	// 清缓存
	_ = rdb.FlushDB(ctx).Err()
}

//
// ===== 1️⃣ 基础性能 =====
//

func Benchmark_DB(b *testing.B) {
	var u User
	for i := 0; i < b.N; i++ {
		db.First(&u, userID)
	}
}

func Benchmark_Cachex_Hit(b *testing.B) {

	// 预热缓存
	cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(userID)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(userID)
	}
}

//
// ===== 2️⃣ 并发性能 =====
//

func Benchmark_Parallel_DB(b *testing.B) {
	var wg sync.WaitGroup

	for i := 0; i < b.N; i++ {
		wg.Add(1000)

		start := time.Now()

		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				var u User
				db.First(&u, userID)
			}()
		}

		wg.Wait()
		b.Log("cost:", time.Since(start))
	}

}

func Benchmark_RealParallel_Cachex(b *testing.B) {
	var wg sync.WaitGroup

	for i := 0; i < b.N; i++ {
		wg.Add(1000)

		start := time.Now()

		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(userID)
			}()
		}

		wg.Wait()
		b.Log("cost:", time.Since(start))
	}
}

// DB Only
func Benchmark_Parallel_DB_Only(b *testing.B) {
	var u User
	for i := 0; i < b.N; i++ {
		db.First(&u, 1)
	}
}

// Redis Only
func Benchmark_Parallel_Redis(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cacheDB.Cache.Get(ctx, "users:pk:1")
	}
}
func Benchmark_Parallel_Official(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(1)

		}
	})
}

//
// ===== 3️⃣ 击穿测试（核心） =====
//

func Benchmark_Breakdown_DB(b *testing.B) {
	for i := 0; i < b.N; i++ {

		// 每次都直接打 DB
		var wg sync.WaitGroup
		wg.Add(1000)

		start := time.Now()

		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				var u User
				db.First(&u, userID)
			}()
		}

		wg.Wait()
		b.Log("DB breakdown cost:", time.Since(start))
	}
}

func Benchmark_Breakdown_Cachex(b *testing.B) {
	for i := 0; i < b.N; i++ {

		// 清缓存 → 模拟击穿
		_ = cacheDB.Cache.Del(ctx, "users:pk:1")

		var wg sync.WaitGroup
		wg.Add(1000)

		start := time.Now()

		for j := 0; j < 1000; j++ {
			go func() {
				defer wg.Done()
				cache.NewModel[User](cacheDB).WithContext(ctx).GetByPK(userID)

			}()
		}

		wg.Wait()
		b.Log("Cachex breakdown cost:", time.Since(start))
	}
}
