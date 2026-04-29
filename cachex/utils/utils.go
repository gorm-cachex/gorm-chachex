package utils

import (
	"database/sql"
	"fmt"
	"time"
)

func EncodeValue(v interface{}) string {
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case fmt.Stringer:
		return x.String()
	case sql.NullString:
		if !x.Valid {
			return ""
		}
		return x.String
	case *sql.NullString:
		if x == nil || !x.Valid {
			return ""
		}
		return x.String
	default:
		return fmt.Sprintf("%v", x)
	}
}
