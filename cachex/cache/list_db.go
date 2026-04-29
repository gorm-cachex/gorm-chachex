package cache

import (
	"cachex/log"
	"time"
)

// getListFromDB 直接从 DB 加载完整实体列表（用于事务 / 不可缓存场景）。
func (b *BaseModel[T]) getListFromDB(
	conds []CacheCondition,
	page int,
	pageSize int,
	order OrderClause,
) ([]T, int64, error) {

	query := b.Db.WithContext(b.ctx).Model(new(T))

	var err error
	query, err = ApplyConds(query, conds)
	if err != nil {
		log.Errorw("apply conds failed", "op", "getListFromDB", "err", err.Error())
		return nil, 0, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Errorw("count failed", "op", "getListFromDB", "err", err.Error())
		return nil, 0, err
	}

	if total == 0 {
		return []T{}, 0, nil
	}

	var list []T

	start := time.Now()
	err = order.Apply(
		query.Offset((page - 1) * pageSize).
			Limit(pageSize),
	).Find(&list).Error

	if err != nil {
		log.Errorw("find failed", "op", "getListFromDB", "err", err.Error())
		return nil, 0, err
	}

	log.Debugw("list db query", "op", "getListFromDB", "page", page, "size", pageSize, "n", len(list), "total", total, "elapsed_ms", time.Since(start).Milliseconds())
	return list, total, nil
}

// getPKListFromDB 仅查询主键列表（用于列表缓存）。
func (b *BaseModel[T]) getPKListFromDB(
	conds []CacheCondition,
	page int,
	pageSize int,
	order OrderClause,
) (pks []string, total int64, err error) {

	query := b.Db.WithContext(b.ctx).Table(b.table)

	// 1. ️where 条件
	query, err = ApplyConds(query, conds)
	if err != nil {
		log.Errorw("apply conds failed", "op", "getPKListFromDB", "err", err.Error())
		return nil, 0, err
	}

	// 2. total
	if err = query.Count(&total).Error; err != nil {
		log.Errorw("count failed", "op", "getPKListFromDB", "err", err.Error())
		return
	}
	if total == 0 {
		return
	}

	// 3. 只查主键
	query = query.
		Select(b.PrimaryKey).
		Order(order).
		Offset((page - 1) * pageSize).
		Limit(pageSize)
	start := time.Now()
	rows, err := order.Apply(query).
		Rows()
	if err != nil {
		log.Errorw("rows failed", "op", "getPKListFromDB", "err", err.Error())
		return
	}
	defer rows.Close()

	// 4. scan PK
	for rows.Next() {
		var pk string
		if err = rows.Scan(&pk); err != nil {
			return
		}
		pks = append(pks, pk)
	}

	log.Debugw("list pk db query", "op", "getPKListFromDB", "page", page, "size", pageSize, "n", len(pks), "total", total, "elapsed_ms", time.Since(start).Milliseconds())
	return
}
