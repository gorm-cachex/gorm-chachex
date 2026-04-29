package cache

// 更新选项
type UpdateOption struct {
	ExpectedVersion *int64
	Conds           []CacheCondition // 更新条件
}
type UpdateResult struct {
	RowsAffected int64
	Updated      bool
}

type Limits struct {
	MaxPageSize   int
	MaxConditions int
	MaxRedisBatch int
	MaxDBIn       int
}
type ListCacheValue struct {
	PKs   []string `json:"pks"`
	Total int64    `json:"total"`
}
var defaultLimits = Limits{
	MaxPageSize:   100,
	MaxConditions: 10,
	MaxRedisBatch: 50,
	MaxDBIn:       100,
}