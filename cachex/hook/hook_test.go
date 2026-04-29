package hook

import (
	"context"
	"sync"
	"testing"
)

func TestUnit_MetricsHook_ConcurrentCount(t *testing.T) {
	m := &MetricsHook{}
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			m.AfterQuery(context.Background(), "k", i%2 == 0)
		}(i)
	}
	wg.Wait()
	hit, total := m.Snapshot()
	if total != N {
		t.Fatalf("total = %d, want %d", total, N)
	}
	if hit != N/2 {
		t.Fatalf("hit = %d, want %d", hit, N/2)
	}
}

func TestUnit_FuncHook_NilFns(t *testing.T) {
	h := FuncHook{}
	ctx := context.Background()
	// 全部 nil 不应 panic
	h.BeforeQuery(ctx, "k")
	h.AfterQuery(ctx, "k", true)
	h.AfterUpdate(ctx, "k")
	h.AfterListInvalidate(ctx, "t")
}

func TestUnit_BaseHook_NoOp(t *testing.T) {
	h := BaseHook{}
	ctx := context.Background()
	h.BeforeQuery(ctx, "k")
	h.AfterQuery(ctx, "k", false)
	h.AfterUpdate(ctx, "k")
	h.AfterListInvalidate(ctx, "t")
}

func TestUnit_LogHook_Smoke(t *testing.T) {
	h := &LogHook{}
	ctx := context.Background()
	h.BeforeQuery(ctx, "k")
	h.AfterQuery(ctx, "k", true)
	h.AfterUpdate(ctx, "k")
	h.AfterListInvalidate(ctx, "t")
}

