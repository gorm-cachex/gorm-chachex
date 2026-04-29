package cache

import (
	"errors"
	"testing"

	cerr "cachex/cachex/errors"
)

func TestUnit_IsCacheableConds_OnlyEq(t *testing.T) {
	if !IsCacheableConds([]CacheCondition{{Field: "a", Op: OpEq, Value: 1}}) {
		t.Fatal("eq should be cacheable")
	}
	if IsCacheableConds([]CacheCondition{{Field: "a", Op: OpGt, Value: 1}}) {
		t.Fatal("gt should not be cacheable")
	}
}

func TestUnit_ApplyConds_InvalidField(t *testing.T) {
	_, err := ApplyConds(nil, []CacheCondition{{Field: "bad field;", Op: OpEq, Value: 1}})
	if !errors.Is(err, cerr.ErrInvalidField) {
		t.Fatalf("want ErrInvalidField, got %v", err)
	}
}

func TestUnit_ApplyConds_UnsupportedOp(t *testing.T) {
	_, err := ApplyConds(nil, []CacheCondition{{Field: "a", Op: "??", Value: 1}})
	if !errors.Is(err, cerr.ErrUnsupportedOp) {
		t.Fatalf("want ErrUnsupportedOp, got %v", err)
	}
}

