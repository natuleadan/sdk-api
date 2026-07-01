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

	if f.Auto {
		switch f.FieldType.Kind() {
		case reflect.Int, reflect.Int64:
			parts = append(parts, "BIGSERIAL")
		default:
			parts = append(parts, "BIGSERIAL")
		}
	} else {
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
