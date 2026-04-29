package cache

import (
	"cachex/cachex/errors"
	"cachex/log"
	"database/sql"
	errors2 "errors"
	"fmt"

	"gorm.io/gorm"
)

// 复杂查询，但是很常见的，可以使用组合键缓存，比如 where a=1 and b=2
type CacheCondition struct {
	Field string
	Op    CondOp
	Value interface{}
}
type CondOp string

const (
	OpEq   CondOp = "="    // 可缓存
	OpGt   CondOp = ">"    // 不缓存
	OpGte  CondOp = ">="   // 不缓存
	OpLt   CondOp = "<"    // 不缓存
	OpLte  CondOp = "<="   // 不缓存
	OpIn   CondOp = "IN"   // 不缓存
	OpLike CondOp = "LIKE" // 不缓存
)

func IsCacheableConds(conds []CacheCondition) bool {
	for _, c := range conds {
		if c.Op == "" {
			c.Op = OpEq
		}
		if c.Op != OpEq {
			return false
		}
	}
	return true
}

//var fieldRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func ApplyConds(db *gorm.DB, conds []CacheCondition) (*gorm.DB, error) {
	for _, c := range conds {
		if err := validateField(c.Field); err != nil {
			return nil, err
		}
		switch c.Op {
		case OpEq:
			db = db.Where(c.Field+" = ?", c.Value)
		case OpGt, OpGte, OpLt, OpLte, OpLike:
			db = db.Where(c.Field+" "+string(c.Op)+" ?", c.Value)
		case OpIn:
			db = db.Where(c.Field+" IN ?", c.Value)
		default:
			log.Errorw("unsupported op", "op", string(c.Op))

			return nil, errors.ErrUnsupportedOp
		}
	}
	return db, nil
}

type OrderDirection string

const (
	OrderAsc  OrderDirection = "asc"
	OrderDesc OrderDirection = "desc"
)

func (b *BaseModel[T]) getByConditionsFromDB(conds []CacheCondition) (*T, error) {
	query := b.Db.WithContext(b.ctx).Table(b.table)

	for _, c := range conds {
		query = query.Where(fmt.Sprintf("%s = ?", c.Field), c.Value)
	}

	var entity T
	err := query.First(&entity).Error
	if err != nil {
		if errors2.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		log.Errorw("get by conds db failed", "op", "getByConditionsFromDB", "err", err.Error())
		return nil, err
	}

	return &entity, nil
}
func (b *BaseModel[T]) getPKByConditions(conds []CacheCondition) (string, error) {
	query := b.Db.WithContext(b.ctx).Table(b.table).Select(b.PrimaryKey)

	for _, c := range conds {
		query = query.Where(fmt.Sprintf("%s = ?", c.Field), c.Value)
	}

	var raw sql.NullString
	err := query.Scan(&raw).Error
	if err != nil {
		log.Error(err)
		return "", err
	}
	if !raw.Valid {
		log.Error(gorm.ErrRecordNotFound)
		return "", gorm.ErrRecordNotFound
	}
	return raw.String, nil
}
