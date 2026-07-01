package lang

import (
	"fmt"
	"reflect"
	"strconv"
)

// Placeholder is a placeholder object that can be used globally.
var Placeholder PlaceholderType

type (
	// AnyType can be used to hold any type.
	AnyType = any
	// PlaceholderType represents a placeholder type.
	PlaceholderType = struct{}
)

// Repr returns the string representation of v.
func Repr(v any) string {
	if v == nil {
		return ""
	}

	// if func (v *Type) String() string, we can't use Elem()
	if vt, ok := v.(fmt.Stringer); ok {
		return vt.String()
	}

	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Pointer && !val.IsNil() {
		val = val.Elem()
	}

	return reprOfValue(val)
}

func reprOfValue(val reflect.Value) string {
	switch vt := val.Interface().(type) {
	case bool:
		return strconv.FormatBool(vt)
	case error:
		return vt.Error()
	case fmt.Stringer:
		return vt.String()
	case string:
		return vt
	case []byte:
		return string(vt)
	default:
		switch v := val.Interface().(type) {
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 32)
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case int, int8, int16, int32, int64:
			return strconv.FormatInt(reflect.ValueOf(v).Int(), 10)
		case uint, uint8, uint16, uint32, uint64:
			return strconv.FormatUint(reflect.ValueOf(v).Uint(), 10)
		default:
			return fmt.Sprint(val.Interface())
		}
	}
}
