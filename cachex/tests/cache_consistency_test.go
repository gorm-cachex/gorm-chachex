//go:build integration

package tests

import (
	"cachex/cachex/cache"
	"cachex/cachex/hook"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/stretchr/testify"
	"github.com/stretchr/testify/require"
)

func Test_Singleflight(t *testing.T) {
	ctx := context.Background()

	var wg sync.WaitGroup
	var dbHit int64

	// hook 统计 DB 命中
	cacheDB.Hooks = []hook.Hook{
		hook.FuncHook{
			AfterQueryFn: func(ctx context.Context, key string, hit bool) {
				if !hit {
					atomic.AddInt64(&dbHit, 1)
				}
			},
		},
	}

	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			userModel := cache.NewModel[User](cacheDB).WithContext(ctx)
			_, _ = userModel.GetByPK(1)
		}()
	}

	wg.Wait()

	t.Log("db hit count:", dbHit)

	// 理想情况：只有 1 次 DB
	require.LessOrEqual(t, dbHit, int64(2))
}
func Test_NullCache(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	_, err := userModel.GetByPK(999999)
	require.Error(t, err)

	start := time.Now()

	// 第二次应该直接命中 __null__
	_, err = userModel.GetByPK(999999)
	elapsed := time.Since(start)

	t.Log("null cache cost:", elapsed)
	require.Error(t, err)
	require.Less(t, elapsed.Microseconds(), int64(1000))
}
func Test_ColdStart_Breakdown(t *testing.T) {
	ctx := context.Background()
	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)

	var wg sync.WaitGroup
	wg.Add(500)

	for i := 0; i < 500; i++ {
		go func() {
			defer wg.Done()
			_, _ = userModel.GetByPK(1)
		}()
	}

	wg.Wait()
}
func Test_Transaction_Consistency(t *testing.T) {
	ctx := context.Background()

	err := cacheDB.Transaction(ctx, func(ctx context.Context) error {
		userModel := cache.NewModel[User](cacheDB).WithContext(ctx)
		_, err := userModel.Update(1, map[string]interface{}{
			"name": "tx_update",
		}, nil)
		return err
	})
	require.NoError(t, err)

	userModel := cache.NewModel[User](cacheDB).WithContext(ctx)
	u, err := userModel.GetByPK(1)
	require.NoError(t, err)
	require.Equal(t, "tx_update", u.Name)
}
