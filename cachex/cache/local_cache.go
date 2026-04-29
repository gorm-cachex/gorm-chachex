package cache

import (
	"sync"
	"time"
)

type l1Item struct {
	val      interface{}
	expireAt int64
}

type L1Cache struct {
	data sync.Map
}

func (c *L1Cache) Get(key string) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	v, ok := c.data.Load(key)
	if !ok {
		return nil, false
	}

	item := v.(*l1Item)

	if time.Now().UnixNano() > item.expireAt {
		c.data.Delete(key)
		return nil, false
	}

	return item.val, true
}

func (c *L1Cache) Set(key string, val interface{}, ttl time.Duration) {
	if c == nil {
		return
	}
	c.data.Store(key, &l1Item{
		val:      val,
		expireAt: time.Now().Add(ttl).UnixNano(),
	})
}

func (c *L1Cache) Delete(key string) {
	if c == nil {
		return
	}
	c.data.Delete(key)
}
