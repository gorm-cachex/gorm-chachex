package log

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
)

// captureOutput 临时把 log 输出指向 bytes.Buffer，并返回读取函数。
func captureOutput(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	buf := &bytes.Buffer{}
	prevLevel := level
	SetOutput(buf)
	return buf, func() {
		SetOutput(nil)
		SetLogLevel(prevLevel)
	}
}

var captureMu sync.Mutex

func TestUnit_Debugw_Filtered(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()

	SetLogLevel(InfoLevel)
	Debugw("should not appear", "k", "v")
	if strings.Contains(buf.String(), "should not appear") {
		t.Fatalf("debug should be filtered out, got: %s", buf.String())
	}

	SetLogLevel(DebugLevel)
	Debugw("should appear", "k", "v")
	if !strings.Contains(buf.String(), "should appear") || !strings.Contains(buf.String(), "k=v") {
		t.Fatalf("debug missing, got: %s", buf.String())
	}
}

func TestUnit_FormatKV_OddArgs(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()

	SetLogLevel(DebugLevel)
	Infow("msg", "k1", "v1", "lonely")
	out := buf.String()
	if !strings.Contains(out, "k1=v1") || !strings.Contains(out, "_extra=lonely") {
		t.Fatalf("odd args not handled: %s", out)
	}
}

func TestUnit_FormatKV_FieldStruct(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()

	SetLogLevel(DebugLevel)
	Infow("msg", F("a", 1), F("b", "x"))
	out := buf.String()
	if !strings.Contains(out, "a=1") || !strings.Contains(out, "b=x") {
		t.Fatalf("Field args not handled: %s", out)
	}
}

func TestUnit_BindCtx_ReleaseClears(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	_, restore := captureOutput(t)
	defer restore()

	ctx := context.WithValue(context.Background(), ReqIDKey, "req-xyz")
	release := BindCtx(ctx)
	if got := GetReqId(); got != "req-xyz" {
		t.Fatalf("GetReqId after Bind = %q", got)
	}
	release()
	if got := GetReqId(); got != "" {
		t.Fatalf("GetReqId after release = %q", got)
	}
}

func TestUnit_BindCtx_NilCtx(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	_, restore := captureOutput(t)
	defer restore()

	release := BindCtx(nil)
	release() // 不应 panic
}

func TestUnit_Warnw_Output(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)
	Warnw("warn-msg", "k", "v")
	if !strings.Contains(buf.String(), "warn-msg") || !strings.Contains(buf.String(), "k=v") {
		t.Fatalf("warnw output missing: %s", buf.String())
	}
}

func TestUnit_Errorw_OutputContainsKV(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	Errorw("redis set failed", "key", "users:pk:1", "err", "boom")
	out := buf.String()
	if !strings.Contains(out, "key=users:pk:1") || !strings.Contains(out, "err=boom") {
		t.Fatalf("structured fields missing: %s", out)
	}
}

