package db

import "reflect"

// IndexFields returns the columns of a model tagged index, unique or primary.
// MongoDB is schemaless (no DDL), so AutoInit there means ensuring these fields
// as indexes at startup: pass the result to the Mongo registration.
func IndexFields[T any]() ([]string, error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return nil, ErrNotStruct
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, ErrNotStruct
	}
	info := parseType(t)
	out := make([]string, 0, len(info.Fields))
	for _, f := range info.Fields {
		if f.Skip || f.Column == "" {
			continue
		}
		if f.Index || f.Unique || f.Primary {
			out = append(out, f.Column)
		}
	}
	return out, nil
}
