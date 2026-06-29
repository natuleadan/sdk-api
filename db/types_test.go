package db

import (
	"testing"
	"time"
)

type testModel struct {
	ID        int64     `db:"id,primary,auto"`
	Name      string    `db:"name,required"`
	Price     float64   `db:"price"`
	Stock     int       `db:"stock,default=0"`
	Active    bool      `db:"active,default=true"`
	CreatedAt time.Time `db:"created_at"`
	Skipped   string    `db:"-"`
}

type modelNoPK struct {
	Name string `db:"name"`
}

type modelNotStruct string

func TestParseStruct(t *testing.T) {
	info, err := parseStruct[testModel]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PrimaryKey != "id" {
		t.Fatalf("expected primary key 'id', got %q", info.PrimaryKey)
	}
	if len(info.Fields) != 7 {
		t.Fatalf("expected 7 fields, got %d", len(info.Fields))
	}

	tests := []struct {
		name   string
		column string
		opts   map[string]bool
	}{
		{"ID", "id", map[string]bool{"Primary": true, "Auto": true}},
		{"Name", "name", map[string]bool{"Required": true}},
		{"Price", "price", map[string]bool{}},
		{"Stock", "stock", map[string]bool{}},
		{"Active", "active", map[string]bool{}},
		{"CreatedAt", "created_at", map[string]bool{}},
		{"Skipped", "", map[string]bool{"Skip": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *FieldInfo
			for _, f := range info.Fields {
				if f.GoName == tt.name {
					found = &f
					break
				}
			}
			if found == nil {
				t.Fatalf("field %q not found", tt.name)
			}
			if found.Column != tt.column {
				t.Errorf("expected column %q, got %q", tt.column, found.Column)
			}
		})
	}

	infoSkipped := info.Fields[len(info.Fields)-1]
	if !infoSkipped.Skip {
		t.Errorf("expected last field to be skipped")
	}
}

func TestParseStructNoPK(t *testing.T) {
	_, err := parseStruct[modelNoPK]()
	if err == nil {
		t.Fatal("expected error for model without primary key")
	}
}

func TestParseStructNotStruct(t *testing.T) {
	_, err := parseStruct[modelNotStruct]()
	if err == nil {
		t.Fatal("expected error for non-struct type")
	}
}

func TestToSnake(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ID", "id"},
		{"Name", "name"},
		{"CreatedAt", "created_at"},
		{"UpdatedAt", "updated_at"},
		{"ProductID", "product_id"},
		{"URL", "url"},
		{"HTTPSConnection", "https_connection"},
		{"userName", "user_name"},
		{"", ""},
		{"A", "a"},
		{"ABC", "abc"},
		{"ABCXYZ", "abcxyz"},
		{"MyHTTPServer", "my_http_server"},
		{"JSONData", "json_data"},
	}

	for _, tt := range tests {
		got := toSnake(tt.input)
		if got != tt.want {
			t.Errorf("toSnake(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
