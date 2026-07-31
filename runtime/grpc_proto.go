package runtime

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/natuleadan/sdk-api/db"
)

func protoType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "int64"
	case reflect.Int32:
		return "int32"
	case reflect.Float64:
		return "double"
	case reflect.Float32:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes"
		}
		return "repeated " + protoType(t.Elem())
	case reflect.Map:
		return "map<string, " + protoType(t.Elem()) + ">"
	case reflect.Struct:
		if t.String() == "time.Time" {
			return "string"
		}
		return "string"
	default:
		return "string"
	}
}

func GenerateProto(info *db.TableInfo, modelName, svcName, pkg string) string {
	if pkg == "" {
		pkg = strings.ToLower(modelName)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `syntax = "proto3";

package %s;

option go_package = "%s;pb";

service %s {
  rpc Get%s(Get%sRequest) returns (%s);
  rpc List%s(List%sRequest) returns (List%sResponse);
  rpc Create%s(Create%sRequest) returns (%s);
  rpc Update%s(Update%sRequest) returns (%s);
  rpc Delete%s(Delete%sRequest) returns (Delete%sResponse);
}

message %s {
`, pkg, pkg, svcName, modelName, modelName, modelName,
		modelName, modelName, modelName,
		modelName, modelName, modelName,
		modelName, modelName, modelName,
		modelName, modelName, modelName,
		modelName)

	idx := 1
	for _, f := range info.Fields {
		if f.Skip || f.Column == "" {
			continue
		}
		pt := protoType(f.FieldType)
		fmt.Fprintf(&b, "  %s %s = %d;\n", pt, f.Column, idx)
		idx++
	}

	fmt.Fprintf(&b, `}

message Get%sRequest {
  int64 id = 1;
}

message List%sRequest {
  int32 page = 1;
  int32 size = 2;
}

message List%sResponse {
  repeated %s items = 1;
  int32 total = 2;
}

message Create%sRequest {
`, modelName, modelName, modelName, modelName, modelName)

	idx = 2
	for _, f := range info.Fields {
		if f.Skip || f.Column == "" || f.Primary || f.Auto {
			continue
		}
		pt := protoType(f.FieldType)
		fmt.Fprintf(&b, "  %s %s = %d;\n", pt, f.Column, idx)
		idx++
	}

	fmt.Fprintf(&b, `}

message Update%sRequest {
  int64 id = 1;
`, modelName)

	idx = 2
	for _, f := range info.Fields {
		if f.Skip || f.Column == "" || f.Primary || f.Auto {
			continue
		}
		pt := protoType(f.FieldType)
		fmt.Fprintf(&b, "  optional %s %s = %d;\n", pt, f.Column, idx)
		idx++
	}

	fmt.Fprintf(&b, `}

message Delete%sRequest {
  int64 id = 1;
}

message Delete%sResponse {
  bool ok = 1;
}
`, modelName, modelName)

	return b.String()
}
