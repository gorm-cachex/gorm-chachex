package cache

import (
	"errors"
	"testing"

	cerr "cachex/cachex/errors"
)

func TestUnit_PageSize_Bounds(t *testing.T) {
	c := &CacheDB{Limits: defaultLimits}
	if err := c.ValidatePageSize(0); !errors.Is(err, cerr.ErrPageSizeTooLarge) {
		t.Fatalf("want ErrPageSizeTooLarge for 0, got %v", err)
	}
	if err := c.ValidatePageSize(c.Limits.MaxPageSize + 1); !errors.Is(err, cerr.ErrPageSizeTooLarge) {
		t.Fatalf("want ErrPageSizeTooLarge for > max, got %v", err)
	}
	if err := c.ValidatePageSize(10); err != nil {
		t.Fatalf("want nil for 10, got %v", err)
	}
}

func TestUnit_Conditions_TooMany(t *testing.T) {
	c := &CacheDB{Limits: defaultLimits}
	conds := make([]CacheCondition, c.Limits.MaxConditions+1)
	if err := c.ValidateConditions(conds); !errors.Is(err, cerr.ErrTooManyConditions) {
		t.Fatalf("want ErrTooManyConditions, got %v", err)
	}
}

