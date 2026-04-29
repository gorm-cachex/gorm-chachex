package cachex

import (
	"gorm.io/gorm"
)

type ShardingStrategy interface {
	GetDB(key string) *gorm.DB
	GetTable(key string, table string) string
}

type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}
