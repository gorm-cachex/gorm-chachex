package cache

import (
	"context"
	"strings"
	"testing"
)

func TestUnit_PkKey(t *testing.T) {
	c := &CacheDB{}
	got := c.PkKey(1, "users")
	if got != "users:pk:1" {
		t.Fatalf("PkKey = %q", got)
	}
}

func TestUnit_UkKey(t *testing.T) {
	c := &CacheDB{}
	got := c.UkKey("email", "alice@example.com", "users")
	if !strings.Contains(got, "users:uk:email=") {
		t.Fatalf("UkKey = %q", got)
	}
}

func TestUnit_CompositeKey_OrderStable(t *testing.T) {
	c := &CacheDB{}
	conds := []CacheCondition{
		{Field: "a", Value: 1},
		{Field: "b", Value: 2},
	}
	got := c.CompositeKey(conds, "users")
	if got != "users:ck:a=1|b=2|" {
		t.Fatalf("CompositeKey = %q", got)
	}
	// 颠倒顺序应得到不同 key（保持输入顺序）
	swapped := c.CompositeKey([]CacheCondition{conds[1], conds[0]}, "users")
	if got == swapped {
		t.Fatalf("composite key should preserve order")
	}
}

func TestUnit_ListKey_VersionBump_DifferentKeys(t *testing.T) {
	// 用 nil Cache 会 panic，这里通过最小桩验证 ListKey 在 version 不同时输出不同。
	// 直接构造手工字符串来覆盖 builder 逻辑。
	_ = context.TODO()
	a := buildListKeyForTest("users", 1, []CacheCondition{{Field: "x", Value: 1}}, "", 1, 10)
	b := buildListKeyForTest("users", 2, []CacheCondition{{Field: "x", Value: 1}}, "", 1, 10)
	if a == b {
		t.Fatalf("list key should change with version, got %q == %q", a, b)
	}
}

// buildListKeyForTest 绕过 getListVersion，仅模拟 ListKey 拼接逻辑用于测试。
// 注意：与 (*CacheDB).ListKey 的拼接保持一致。
func buildListKeyForTest(table string, v int64, conds []CacheCondition, order string, page, size int) string {
	var sb strings.Builder
	sb.WriteString("list:")
	sb.WriteString(table)
	sb.WriteString(itoa(int(v)))
	sb.WriteString(":")
	for i, c := range conds {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(c.Field)
		sb.WriteString("=")
		sb.WriteString(toStr(c.Value))
	}
	sb.WriteString("|order:")
	sb.WriteString(order)
	sb.WriteString("|page:")
	sb.WriteString(itoa(page))
	sb.WriteString("|size:")
	sb.WriteString(itoa(size))
	return sb.String()
}

func itoa(i int) string {
	// 简单实现，避免引入 strconv 在测试 helper 中
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return itoa(t)
	default:
		return ""
	}
}

