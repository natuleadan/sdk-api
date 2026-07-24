package db

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

func (t *Table[T]) AutoInit(ctx context.Context) error {
	var parts []string
	var indexes []string

	for _, f := range t.info.Fields {
		if f.Skip {
			continue
		}
		col := buildColumnDef(f)
		parts = append(parts, col)

		if f.Index || f.Unique {
			idxType := "INDEX"
			if f.Unique {
				idxType = "UNIQUE INDEX"
			}
			idxName := fmt.Sprintf("idx_%s_%s", t.tableName, f.Column)
			indexes = append(indexes, fmt.Sprintf(
				"CREATE %s IF NOT EXISTS %s ON %s (%s)", idxType, idxName, t.tableName, f.Column))
		}
	}

	var zero T
	if tc, ok := any(zero).(TableConstraints); ok {
		for _, c := range tc.Constraints() {
			switch c.Type {
			case "UNIQUE":
				parts = append(parts, fmt.Sprintf("UNIQUE (%s)", strings.Join(c.Columns, ", ")))
			case "INDEX":
				idxName := c.Name
				if idxName == "" {
					idxName = fmt.Sprintf("idx_%s_%s", t.tableName, strings.Join(c.Columns, "_"))
				}
				indexes = append(indexes, fmt.Sprintf(
					"CREATE INDEX IF NOT EXISTS %s ON %s (%s)", idxName, t.tableName, strings.Join(c.Columns, ", ")))
			case "CHECK":
				parts = append(parts, fmt.Sprintf("CHECK (%s)", c.Columns[0]))
			}
		}
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n)", t.tableName, strings.Join(parts, ",\n  "))
	if _, err := t.pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("db: migrate create table: %w", err)
	}

	for _, idx := range indexes {
		if _, err := t.pool.Exec(ctx, idx); err != nil {
			return fmt.Errorf("db: migrate index: %w", err)
		}
	}

	return nil
}

func buildColumnDef(f FieldInfo) string {
	var parts []string
	parts = append(parts, f.Column)

	switch {
	case f.Auto:
		parts = append(parts, "BIGSERIAL")
	case f.TypeOverride != "":
		parts = append(parts, f.TypeOverride)
	default:
		parts = append(parts, sqlType(f.FieldType))
	}

	if f.Primary {
		parts = append(parts, "PRIMARY KEY")
	}
	if f.Required {
		parts = append(parts, "NOT NULL")
	}
	if f.Default != "" {
		parts = append(parts, "DEFAULT "+f.Default)
	}
	if f.FK != "" {
		parts = append(parts, "REFERENCES "+f.FK)
	}

	return strings.Join(parts, " ")
}

func sqlType(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE PRECISION"
	case reflect.String:
		return "TEXT"
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "BYTEA"
		}
		return "JSONB"
	case reflect.Map:
		return "JSONB"
	case reflect.Struct:
		if t.String() == "time.Time" {
			return "TIMESTAMPTZ"
		}
		if t.String() == "netip.Addr" {
			return "INET"
		}
		return "JSONB"
	default:
		return "TEXT"
	}
}
