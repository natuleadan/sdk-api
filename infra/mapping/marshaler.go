package mapping

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

const (
	emptyTag       = ""
	tagKVSeparator = ":"
)

// Marshal marshals the given val and returns the map that contains the fields.
// optional=another is not implemented, and it's hard to implement and not commonly used.
// support anonymous field, e.g.:
//
//	type Foo struct {
//		Token string `header:"token"`
//	}
//	type FooB struct {
//		Foo
//		Bar string  `json:"bar"`
//	}
func Marshal(val any) (map[string]map[string]any, error) {
	ret := make(map[string]map[string]any)
	tp := reflect.TypeOf(val)
	if tp.Kind() == reflect.Pointer {
		tp = tp.Elem()
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	for i := 0; i < tp.NumField(); i++ {
		field := tp.Field(i)
		value := rv.Field(i)
		if err := processMember(field, value, ret); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func getTag(field reflect.StructField) (string, bool) {
	tag := string(field.Tag)
	if before, _, ok := strings.Cut(tag, tagKVSeparator); ok {
		return strings.TrimSpace(before), true
	}

	return strings.TrimSpace(tag), false
}

func insertValue(collector map[string]map[string]any, tag string, key string, val any) {
	if m, ok := collector[tag]; ok {
		m[key] = val
	} else {
		collector[tag] = map[string]any{
			key: val,
		}
	}
}

func processMember(field reflect.StructField, value reflect.Value,
	collector map[string]map[string]any) error {
	var key string
	var opt *fieldOptions
	var err error
	tag, ok := getTag(field)
	if !ok {
		tag = emptyTag
		key = field.Name
	} else {
		key, opt, err = parseKeyAndOptions(tag, field)
		if err != nil {
			return err
		}

		if err = validate(field, value, opt); err != nil {
			return err
		}
	}

	val := value.Interface()
	if opt != nil && opt.FromString {
		val = fmt.Sprint(val)
	}

	if field.Anonymous {
		anonCollector, err := Marshal(val)
		if err != nil {
			return err
		}

		for anonTag, anonMap := range anonCollector {
			for anonKey, anonVal := range anonMap {
				insertValue(collector, anonTag, anonKey, anonVal)
			}
		}
	} else {
		insertValue(collector, tag, key, val)
	}

	return nil
}

func validate(field reflect.StructField, value reflect.Value, opt *fieldOptions) error {
	if opt == nil || !opt.Optional {
		if err := validateOptional(field, value); err != nil {
			return err
		}
	}

	if opt == nil {
		return nil
	}

	if opt.Optional && value.IsZero() {
		return nil
	}

	if len(opt.Options) > 0 {
		if err := validateOptions(value, opt); err != nil {
			return err
		}
	}

	if opt.Range != nil {
		if err := validateRange(value, opt); err != nil {
			return err
		}
	}

	return nil
}

func validateOptional(field reflect.StructField, value reflect.Value) error {
	switch field.Type.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return fmt.Errorf("field %q is nil", field.Name)
		}
	case reflect.Slice, reflect.Map:
		if value.IsNil() || value.Len() == 0 {
			return fmt.Errorf("field %q is empty", field.Name)
		}
	}

	return nil
}

func validateOptions(value reflect.Value, opt *fieldOptions) error {
	val := fmt.Sprint(value.Interface())
	if !slices.Contains(opt.Options, val) {
		return fmt.Errorf("field %q not in options", val)
	}

	return nil
}

func validateRange(value reflect.Value, opt *fieldOptions) error {
	if opt.Range == nil {
		return nil
	}
	val, err := toFloat64Err(value.Interface())
	if err != nil {
		return err
	}
	if opt.Range.left > val || (!opt.Range.leftInclude && val == opt.Range.left) ||
		opt.Range.right < val || (!opt.Range.rightInclude && val == opt.Range.right) {
		return fmt.Errorf("%v out of range", value.Interface())
	}
	return nil
}

func toFloat64Err(v any) (float64, error) {
	switch val := v.(type) {
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	default:
		return 0, fmt.Errorf("unknown support type for range %q", reflect.TypeOf(v).String())
	}
}
