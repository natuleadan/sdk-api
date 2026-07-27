package db

import (
	"testing"
)

func TestPreparedStmtName(t *testing.T) {
	underlying := &Table[struct {
		ID int64 `db:"id,primary,auto"`
	}]{
		tableName: "products",
		info:      &TableInfo{PrimaryKey: "id"},
	}
	tbl := &PreparedTable[struct {
		ID int64 `db:"id,primary,auto"`
	}]{
		Table: underlying,
	}

	name := tbl.stmtName("list")
	expected := "sdk_products:list"
	if name != expected {
		t.Errorf("stmtName = %q, want %q", name, expected)
	}
}

func TestPreparedBuildSQL(t *testing.T) {
	underlying := &Table[struct {
		ID int64 `db:"id,primary,auto"`
	}]{
		tableName: "test",
		info:      &TableInfo{PrimaryKey: "id"},
	}
	tbl := &PreparedTable[struct {
		ID int64 `db:"id,primary,auto"`
	}]{
		Table: underlying,
	}

	tests := []struct {
		op       string
		wantPart string
	}{
		{"list", "SELECT  FROM test ORDER BY id LIMIT 1000"},
		{"get", "SELECT  FROM test WHERE id = $1"},
		{"count", "SELECT COUNT(*) FROM test"},
		{"exists", "SELECT 1 FROM test WHERE id = $1 LIMIT 1"},
		{"delete", "DELETE FROM test WHERE id = $1"},
	}

	for _, tt := range tests {
		sql, err := tbl.buildSQL(tt.op)
		if err != nil {
			t.Fatalf("buildSQL(%q): %v", tt.op, err)
		}
		if sql != tt.wantPart {
			t.Errorf("buildSQL(%q) = %q, want %q", tt.op, sql, tt.wantPart)
		}
	}
}

func TestPreparedBuildSQL_Unknown(t *testing.T) {
	tbl := &PreparedTable[struct {
		ID int64 `db:"id,primary,auto"`
	}]{Table: &Table[struct {
		ID int64 `db:"id,primary,auto"`
	}]{}}
	_, err := tbl.buildSQL("unknown")
	if err == nil {
		t.Error("expected error for unknown op")
	}
}

func TestPreparedNewTable_ValidStruct(t *testing.T) {
	tbl, err := NewPreparedTable[struct {
		ID int64 `db:"id,primary,auto"`
	}](nil, "test")
	if err != nil {
		t.Fatalf("NewPreparedTable: %v", err)
	}
	if tbl == nil {
		t.Fatal("expected non-nil table")
	}
	if tbl.pool != nil {
		t.Error("expected nil pool")
	}
}
