package errors

import "errors"

var (
	ErrNotFound          = errors.New("record not found")
	ErrUnsafeQuery       = errors.New("unsafe query")
	ErrPageSizeTooLarge  = errors.New("page size too large")
	ErrInvalidOrder      = errors.New("invalid order by clause")
	ErrTooManyConditions = errors.New("too many conditions")
	ErrCacheCorrupted    = errors.New("cache corrupted")
	ErrUnsupportedOp     = errors.New("unsupported operation")
	ErrInvalidField      = errors.New("invalid field")
	RowsAffected         = errors.New("no rows affected")
)
