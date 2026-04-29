package cache

import (
	"cachex/cachex/errors"
	"strings"

	"gorm.io/gorm"
)

type OrderItem struct {
	Field string
	Dir   OrderDirection
}
type OrderClause []OrderItem

func (o OrderClause) Validate() error {
	for _, item := range o {
		if !FieldRegexp.MatchString(item.Field) {
			return errors.ErrInvalidOrder
		}
		if item.Dir != OrderAsc && item.Dir != OrderDesc {
			return errors.ErrInvalidOrder
		}
	}
	return nil
}
func (o OrderClause) Apply(db *gorm.DB) *gorm.DB {
	for _, item := range o {
		db = db.Order(item.Field + " " + string(item.Dir))
	}
	return db
}
func (o OrderClause) String() string {
	var b strings.Builder
	for _, item := range o {
		b.WriteString(item.Field)
		b.WriteString(":")
		b.WriteString(string(item.Dir))
		b.WriteString("|")
	}
	return b.String()
}
