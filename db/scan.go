package db

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// scanTargets builds scan destinations for the model fields backing columns.
// Values are buffered into temporaries so driver-specific representations
// (sqlite TEXT timestamps, 0/1 booleans) can be converted into the Go field
// types afterwards.
func scanTargets(v reflect.Value, fields []FieldInfo, columns []string) ([]any, []reflect.Value) {
	ptrs := make([]any, 0, len(columns))
	dests := make([]reflect.Value, 0, len(columns))
	for _, col := range columns {
		var fi *FieldInfo
		for i := range fields {
			if fields[i].Column == col {
				fi = &fields[i]
				break
			}
		}
		fv := reflect.Value{}
		if fi != nil {
			candidate := v.FieldByName(fi.GoName)
			if candidate.IsValid() && candidate.CanSet() {
				fv = candidate
			}
		}
		var tmp any
		ptrs = append(ptrs, &tmp)
		dests = append(dests, fv)
	}
	return ptrs, dests
}

// assignTargets copies the buffered scan values into the model fields.
func assignTargets(dests []reflect.Value, ptrs []any) error {
	for i, fv := range dests {
		if !fv.IsValid() {
			continue
		}
		val := *ptrs[i].(*any)
		if err := setField(fv, val); err != nil {
			return err
		}
	}
	return nil
}

// setField assigns a driver value to a struct field with type conversion.
func setField(fv reflect.Value, val any) error {
	if val == nil {
		return nil
	}
	if fv.Kind() == reflect.Pointer {
		elem := reflect.New(fv.Type().Elem())
		if err := setField(elem.Elem(), val); err != nil {
			return err
		}
		fv.Set(elem)
		return nil
	}
	if fv.Type() == reflect.TypeFor[time.Time]() {
		ts, err := toTime(val)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(ts))
		return nil
	}
	if fv.Kind() == reflect.Bool {
		b, err := toBool(val)
		if err != nil {
			return err
		}
		fv.SetBool(b)
		return nil
	}
	rv := reflect.ValueOf(val)
	if rv.Type().AssignableTo(fv.Type()) {
		fv.Set(rv)
		return nil
	}
	if rv.Type().ConvertibleTo(fv.Type()) {
		fv.Set(rv.Convert(fv.Type()))
		return nil
	}
	if s, ok := val.([]byte); ok && fv.Kind() == reflect.String {
		fv.SetString(string(s))
		return nil
	}
	if s, ok := asString(val); ok {
		if err := setFromString(fv, s); err == nil {
			return nil
		}
	}
	return fmt.Errorf("db: cannot scan %T into %s", val, fv.Type())
}

// asString returns the textual form of string-like driver values.
func asString(val any) (string, bool) {
	switch v := val.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

// setFromString parses a textual driver value into a numeric field.
func setFromString(fv reflect.Value, s string) error {
	switch fv.Kind() {
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
		return nil
	case reflect.String:
		fv.SetString(s)
		return nil
	default:
		return fmt.Errorf("db: cannot set %s from string", fv.Type())
	}
}

// ParseTime converts a driver value (time.Time, string or []byte) into
// time.Time. Handy when scanning a timestamp by hand across dialects.
func ParseTime(val any) (time.Time, error) {
	return toTime(val)
}

// ParseTimePtr converts a nullable driver value into *time.Time.
func ParseTimePtr(val any) (*time.Time, error) {
	if val == nil {
		return nil, nil
	}
	t, err := toTime(val)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func toTime(val any) (time.Time, error) {
	switch v := val.(type) {
	case time.Time:
		return v, nil
	case string:
		return parseTimeString(v)
	case []byte:
		return parseTimeString(string(v))
	case int64:
		return time.Unix(v, 0), nil
	default:
		return time.Time{}, fmt.Errorf("db: cannot scan %T into time.Time", val)
	}
}

func parseTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("db: cannot parse %q as time", s)
}

func toBool(val any) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	case int:
		return v != 0, nil
	case string:
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "t"), nil
	case []byte:
		return toBool(string(v))
	default:
		return false, fmt.Errorf("db: cannot scan %T into bool", val)
	}
}
