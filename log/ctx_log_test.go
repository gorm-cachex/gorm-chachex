package log

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUnit_InfofCtx_UsesCtxReqId(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	ctx := context.WithValue(context.Background(), ReqIDKey, "req-ctx-1")
	InfofCtx(ctx, "hello %s", "world")
	out := buf.String()
	if !strings.Contains(out, "[req-ctx-1]") || !strings.Contains(out, "hello world") {
		t.Fatalf("InfofCtx missing reqId/msg: %s", out)
	}
}

func TestUnit_InfowCtx_StructuredWithCtxReqId(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	ctx := context.WithValue(context.Background(), ReqIDKey, "req-ctx-2")
	InfowCtx(ctx, "cache hit", "key", "users:pk:1", "layer", "redis")
	out := buf.String()
	if !strings.Contains(out, "[req-ctx-2]") ||
		!strings.Contains(out, "key=users:pk:1") ||
		!strings.Contains(out, "layer=redis") {
		t.Fatalf("InfowCtx output wrong: %s", out)
	}
}

func TestUnit_Ctx_FallsBackToGLS(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	SetReqId("req-gls")
	defer ClearReqId()

	// ctx 无 reqId → 回退到 GLS 的 req-gls
	InfowCtx(context.Background(), "msg")
	if !strings.Contains(buf.String(), "[req-gls]") {
		t.Fatalf("expected GLS fallback reqId, got: %s", buf.String())
	}
}

func TestUnit_Ctx_NilCtxFallsBackToGLS(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	SetReqId("req-gls-nil")
	defer ClearReqId()

	//lint:ignore SA1012 intentionally testing nil ctx safety
	InfofCtx(nil, "nil ctx safe")
	out := buf.String()
	if !strings.Contains(out, "[req-gls-nil]") || !strings.Contains(out, "nil ctx safe") {
		t.Fatalf("nil ctx handling wrong: %s", out)
	}
}

func TestUnit_DebugwCtx_FilteredByLevel(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()

	SetLogLevel(InfoLevel)
	DebugwCtx(context.Background(), "should not appear", "k", "v")
	if strings.Contains(buf.String(), "should not appear") {
		t.Fatalf("debug ctx should be filtered: %s", buf.String())
	}
}

func TestUnit_FormatTime_CacheStable(t *testing.T) {
	// 同一秒内多次调用应返回一致的格式化结果，且不触发 race（-race 下运行验证）。
	base := time.Now()
	first := formatTime(base)
	for i := 0; i < 100; i++ {
		if got := formatTime(base); got != first {
			t.Fatalf("formatTime unstable within same second: %q vs %q", first, got)
		}
	}
}

func BenchmarkFormatLog(b *testing.B) {
	SetLogLevel(DebugLevel)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = formatLogReq(InfoLevel, "req-bench", "benchmark message key=value", 2)
	}
}
