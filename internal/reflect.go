package internal

import (
	"fmt"
	"reflect"
	"strings"
)

func NewEntity(resp interface{}) interface{} {
	t := reflect.TypeOf(resp).Elem().Elem()
	return reflect.New(t).Interface()
}

func NewEntitySlice(resp interface{}) interface{} {
	t := reflect.TypeOf(resp).Elem()
	return reflect.New(t).Interface()
}

func GetPKValue1(entity interface{}, pkField string) string {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// 优先匹配 gorm tag
		if strings.Contains(field.Tag.Get("gorm"), "primaryKey") {
			return fmt.Sprintf("%v", v.Field(i).Interface())
		}
		// 字段名
		if field.Name == pkField {
			return fmt.Sprintf("%v", v.Field(i).Interface())
		}

	}
	panic("primary key not found")
}
