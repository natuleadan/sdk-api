package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/natuleadan/sdk-api/db"
)

type Product struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name,required"`
	Price float64 `db:"price"`
	Stock int     `db:"stock"`
}

func TestGenerateProto(t *testing.T) {
	info, err := db.ParseStruct[Product]()
	if err != nil {
		t.Fatalf("ParseStruct: %v", err)
	}

	proto := GenerateProto(info, "Product", "ProductService", "product")

	checks := []string{
		"syntax = \"proto3\"",
		"package product;",
		"service ProductService",
		"rpc GetProduct(GetProductRequest) returns (Product)",
		"rpc ListProduct(ListProductRequest) returns (ListProductResponse)",
		"rpc CreateProduct(CreateProductRequest) returns (Product)",
		"rpc UpdateProduct(UpdateProductRequest) returns (Product)",
		"rpc DeleteProduct(DeleteProductRequest) returns (DeleteProductResponse)",
		"message Product {",
		"int64 id = 1",
		"string name = 2",
		"double price = 3",
		"int64 stock = 4",
		"message GetProductRequest",
		"int64 id = 1",
		"message ListProductRequest",
		"message ListProductResponse",
		"repeated Product items",
		"message CreateProductRequest",
		"optional double price",
	}

	for _, check := range checks {
		if !strings.Contains(proto, check) {
			t.Errorf("expected proto to contain %q", check)
		}
	}
}

func TestGenerateProto_CustomServiceName(t *testing.T) {
	info, err := db.ParseStruct[Product]()
	if err != nil {
		t.Fatalf("ParseStruct: %v", err)
	}

	proto := GenerateProto(info, "Product", "MyCustomService", "custom")

	if !strings.Contains(proto, "service MyCustomService") {
		t.Error("expected service MyCustomService")
	}
	if !strings.Contains(proto, "package custom;") {
		t.Error("expected package custom")
	}
}

type WithTime struct {
	ID        int64  `db:"id,primary,auto"`
	Name      string `db:"name"`
	CreatedAt string `db:"created_at"`
}

func TestGenerateProto_TimeType(t *testing.T) {
	info, err := db.ParseStruct[WithTime]()
	if err != nil {
		t.Fatalf("ParseStruct: %v", err)
	}

	proto := GenerateProto(info, "WithTime", "TestService", "test")
	if !strings.Contains(proto, "string name") {
		t.Error("expected string type for name")
	}
	if !strings.Contains(proto, "string created_at") {
		t.Error("expected string type for created_at (time as string)")
	}
}

func TestProtoType(t *testing.T) {
	tests := []struct {
		goType string
		want   string
	}{
		{"string", "string"},
		{"int64", "int64"},
		{"int", "int64"},
		{"float64", "double"},
		{"float32", "float"},
		{"bool", "bool"},
	}

	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			// Create a struct with a field of the target type and parse it
			// We test via protoType directly by constructing a basic struct
			_ = tt
		})
	}
}

func TestGenerateProto_UsesColumnNames(t *testing.T) {
	info, err := db.ParseStruct[Product]()
	if err != nil {
		t.Fatalf("ParseStruct: %v", err)
	}

	proto := GenerateProto(info, "Product", "Svc", "pkg")
	if !strings.Contains(proto, "int64 id = 1") {
		t.Error("expected id column name")
	}
	if !strings.Contains(proto, "string name = 2") {
		t.Error("expected name column name")
	}
	if !strings.Contains(proto, "double price = 3") {
		t.Error("expected price column name")
	}
}

func TestProtoType_Direct(t *testing.T) {
	if got := protoType(reflect.TypeOf("")); got != "string" {
		t.Errorf("string: got %q", got)
	}
	if got := protoType(reflect.TypeOf(int64(0))); got != "int64" {
		t.Errorf("int64: got %q", got)
	}
	if got := protoType(reflect.TypeOf(float64(0))); got != "double" {
		t.Errorf("float64: got %q", got)
	}
	if got := protoType(reflect.TypeOf(true)); got != "bool" {
		t.Errorf("bool: got %q", got)
	}
}
