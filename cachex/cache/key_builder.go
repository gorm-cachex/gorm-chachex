package cache

import (
	"cachex/cachex/utils"
	"context"
	"fmt"
	"strconv"
	"strings"
)

// 主键缓存key
func (c *CacheDB) PkKey(id any, table string) string {
	return fmt.Sprintf("%s:pk:%s", table, utils.EncodeValue(id))
}

// 唯一键缓存key
func (c *CacheDB) UkKey(field string, value any, table string) string {
	return fmt.Sprintf("%s:uk:%s=%s", table, field, utils.EncodeValue(value))
}

// 组合条件缓存key
func (c *CacheDB) CompositeKey(
	conds []CacheCondition,
	table string,
) string {
	// default: 不排序，保持输入顺序，如果业务需要，可以开启排序，可能会导致联合索引失效，性能下降
	//cp := make([]CacheCondition, len(conds))
	//copy(cp, conds)

	//sort.Slice(cp, func(i, j int) bool {
	//	return cp[i].Field < cp[j].Field
	//})

	var b strings.Builder
	for _, c := range conds {
		b.WriteString(c.Field)
		b.WriteString("=")
		b.WriteString(utils.EncodeValue(c.Value))
		b.WriteString("|")
	}

	return fmt.Sprintf("%s:ck:%s", table, b.String())
}

func (c *CacheDB) ListKey(
	ctx context.Context,
	table string,
	conds []CacheCondition,
	order string,
	page int,
	pageSize int,
) string {
	v := c.getListVersion(ctx, table)

	// default: 不排序，保持输入顺序，如果业务需要，可以开启排序，可能会导致联合索引失效，性能下降
	//cp := make([]CacheCondition, len(conds))
	//copy(cp, conds)
	//
	//sort.Slice(cp, func(i, j int) bool {
	//	return cp[i].Field < cp[j].Field
	//})

	// 拼接条件
	var sb strings.Builder
	sb.WriteString("list:")
	sb.WriteString(table)
	sb.WriteString(fmt.Sprintf("%d", v))
	sb.WriteString(":")

	for i, cond := range conds {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(cond.Field)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", cond.Value))
	}

	// order / page 信息
	sb.WriteString("|order:")
	sb.WriteString(order)

	sb.WriteString("|page:")
	sb.WriteString(strconv.Itoa(page))

	sb.WriteString("|size:")
	sb.WriteString(strconv.Itoa(pageSize))

	return sb.String()
}
