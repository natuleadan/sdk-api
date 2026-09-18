package db

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColumnType_TypeOverride(t *testing.T) {
	str := reflect.TypeOf("")
	cases := []struct {
		name     string
		override string
		pg       string
		mysql    string
		sqlite   string
	}{
		{"jsonb", "JSONB", "JSONB", "JSON", "TEXT"},
		{"text array", "TEXT[]", "TEXT[]", "JSON", "TEXT"},
		{"decimal", "DECIMAL(10,2)", "DECIMAL(10,2)", "DECIMAL(10,2)", "NUMERIC"},
		{"timestamptz", "TIMESTAMPTZ", "TIMESTAMPTZ", "DATETIME(3)", "TEXT"},
		{"uuid", "UUID", "UUID", "CHAR(36)", "TEXT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := FieldInfo{Column: "c", FieldType: str, TypeOverride: tc.override}
			assert.Equal(t, tc.pg, columnType(dialectPostgres, f))
			assert.Equal(t, tc.mysql, columnType(dialectMySQL, f))
			assert.Equal(t, tc.sqlite, columnType(dialectSQLite, f))
		})
	}
}

func TestColumnType_BaseAndAuto(t *testing.T) {
	str := reflect.TypeOf("")
	assert.Equal(t, "TEXT", columnType(dialectPostgres, FieldInfo{FieldType: str}))
	assert.Equal(t, "VARCHAR(255)", columnType(dialectMySQL, FieldInfo{FieldType: str}))
	assert.Equal(t, "TEXT", columnType(dialectSQLite, FieldInfo{FieldType: str}))

	assert.Equal(t, "BIGSERIAL", columnType(dialectPostgres, FieldInfo{Auto: true}))
	assert.Equal(t, "BIGINT UNSIGNED AUTO_INCREMENT", columnType(dialectMySQL, FieldInfo{Auto: true}))
	assert.Equal(t, "INTEGER", columnType(dialectSQLite, FieldInfo{Auto: true}))
}

func TestColumnDefault(t *testing.T) {
	cases := []struct {
		def      string
		pg       string
		pgOK     bool
		mysql    string
		sqlite   string
		sqliteOK bool
	}{
		{"now()", "now()", true, "CURRENT_TIMESTAMP(3)", "CURRENT_TIMESTAMP", true},
		{"gen_random_uuid()", "gen_random_uuid()", true, "(UUID())", "", false},
		{"true", "true", true, "1", "1", true},
		{"false", "false", true, "0", "0", true},
		{"null", "NULL", true, "NULL", "NULL", true},
		{"0", "0", true, "0", "0", true},
		{"viewer", "'viewer'", true, "'viewer'", "'viewer'", true},
		{"''", "''", true, "''", "''", true},
	}
	for _, tc := range cases {
		t.Run(tc.def, func(t *testing.T) {
			got, ok := columnDefault(dialectPostgres, tc.def)
			assert.Equal(t, tc.pg, got)
			assert.Equal(t, tc.pgOK, ok)

			gotMySQL, _ := columnDefault(dialectMySQL, tc.def)
			assert.Equal(t, tc.mysql, gotMySQL)

			gotSQLite, ok := columnDefault(dialectSQLite, tc.def)
			assert.Equal(t, tc.sqlite, gotSQLite)
			assert.Equal(t, tc.sqliteOK, ok)
		})
	}
}
