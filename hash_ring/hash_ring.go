package dbrouter

import (
	"context"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"

	"gorm.io/gorm"
)

const VirtualNodeNum = 100

type HashRing struct {
	virtualNodes int
	hashKeys     []uint32
	hashMap      map[uint32]int // hash值到实例索引
	dbs          []*gorm.DB
	lock         sync.RWMutex
}

func NewHashRing(dbs []*gorm.DB, virtualNodes int) *HashRing {
	ring := &HashRing{
		virtualNodes: virtualNodes,
		hashMap:      make(map[uint32]int),
		dbs:          dbs,
	}

	ring.generateRing()
	return ring
}

func (h *HashRing) generateRing() {
	h.lock.Lock()
	defer h.lock.Unlock()

	for i := range h.dbs {
		for v := 0; v < h.virtualNodes; v++ {
			// 每个节点生成多个虚拟节点
			hashKey := hashKey("DB-" + strconv.Itoa(i) + "-VN-" + strconv.Itoa(v))
			h.hashKeys = append(h.hashKeys, hashKey)
			h.hashMap[hashKey] = i
		}
	}
	sort.Slice(h.hashKeys, func(i, j int) bool {
		return h.hashKeys[i] < h.hashKeys[j]
	})
}

func hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// Get returns the *gorm.DB instance for a given key
func (h *HashRing) Route(ctx context.Context, key string, table string) (*gorm.DB, string) {
	if key == "" {
		return nil, table
	}
	h.lock.RLock()
	defer h.lock.RUnlock()

	if len(h.hashKeys) == 0 {
		return nil, table + "_" + key
	}

	hash := hashKey(key)

	// 二分查找最近的 hash 值
	idx := sort.Search(len(h.hashKeys), func(i int) bool {
		return h.hashKeys[i] >= hash
	})
	if idx == len(h.hashKeys) {
		idx = 0
	}
	nodeIdx := h.hashMap[h.hashKeys[idx]]
	return h.dbs[nodeIdx], table + "_" + key
}
