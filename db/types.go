package db

import (
	"reflect"
	"strings"
)

const tagName = "db"

type FieldInfo struct {
	Column       string
	GoName       string
	Primary      bool
	Auto         bool
	Required     bool
	Default      string
	Index        bool
	Unique       bool
	Skip         bool
	FieldType    reflect.Type
	Tags         reflect.StructTag
	TypeOverride string // type=DECIMAL(10,2), type=JSONB, type=TEXT[]
	FK           string // fk=users.id
}

// TableConstraints is an optional interface that structs can implement
// to declare table-level constraints (composite unique, foreign keys, etc.).
type TableConstraints interface {
	Constraints() []Constraint
}

// Constraint represents a table-level constraint such as UNIQUE(a,b).
type Constraint struct {
	Type    string   // "UNIQUE", "INDEX", "CHECK"
	Columns []string // column names in order
	Name    string   // optional constraint name
}

type TableInfo struct {
	Name       string
	Fields     []FieldInfo
	PrimaryKey string
}

func parseStruct[T any]() (*TableInfo, error) {
	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, ErrNotStruct
	}
	info := parseType(typ)
	if info.PrimaryKey == "" {
		return nil, ErrNoPrimaryKey
	}
	return info, nil
}

func ParseStruct[T any]() (*TableInfo, error) {
	return parseStruct[T]()
}

// ParseStructReflect parses struct tags from a reflect.Type (non-generic version).
func ParseStructReflect(typ reflect.Type) (*TableInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, ErrNotStruct
	}
	return parseType(typ), nil
}

func parseType(typ reflect.Type) *TableInfo {
	info := &TableInfo{Fields: make([]FieldInfo, 0, typ.NumField())}
	for f := range typ.Fields() {
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get(tagName)
		fi := FieldInfo{
			GoName:    f.Name,
			FieldType: f.Type,
			Tags:      f.Tag,
		}
		parts := strings.Split(tag, ",")
		col := strings.TrimSpace(parts[0])
		if col == "" || col == "-" {
			if col == "-" {
				fi.Skip = true
				info.Fields = append(info.Fields, fi)
				continue
			}
			col = toSnake(f.Name)
		}
		fi.Column = col
		parseTagParts(parts[1:], &fi, col, info)
		info.Fields = append(info.Fields, fi)
	}
	return info
}

func parseTagParts(parts []string, fi *FieldInfo, col string, info *TableInfo) {
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case p == "primary":
			fi.Primary = true
			info.PrimaryKey = col
		case p == "auto":
			fi.Auto = true
		case p == "required":
			fi.Required = true
		case p == "index":
			fi.Index = true
		case p == "unique":
			fi.Unique = true
		case p == "-":
			fi.Skip = true
		case strings.HasPrefix(p, "default="):
			fi.Default = strings.TrimPrefix(p, "default=")
		case strings.HasPrefix(p, "type="):
			fi.TypeOverride = strings.TrimPrefix(p, "type=")
		case strings.HasPrefix(p, "fk="):
			fi.FK = strings.TrimPrefix(p, "fk=")
		}
	}
}

func toSnake(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if isUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if isLower(prev) {
					b.WriteRune('_')
				} else if i+1 < len(runes) && isLower(runes[i+1]) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}
