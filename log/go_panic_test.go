package log

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUnit_Go_RecoversPanic_NoCrash(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-panic")

	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	SetReqId("req-panic")

	// 用 callback 同步：onPanic 在所有 panic 日志写完之后才被调用，
	// 比 wg.Done() 在 fn 内 defer 更可靠。
	done := make(chan struct{})
	GoSafe(func() {
		panic(errors.New("boom"))
	}, func(err interface{}) {
		close(done)
	})
	<-done

	out := buf.String()
	if !strings.Contains(out, "PROCESS PANIC") {
		t.Fatalf("missing panic header: %s", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("missing panic message: %s", out)
	}
	if !strings.Contains(out, "[req-panic.1]") {
		t.Fatalf("panic log should include child reqId: %s", out)
	}
	if !strings.Contains(out, "dump stack") {
		t.Fatalf("missing stack header: %s", out)
	}
}

func TestUnit_GoSafe_CallbackInvoked(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-cb")

	captureMu.Lock()
	defer captureMu.Unlock()
	_, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	SetReqId("req-cb")

	var got atomic.Value
	var done sync.WaitGroup
	done.Add(1)
	GoSafe(func() {
		panic("oops")
	}, func(err interface{}) {
		got.Store(err)
		done.Done()
	})
	done.Wait()

	if v, ok := got.Load().(string); !ok || v != "oops" {
		t.Fatalf("callback got %v", got.Load())
	}
}

func TestUnit_Go_NoPanic_NoNoise(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-quiet")

	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	SetReqId("req-quiet")
	var done sync.WaitGroup
	done.Add(1)
	Go(func() {
		defer done.Done()
		// 正常退出，不应有 PANIC 日志
	})
	done.Wait()

	if strings.Contains(buf.String(), "PROCESS PANIC") {
		t.Fatalf("clean exit should not log panic: %s", buf.String())
	}
}

// TestDemo_GoPanicRecovery 演示 Go() 自动 recover panic 的输出。
//
// 运行：
//
//	go test -v -run TestDemo_GoPanicRecovery ./log/...
func TestDemo_GoPanicRecovery(t *testing.T) {
	defer ClearReqId()
	defer ResetChildCounter("req-demo-panic")

	prev := GetLogLevel()
	defer SetLogLevel(prev)
	SetLogLevel(DebugLevel)
	SetOutput(nil)

	SetReqId("req-demo-panic")
	Infow("parent: dispatching workers")

	var wg sync.WaitGroup
	for i := 1; i <= 2; i++ {
		i := i
		wg.Add(1)
		GoSafe(func() {
			defer wg.Done()
			Infow("worker running", "idx", i)
			if i == 2 {
				panic("simulated worker crash")
			}
			Infow("worker finished ok", "idx", i)
		}, func(err interface{}) {
			Errorw("alert: worker crashed", "idx", i, "err", err)
		})
	}
	wg.Wait()

	Infow("parent: all workers joined (process is alive)")
}

