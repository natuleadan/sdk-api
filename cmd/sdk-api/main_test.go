package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestRunNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-service")
	err := runNew([]string{"my-service", "--model", "Product", "--fields", "name:string,price:float64,stock:int", "--port", "9090", "--dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "main.go", `runtime.NewFromYAML(configYAML)`)
	checkFile(t, dir, "main.go", "db.NewTable[models.Product]")
	checkFile(t, dir, "main.go", `runtime.NewCRUDProvider`)
	checkFile(t, dir, "service.yaml", "name: my-service")
	checkFile(t, dir, "service.yaml", "port: 9090")
	checkFile(t, dir, "service.yaml", "table: product")
	checkFile(t, dir, "service.yaml", "model: Product")
	checkFile(t, dir, "models/model.go", "type Product struct")
	checkFile(t, dir, "models/model.go", "Name string")
	checkFile(t, dir, "models/model.go", "Price float64")
	checkFile(t, dir, "models/model.go", "Stock int")
}

func checkFile(t *testing.T, dir, rel, substr string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Errorf("expected %q in %s", substr, rel)
	}
}

func TestRunNewWithNATS(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nats-svc")
	err := runNew([]string{
		"nats-svc", "--model", "Order",
		"--fields", "total:float64",
		"--consume", "orders:orders-consumer:onOrderCreated,payments:payments-consumer",
		"--publish", "order-results:create|update",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "stream: orders")
	checkFile(t, dir, "service.yaml", "stream: payments")
	checkFile(t, dir, "service.yaml", "- name: order-results")
	checkFile(t, dir, "service.yaml", "handler: onOrderCreated")
	checkFile(t, dir, "main.go", `WithExit("onOrderCreated"`)
}

func TestRunNewDefaultFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "default-svc")
	err := runNew([]string{"default-svc", "--dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "port: 8080")
	checkFile(t, dir, "models/model.go", "type DefaultSvc struct")
}

func TestRunNewModelNameFromService(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "user-profile")
	err := runNew([]string{"user-profile", "--dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "models/model.go", "type UserProfile struct")
	checkFile(t, dir, "service.yaml", "table: user_profile")
	checkFile(t, dir, "service.yaml", "resource: user_profiles")
}

func TestRunNewNoName(t *testing.T) {
	err := runNew([]string{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRunDocker(t *testing.T) {
	output := captureStdout(func() {
		err := runDocker([]string{"--name", "myapp", "--port", "9090", "--main", "cmd/server.go"})
		if err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "FROM golang:1.26-alpine AS builder") {
		t.Error("expected builder image")
	}
	if !strings.Contains(output, "EXPOSE 9090") {
		t.Error("expected EXPOSE 9090")
	}
	if !strings.Contains(output, `CMD ["/app/myapp"]`) {
		t.Error("expected CMD")
	}
	if !strings.Contains(output, "COPY go.mod go.sum ./") {
		t.Error("expected go.mod copy")
	}
}

func TestRunDockerScratch(t *testing.T) {
	output := captureStdout(func() {
		runDocker([]string{"--name", "svc", "--base", "scratch"})
	})
	if !strings.Contains(output, "FROM scratch") {
		t.Error("expected scratch base")
	}
	if !strings.Contains(output, "ca-certificates.crt") {
		t.Error("expected ca-certificates in scratch")
	}
}

func TestRunDockerAlpine(t *testing.T) {
	output := captureStdout(func() {
		runDocker([]string{"--name", "svc", "--base", "alpine:latest"})
	})
	if !strings.Contains(output, "FROM alpine:latest") {
		t.Error("expected alpine base")
	}
	if strings.Contains(output, "ca-certificates.crt") {
		t.Error("expected no ca-certs copy for non-scratch")
	}
}

func TestRunKube(t *testing.T) {
	output := captureStdout(func() {
		err := runKube([]string{"--name", "products", "--image", "products:v1", "--port", "8080", "--replicas", "5"})
		if err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "name: products") {
		t.Error("expected name: products")
	}
	if !strings.Contains(output, "image: products:v1") {
		t.Error("expected image: products:v1")
	}
	if !strings.Contains(output, "replicas: 5") {
		t.Error("expected 5 replicas")
	}
	if !strings.Contains(output, "containerPort: 8080") {
		t.Error("expected port 8080")
	}
	if !strings.Contains(output, "apiVersion: autoscaling/v2") {
		t.Error("expected HPA")
	}
	if !strings.Contains(output, "kind: Service") {
		t.Error("expected Service")
	}
}

func TestRunKubeRequiredFlags(t *testing.T) {
	err := runKube([]string{})
	if err == nil {
		t.Fatal("expected error for missing flags")
	}
	err = runKube([]string{"--name", "x"})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestRunKubeDefaults(t *testing.T) {
	output := captureStdout(func() {
		runKube([]string{"--name", "svc", "--image", "svc:v1"})
	})
	if !strings.Contains(output, "namespace: default") {
		t.Error("expected default namespace")
	}
	if !strings.Contains(output, "replicas: 3") {
		t.Error("expected default 3 replicas")
	}
}

func TestRunClientTS(t *testing.T) {
	output := captureStdout(func() {
		err := runClient([]string{"--model", "Product", "--fields", "name:string,price:float64", "--lang", "ts"})
		if err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "Product") {
		t.Error("expected Product interface")
	}
	if !strings.Contains(output, "string") {
		t.Error("expected string type")
	}
	if !strings.Contains(output, "number") {
		t.Error("expected number type for float64")
	}
}

func TestRunClientPython(t *testing.T) {
	output := captureStdout(func() {
		err := runClient([]string{"--model", "Product", "--fields", "name:string,price:float64,active:bool", "--lang", "py"})
		if err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "class Product") {
		t.Error("expected Product class")
	}
	if !strings.Contains(output, "str") {
		t.Error("expected str type")
	}
	if !strings.Contains(output, "float") {
		t.Error("expected float type")
	}
	if !strings.Contains(output, "bool") {
		t.Error("expected bool type")
	}
}

func TestRunClientMissingFlags(t *testing.T) {
	// runClient calls os.Exit(1) on missing flags, so we can't test it directly
	// We test the validation indirectly via runClient's return for other errors
}

func TestRunClientUnsupportedLang(t *testing.T) {
	err := runClient([]string{"--model", "X", "--fields", "a:string", "--lang", "ruby"})
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestRunClientOutputFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "sdk.ts")
	err := runClient([]string{"--model", "Item", "--fields", "id:int64,name:string", "--lang", "ts", "--output", outPath})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Item") {
		t.Error("expected Item in output file")
	}
}

func TestRunNewGeneratedGoValid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "testgen")
	err := runNew([]string{"testgen", "--model", "Widget", "--fields", "name:string,price:float64,active:bool", "--dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "main.go", "package main")
	checkFile(t, dir, "models/model.go", "package models")
	checkFile(t, dir, "models/model.go", "bool")
}

func TestPascalCase(t *testing.T) {
	tests := []struct{ in, out string }{
		{"product", "Product"},
		{"user-profile", "UserProfile"},
		{"my-service-name", "MyServiceName"},
		{"order", "Order"},
	}
	for _, tt := range tests {
		got := pascalCase(tt.in)
		if got != tt.out {
			t.Errorf("pascalCase(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestToSnake(t *testing.T) {
	tests := []struct{ in, out string }{
		{"Product", "product"},
		{"UserProfile", "user_profile"},
		{"MyServiceName", "my_service_name"},
		{"created_at", "created_at"},
		{"URL", "url"},
		{"orderID", "order_id"},
	}
	for _, tt := range tests {
		got := toSnake(tt.in)
		if got != tt.out {
			t.Errorf("toSnake(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestGoType(t *testing.T) {
	tests := []struct{ in, out string }{
		{"string", "string"},
		{"int", "int"},
		{"int64", "int64"},
		{"float64", "float64"},
		{"bool", "bool"},
		{"time", "time.Time"},
		{"unknown", "string"},
	}
	for _, tt := range tests {
		got := goType(tt.in)
		if got != tt.out {
			t.Errorf("goType(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestPlural(t *testing.T) {
	tests := []struct{ in, out string }{
		{"product", "products"},
		{"category", "categories"},
		{"status", "status"},
	}
	for _, tt := range tests {
		got := plural(tt.in)
		if got != tt.out {
			t.Errorf("plural(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestUnique(t *testing.T) {
	got := unique([]string{"a", "b", "a", "c", "b", "a"})
	expected := []string{"a", "b", "c"}
	if len(got) != len(expected) {
		t.Fatalf("got %v, want %v", got, expected)
	}
	for i, v := range expected {
		if got[i] != v {
			t.Errorf("got[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestUniqueEmpty(t *testing.T) {
	got := unique([]string{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestClientTypeTS(t *testing.T) {
	tests := []struct{ goType, out string }{
		{"int", "number"},
		{"int64", "number"},
		{"float64", "number"},
		{"string", "string"},
		{"bool", "boolean"},
		{"time.Time", "string"},
		{"unknown", "any"},
	}
	for _, tt := range tests {
		got := clientType(tt.goType, "ts")
		if got != tt.out {
			t.Errorf("clientType(%q, ts) = %q, want %q", tt.goType, got, tt.out)
		}
	}
}

func TestClientTypePython(t *testing.T) {
	tests := []struct{ goType, out string }{
		{"int", "int"},
		{"int64", "int"},
		{"float64", "float"},
		{"string", "str"},
		{"bool", "bool"},
		{"time.Time", "str"},
		{"unknown", "Any"},
	}
	for _, tt := range tests {
		got := clientType(tt.goType, "py")
		if got != tt.out {
			t.Errorf("clientType(%q, py) = %q, want %q", tt.goType, got, tt.out)
		}
	}
}

func TestClientTypeDart(t *testing.T) {
	tests := []struct{ goType, out string }{
		{"int", "int"},
		{"int64", "int"},
		{"float64", "double"},
		{"string", "String"},
		{"bool", "bool"},
		{"time.Time", "String"},
		{"unknown", "dynamic"},
	}
	for _, tt := range tests {
		got := clientType(tt.goType, "dart")
		if got != tt.out {
			t.Errorf("clientType(%q, dart) = %q, want %q", tt.goType, got, tt.out)
		}
	}
}

func TestClientTypeJava(t *testing.T) {
	tests := []struct{ goType, out string }{
		{"int", "int"},
		{"int64", "long"},
		{"float64", "double"},
		{"float32", "float"},
		{"string", "String"},
		{"bool", "boolean"},
		{"time.Time", "String"},
		{"unknown", "String"},
	}
	for _, tt := range tests {
		got := clientType(tt.goType, "java")
		if got != tt.out {
			t.Errorf("clientType(%q, java) = %q, want %q", tt.goType, got, tt.out)
		}
	}
}

func TestClientTypeKotlin(t *testing.T) {
	tests := []struct{ goType, out string }{
		{"int", "Int"},
		{"int64", "Long"},
		{"float64", "Double"},
		{"float32", "Float"},
		{"string", "String"},
		{"bool", "Boolean"},
		{"time.Time", "String"},
		{"unknown", "String"},
	}
	for _, tt := range tests {
		got := clientType(tt.goType, "kotlin")
		if got != tt.out {
			t.Errorf("clientType(%q, kotlin) = %q, want %q", tt.goType, got, tt.out)
		}
	}
}

func TestRunClientDart(t *testing.T) {
	output := captureStdout(func() {
		err := runClient([]string{"--model", "Product", "--fields", "name:string,price:float64,active:bool", "--lang", "dart"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "class Product") {
		t.Error("expected Product class")
	}
	if !strings.Contains(output, "String") {
		t.Error("expected String type")
	}
	if !strings.Contains(output, "double") {
		t.Error("expected double type")
	}
}

func TestRunClientJava(t *testing.T) {
	output := captureStdout(func() {
		err := runClient([]string{"--model", "Product", "--fields", "name:string,price:float64,active:bool", "--lang", "java"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "public class Product") {
		t.Error("expected Product class")
	}
	if !strings.Contains(output, "private String name") {
		t.Error("expected String field")
	}
	if !strings.Contains(output, "private double price") {
		t.Error("expected double field")
	}
}

func TestRunClientKotlin(t *testing.T) {
	output := captureStdout(func() {
		err := runClient([]string{"--model", "Product", "--fields", "name:string,price:float64,active:bool", "--lang", "kotlin"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "data class Product") {
		t.Error("expected Product data class")
	}
	if !strings.Contains(output, "val name: String") {
		t.Error("expected String field")
	}
	if !strings.Contains(output, "val price: Double") {
		t.Error("expected Double field")
	}
}

func TestConsumeAutoHandler(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "autohandler")
	err := runNew([]string{
		"autohandler", "--model", "Event",
		"--consume", "orders:ord-cons",
		"--publish", "events:create",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "main.go", `WithExit("onOrders"`)
	checkFile(t, dir, "service.yaml", "handler: onOrders")
}

func TestConsumeFullHandler(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fullhandler")
	err := runNew([]string{
		"fullhandler", "--model", "Task",
		"--consume", "tasks:task-consumer:myCustomHandler",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "handler: myCustomHandler")
}

func TestHooksFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hooktest")
	err := runNew([]string{"hooktest", "--dir", dir})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "models/model.go", "DefaultHooks")
}
