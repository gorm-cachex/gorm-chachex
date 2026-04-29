package log

import (
	"fmt"
	"strings"
)

// Field 是结构化日志的键值对。
type Field struct {
	Key string
	Val any
}

// F 是 Field 的快捷构造器。
func F(key string, val any) Field {
	return Field{Key: key, Val: val}
}

// formatKV 将 [k1, v1, k2, v2, ...] 或 [Field, Field, ...] 格式化为 " k1=v1 k2=v2"。
// 任何无法配对的奇数元素都会被追加为 "_extra=<v>"，保证日志不丢失。
// 返回值在非空时以一个前导空格开头，便于直接拼接到消息后。
func formatKV(args []any) string {
	if len(args) == 0 {
		return ""
	}
	var b strings.Builder
	i := 0
	for i < len(args) {
		switch v := args[i].(type) {
		case Field:
			writeKV(&b, v.Key, v.Val)
			i++
		case string:
			if i+1 < len(args) {
				writeKV(&b, v, args[i+1])
				i += 2
			} else {
				writeKV(&b, "_extra", v)
				i++
			}
		default:
			writeKV(&b, "_extra", v)
			i++
		}
	}
	return b.String()
}

func writeKV(b *strings.Builder, k string, v any) {
	b.WriteByte(' ')
	b.WriteString(k)
	b.WriteByte('=')
	fmt.Fprintf(b, "%v", v)
}

