package log

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUnit_Go_PropagatesChildReqID(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-parent")

	SetReqId("req-parent")

	var (
		mu       sync.Mutex
		children []string
		wg       sync.WaitGroup
	)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		Go(func() {
			defer wg.Done()
			id := GetReqId()
			mu.Lock()
			children = append(children, id)
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(children) != 3 {
		t.Fatalf("got %d children", len(children))
	}
	// 期望：req-parent.1, req-parent.2, req-parent.3（顺序不保证，但集合一致）
	got := map[string]bool{}
	for _, c := range children {
		got[c] = true
		if !strings.HasPrefix(c, "req-parent.") {
			t.Fatalf("child id without prefix: %q", c)
		}
	}
	for _, want := range []string{"req-parent.1", "req-parent.2", "req-parent.3"} {
		if !got[want] {
			t.Fatalf("missing child %q in %v", want, children)
		}
	}
}

func TestUnit_Go_NoParent_NoChild(t *testing.T) {
	defer ClearReqId()
	ClearReqId() // 确保父无 id

	var got atomic.Value
	got.Store("INIT")
	var wg sync.WaitGroup
	wg.Add(1)
	Go(func() {
		defer wg.Done()
		got.Store(GetReqId())
	})
	wg.Wait()
	if got.Load().(string) != "" {
		t.Fatalf("expected empty child id, got %q", got.Load())
	}
}

func TestUnit_Go_Nested_GrandchildID(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-root")
	defer ResetChildCounter("req-root.1")

	SetReqId("req-root")

	var grandchild string
	var wg sync.WaitGroup
	wg.Add(1)
	Go(func() {
		defer wg.Done()
		// 这里 reqId = req-root.1
		var inner sync.WaitGroup
		inner.Add(1)
		Go(func() {
			defer inner.Done()
			grandchild = GetReqId()
		})
		inner.Wait()
	})
	wg.Wait()

	if grandchild != "req-root.1.1" {
		t.Fatalf("grandchild = %q, want req-root.1.1", grandchild)
	}
}

func TestUnit_Go_AfterChildExit_ParentStillHasID(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-keep")

	SetReqId("req-keep")
	var wg sync.WaitGroup
	wg.Add(1)
	Go(func() {
		defer wg.Done()
		_ = GetReqId()
	})
	wg.Wait()
	if got := GetReqId(); got != "req-keep" {
		t.Fatalf("parent id lost: %q", got)
	}
}

// TestDemo_GoChildReqID 演示父子协程 reqId 关联。
//
// 运行：
//
//	go test -v -run TestDemo_GoChildReqID ./log/...
func TestDemo_GoChildReqID(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-demo-go")

	prev := GetLogLevel()
	defer SetLogLevel(prev)
	SetLogLevel(DebugLevel)
	SetOutput(nil)

	SetReqId("req-demo-go")
	Infow("parent task", "step", "start")

	var wg sync.WaitGroup
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		Go(func() {
			defer wg.Done()
			Infow("child task", "step", "running")

			var inner sync.WaitGroup
			inner.Add(1)
			Go(func() {
				defer inner.Done()
				Infow("grandchild task", "step", "leaf")
			})
			inner.Wait()
		})
	}
	wg.Wait()

	Infow("parent task", "step", "done")
}

