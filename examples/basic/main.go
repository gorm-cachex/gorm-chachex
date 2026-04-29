package main

import (
	"cachex/cachex/cache"
	_ "cachex/cachex/cache"
	"cachex/cachex/hook"
	"cachex/cachex/tx"
	dbrouter "cachex/hash_ring"
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ===== Model =====
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

// ===== Init DB =====
func initDB() *gorm.DB {
	dsn := "root:123.comM@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	_ = db.AutoMigrate(&User{})
	return db
}

// ===== Init HashRing =====
func initHashRing(db *gorm.DB) *dbrouter.HashRing {
	return dbrouter.NewHashRing([]*gorm.DB{db}, 100)
}

// ===== Main =====
func main() {
	ctx := context.Background()

	// ===== DB =====
	db := initDB()

	// ===== Redis =====
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})

	// ⭐ 清空缓存（保证测试准确）
	_ = rdb.FlushDB(ctx).Err()

	// ===== cachex =====

	cacheDB := cache.NewCacheDb(&cache.CacheDB{
		DB:            db,
		Cache:         &cache.RedisCache{Client: rdb},
		Router:        initHashRing(db),
		TxManager:     &tx.DefaultTxManager{},
		Timeout:       time.Minute * 10,
		Seed:          500,
		EnableL1Cache: true,
	})
	cacheDB.UseHook(&hook.LogHook{})
	// ===== Insert =====
	user := User{Name: "Alice", Email: "sister", Status: "active"}
	_ = cache.NewModel[User](cacheDB).Insert(&user)
	fmt.Println("insert id:", user.ID)

	// ===== First Query（DB）=====
	fmt.Println("=== First Query (DB expected) ===")
	var u1 *User
	start := time.Now()
	u1, _ = cache.NewModel[User](cacheDB).GetByPK(user.ID)
	fmt.Println("cost:", time.Since(start), *u1)

	// ===== Second Query（Cache）=====
	fmt.Println("=== Second Query (Cache expected) ===")
	var u2 *User
	start = time.Now()
	u2, _ = cache.NewModel[User](cacheDB).GetByPK(user.ID)
	fmt.Println("cost:", time.Since(start), *u2)

	// ===== Update =====
	fmt.Println("=== Update ===")
	_, _ = cache.NewModel[User](cacheDB).Update(user.ID, map[string]interface{}{
		"name": "Bob",
	}, nil)

	// ===== Query After Update =====
	fmt.Println("=== Query After Update (Cache Miss expected) ===")
	var u3 *User
	u3, _ = cache.NewModel[User](cacheDB).GetByPK(user.ID)
	fmt.Println(u3)

	// ===== Transaction =====
	fmt.Println("=== Transaction Update ===")
	err := cacheDB.Transaction(ctx, func(ctx context.Context) error {
		userModel := cache.NewModel[User](cacheDB).WithContext(ctx)
		_, err := userModel.Update(user.ID, map[string]interface{}{
			"name": "TxUser",
		}, nil)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	var u4 *User
	u4, _ = cache.NewModel[User](cacheDB).GetByPK(user.ID)
	fmt.Println("after tx:", *u4)
}
