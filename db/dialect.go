package db

import (
	"reflect"
	"strings"
)

// dialect identifies the SQL family used to render DDL. A model is generated
// for every family: the `type=` override and the `default=` expression are
// translated by the dialect instead of being tied to PostgreSQL.
type dialect int

const (
	dialectPostgres dialect = iota
	dialectMySQL
	dialectSQLite // Turso / libSQL
)

// columnType returns the column type for a field in the given dialect. The
// `type=` override wins and is translated across families.
func columnType(d dialect, f FieldInfo) string {
	if f.Auto {
		switch d {
		case dialectMySQL:
			return "BIGINT UNSIGNED AUTO_INCREMENT"
		case dialectSQLite:
			return "INTEGER"
		default:
			return "BIGSERIAL"
		}
	}
	if f.TypeOverride != "" {
		return translateType(d, f.TypeOverride)
	}
	return baseType(d, f.FieldType)
}

// typeKind is the canonical family of a `type=` override.
type typeKind int

const (
	tkUnknown typeKind = iota
	tkJSON
	tkArray
	tkDecimal
	tkTime
	tkUUID
	tkBool
	tkBytes
)

// typeTranslation maps a canonical override to the dialect's type. An empty
// value means "keep the original override" (Postgres-flavoured). Postgres is
// absent on purpose: overrides pass through untouched.
var typeTranslation = map[dialect]map[typeKind]string{
	dialectMySQL: {
		tkJSON:    "JSON",
		tkArray:   "JSON",
		tkDecimal: "",
		tkTime:    "DATETIME(3)",
		tkUUID:    "CHAR(36)",
		tkBool:    "TINYINT(1)",
		tkBytes:   "BLOB",
	},
	dialectSQLite: {
		tkJSON:    "TEXT",
		tkArray:   "TEXT",
		tkDecimal: "NUMERIC",
		tkTime:    "TEXT",
		tkUUID:    "TEXT",
		tkBool:    "INTEGER",
		tkBytes:   "BLOB",
	},
}

// translateType maps a PostgreSQL-flavoured type override to the dialect.
// Unknown types pass through for Postgres and MySQL and fall back to TEXT on
// SQLite, which is dynamically typed.
func translateType(d dialect, t string) string {
	rules, ok := typeTranslation[d]
	if !ok {
		return t
	}
	if mapped, ok := rules[canonicalType(strings.ToUpper(strings.TrimSpace(t)))]; ok && mapped != "" {
		return mapped
	}
	if d == dialectSQLite {
		return "TEXT"
	}
	return t
}

func canonicalType(up string) typeKind {
	switch {
	case up == "JSON" || strings.HasPrefix(up, "JSONB"):
		return tkJSON
	case up == "TEXT[]" || strings.HasSuffix(up, "[]"):
		return tkArray
	case strings.HasPrefix(up, "DECIMAL") || strings.HasPrefix(up, "NUMERIC"):
		return tkDecimal
	case strings.HasPrefix(up, "TIMESTAMPTZ") || strings.HasPrefix(up, "TIMESTAMP"):
		return tkTime
	case up == "UUID":
		return tkUUID
	case up == "BOOLEAN" || up == "BOOL":
		return tkBool
	case up == "BYTEA" || up == "BLOB":
		return tkBytes
	default:
		return tkUnknown
	}
}

// baseType maps a Go kind to the dialect's column type when there is no
// `type=` override.
func baseType(d dialect, t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch d {
	case dialectMySQL:
		return baseTypeMySQL(t)
	case dialectSQLite:
		return baseTypeSQLite(t)
	default:
		return sqlType(t)
	}
}

func baseTypeMySQL(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE"
	case reflect.String:
		return "VARCHAR(255)"
	case reflect.Bool:
		return "TINYINT(1)"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "BLOB"
		}
		return "JSON"
	case reflect.Map:
		return "JSON"
	case reflect.Struct:
		if t.String() == "time.Time" {
			return "DATETIME(3)"
		}
		return "JSON"
	default:
		return "VARCHAR(255)"
	}
}

func baseTypeSQLite(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Bool:
		return "INTEGER"
	case reflect.Float32, reflect.Float64:
		return "REAL"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "BLOB"
		}
		return "TEXT"
	default:
		return "TEXT"
	}
}

// columnDefault renders the DEFAULT token for the dialect. ok=false means the
// default must be omitted: SQLite has no server-side uuid generator the SDK
// relies on, so the value is produced by the application instead.
func columnDefault(d dialect, def string) (string, bool) {
	trimmed := strings.TrimSpace(def)
	if trimmed == "" {
		return "", false
	}
	switch strings.ToLower(trimmed) {
	case "now()", "current_timestamp":
		switch d {
		case dialectMySQL:
			return "CURRENT_TIMESTAMP(3)", true
		case dialectSQLite:
			return "CURRENT_TIMESTAMP", true
		default:
			return "now()", true
		}
	case "gen_random_uuid()", "uuid()", "uuid_generate_v4()":
		switch d {
		case dialectMySQL:
			return "(UUID())", true
		case dialectSQLite:
			return "(lower(hex(randomblob(16))))", true
		default:
			return "gen_random_uuid()", true
		}
	case "true", "false":
		if d == dialectPostgres {
			return strings.ToLower(trimmed), true
		}
		if strings.EqualFold(trimmed, "true") {
			return "1", true
		}
		return "0", true
	case "null":
		return "NULL", true
	}
	if strings.Contains(trimmed, "(") {
		if d == dialectSQLite {
			return "", false
		}
		return trimmed, true
	}
	if needsQuotedDefault(trimmed) {
		return "'" + trimmed + "'", true
	}
	return trimmed, true
}
