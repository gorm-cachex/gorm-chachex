package cache

import (
	"cachex/cachex/errors"
	"regexp"
)

func validateField(field string) error {
	if !FieldRegexp.MatchString(field) {
		return errors.ErrInvalidField
	}
	return nil
}

var FieldRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func (c *CacheDB) ValidatePageSize(pageSize int) error {
	if pageSize <= 0 || pageSize > c.Limits.MaxPageSize {
		return errors.ErrPageSizeTooLarge
	}
	return nil
}
func (c *CacheDB) ValidateConditions(conds []CacheCondition) error {
	if len(conds) > c.Limits.MaxConditions {
		return errors.ErrTooManyConditions
	}
	return nil
}
