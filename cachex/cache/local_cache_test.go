package cache

import (
	"testing"
	"time"
)

func TestUnit_L1_NilSafe(t *testing.T) {
	var c *L1Cache
	// 不应 panic
	if v, ok := c.Get("x"); ok || v != nil {
		t.Fatalf("nil L1.Get should miss")
	}
	c.Set("x", 1, time.Second)
	c.Delete("x")
}

func TestUnit_L1_SetGet(t *testing.T) {
	c := &L1Cache{}
	c.Set("k", "v", time.Second)
	v, ok := c.Get("k")
	if !ok || v.(string) != "v" {
		t.Fatalf("expected hit, got ok=%v v=%v", ok, v)
	}
}

func TestUnit_L1_Expire(t *testing.T) {
	c := &L1Cache{}
	c.Set("k", "v", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatalf("expected miss after expire")
	}
}

func TestUnit_L1_Delete(t *testing.T) {
	c := &L1Cache{}
	c.Set("k", "v", time.Second)
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatalf("expected miss after delete")
	}
}

