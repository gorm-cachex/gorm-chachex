package log

import (
	"strings"
	"testing"
)

// 这些测试覆盖既有 Info/Warn/Error/Debug 等旧 API，确保结构化扩展不破坏向后兼容。

func TestUnit_LegacyAPIs_Smoke(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	buf, restore := captureOutput(t)
	defer restore()
	SetLogLevel(DebugLevel)

	Info("info-msg")
	Infof("info-%s", "fmt")
	Warn("warn-msg")
	Warnf("warn-%s", "fmt")
	Error("err-msg")
	Errorf("err-%s", "fmt")
	Debug("dbg-msg")
	Debugf("dbg-%s", "fmt")

	out := buf.String()
	for _, s := range []string{"info-msg", "info-fmt", "warn-msg", "warn-fmt", "err-msg", "err-fmt", "dbg-msg", "dbg-fmt"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in output: %s", s, out)
		}
	}
}

func TestUnit_Must_Nil(t *testing.T) {
	// 不应 panic / Fatal
	Must(nil)
}

func TestUnit_LevelString(t *testing.T) {
	cases := []struct {
		l    Level
		want string
	}{
		{DebugLevel, "debug"},
		{InfoLevel, "info"},
		{WarnLevel, "warn"},
		{ErrorLevel, "error"},
		{FatalLevel, "fatal"},
	}
	for _, c := range cases {
		if got := c.l.String(); got != c.want {
			t.Fatalf("Level(%d).String()=%q want %q", c.l, got, c.want)
		}
		if c.l.ShortString() == "" {
			t.Fatalf("ShortString empty")
		}
		if c.l.Color() == "" {
			t.Fatalf("Color empty")
		}
	}
}

func TestUnit_ReqIDStore(t *testing.T) {
	SetReqId("abc")
	if got := GetReqId(); got != "abc" {
		t.Fatalf("GetReqId=%q", got)
	}
	ClearReqId()
	if got := GetReqId(); got != "" {
		t.Fatalf("after Clear=%q", got)
	}
}

// TestDemo_LogOutput 是一个"演示型"测试：不做断言，仅把 log 各种场景的真实输出打到 stdout，
// 方便用 `go test -v -run TestDemo_LogOutput ./log/...` 直观查看日志格式与色彩。
//
// 运行：
//
//	go test -v -run TestDemo_LogOutput ./log/...
func TestDemo_LogOutput(t *testing.T) {
	// 不抢 captureMu：让日志直接打到 stdout，便于人眼观察。
	prev := GetLogLevel()
	defer SetLogLevel(prev)
	SetLogLevel(DebugLevel)
	SetOutput(nil) // 恢复 os.Stdout

	t.Log("===== 1) 旧 API（向后兼容） =====")
	Debug("旧 Debug：进入 GetByPK")
	Info("旧 Info：从 DB 加载实体 id=", 1)
	Warn("旧 Warn：list cache corrupted")
	Error("旧 Error：redis get failed err=", "connection refused")
	Infof("旧 Infof：cost=%dms key=%s", 23, "users:pk:1")

	t.Log("===== 2) 新结构化 API（推荐） =====")
	Debugw("cache hit",
		"op", "GetByPK",
		"key", "users:pk:1",
		"hit_layer", "l1",
	)
	Debugw("cache miss",
		"op", "GetByPK",
		"key", "users:pk:404",
		"layer", "redis",
	)
	Infow("db fallback done",
		"op", "GetByPK",
		"key", "users:pk:404",
		"elapsed_ms", 12,
		"err", "",
	)
	Warnw("update no rows affected",
		"op", "Update",
		"table", "users",
		"pk", 9999,
	)
	Errorw("redis set failed",
		"key", "users:pk:1",
		"ttl", "10m",
		"err", "i/o timeout",
	)

	t.Log("===== 3) Field 构造器 =====")
	Infow("list version bump",
		F("op", "Insert"),
		F("table", "users"),
	)

	t.Log("===== 4) 奇数参数容错（不会丢日志） =====")
	Infow("odd args demo", "k1", "v1", "lonely_value")

	t.Log("===== 5) req_id 自动透传 =====")
	SetReqId("req-demo-2026")
	Infow("with req id", "op", "GetByPK", "key", "users:pk:1")
	ClearReqId()
	Infow("without req id", "op", "GetByPK", "key", "users:pk:1")

	t.Log("===== 6) 级别过滤（生产级 InfoLevel） =====")
	SetLogLevel(InfoLevel)
	Debugw("这条 Debug 被过滤掉", "k", "v") // 不会出现
	Infow("这条 Info 仍然输出", "k", "v")
	SetLogLevel(DebugLevel)
}

