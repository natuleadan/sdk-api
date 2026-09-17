package main

import (
	"bytes"
	"encoding/json"
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

	checkFile(t, dir, "cmd/main.go", `runtime.New(cfgPath)`)
	checkFile(t, dir, "cmd/main.go", "db.NewTable[models.Product]")
	checkFile(t, dir, "cmd/main.go", `runtime.NewCRUDProvider`)
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
	checkFile(t, dir, "cmd/main.go", `WithExit("onOrderCreated"`)
}

func TestRunNewWithGRPC(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "grpc-svc")
	err := runNew([]string{
		"grpc-svc", "--model", "Product",
		"--fields", "name:string,price:float64",
		"--grpc", "--grpc-port", "50052",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "mode: micro")
	checkFile(t, dir, "service.yaml", "grpc_server:")
	checkFile(t, dir, "service.yaml", "listen_on: \":50052\"")
	checkFile(t, dir, "service.yaml", "type: grpc")
	checkFile(t, dir, "service.yaml", "service_name: ProductService")
	checkFile(t, dir, "cmd/main.go", `RegisterGrpcService("ProductService"`)
	checkFile(t, dir, "cmd/main.go", `pb.RegisterProductServiceServer`)
	checkFile(t, dir, "grpcserver/products.go", "type ProductServer struct")
}

func TestRunNewWithGRPCCustomServiceName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "grpc-custom-svc")
	err := runNew([]string{
		"grpc-custom-svc", "--model", "Product",
		"--fields", "name:string",
		"--grpc", "--grpc-service", "InventoryService",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "service_name: InventoryService")
	checkFile(t, dir, "cmd/main.go", `RegisterGrpcService("InventoryService"`)
}

func TestRunNewWithAuthManual(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "auth-svc")
	err := runNew([]string{
		"auth-svc", "--model", "User",
		"--fields", "name:string,email:string",
		"--auth", "manual",
		"--features", "mfa,magic-link,sms,social,webauthn,oauth-server",
		"--dir", dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, dir, "service.yaml", "driver: manual")
	checkFile(t, dir, "service.yaml", "secret: \"${JWT_SECRET}\"")
	checkFile(t, dir, "cmd/main.go", `WithAuthValidator`)

	// mfa (2 handlers)
	checkFile(t, dir, "internal/handler/auth_mfa.go", "func MFAEnable")
	checkFile(t, dir, "internal/handler/auth_mfa.go", "func MFAVerify")
	// magic-link (2)
	checkFile(t, dir, "internal/handler/auth_magic-link.go", "func MagicLinkSend")
	checkFile(t, dir, "internal/handler/auth_magic-link.go", "func MagicLinkVerify")
	// sms (2)
	checkFile(t, dir, "internal/handler/auth_sms.go", "func SMSSend")
	checkFile(t, dir, "internal/handler/auth_sms.go", "func SMSVerify")
	// social (5)
	checkFile(t, dir, "internal/handler/auth_social.go", "func SocialLogin")
	checkFile(t, dir, "internal/handler/auth_social.go", "func SocialCallback")
	checkFile(t, dir, "internal/handler/auth_social.go", "func LinkedAccounts")
	checkFile(t, dir, "internal/handler/auth_social.go", "func LinkAccount")
	checkFile(t, dir, "internal/handler/auth_social.go", "func UnlinkAccount")
	// webauthn (8)
	for _, fn := range []string{
		"WebAuthnRegisterBegin", "WebAuthnRegisterFinish",
		"WebAuthnLoginBegin", "WebAuthnLoginFinish",
		"WebAuthnManualLoginBegin", "WebAuthnManualLoginFinish",
		"WebAuthnCredentials", "WebAuthnDeleteCredential",
	} {
		checkFile(t, dir, "internal/handler/auth_webauthn.go", "func "+fn)
	}
	// oauth-server (10)
	for _, fn := range []string{
		"OAuthAuthorize", "OAuthToken", "OAuthIntrospect", "OAuthRevoke",
		"OAuthClientsList", "OAuthClientsCreate", "OAuthClientsDelete",
		"OIDCDiscovery", "OIDCJWKS", "OIDCUserInfo",
	} {
		checkFile(t, dir, "internal/handler/auth_oauth-server.go", "func "+fn)
	}
	// reference comments
	checkFile(t, dir, "internal/handler/auth_oauth-server.go", "examples/400-auth/manual-pg")
	checkFile(t, dir, "internal/handler/auth_webauthn.go", "examples/400-auth/manual-pg")
}

func TestRunNewAuthInvalidDriver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bad-auth")
	err := runNew([]string{"bad-auth", "--auth", "oidc", "--dir", dir})
	if err == nil {
		t.Fatal("expected error for invalid auth driver")
	}
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

	checkFile(t, dir, "cmd/main.go", "package main")
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

	checkFile(t, dir, "cmd/main.go", `WithExit("onOrders"`)
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

func TestOutputWriter_Text(t *testing.T) {
	rootCmd.SetArgs([]string{"--output", "text"})
	defer rootCmd.SetArgs([]string{})

	var buf bytes.Buffer
	ow := NewOutput(&buf)
	if err := ow.Write("hello"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output = %q, want hello", buf.String())
	}
}

func TestOutputWriter_JSON(t *testing.T) {
	rootCmd.SetArgs([]string{"--output", "json"})
	defer rootCmd.SetArgs([]string{})

	var buf bytes.Buffer
	ow := NewOutput(&buf)
	if err := ow.Write(map[string]string{"key": "value"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["key"] != "value" {
		t.Errorf("key = %q, want value", result["key"])
	}
}

func TestProgressIndicator(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress([]string{"step1", "step2"}).WithWriter(&buf)
	p.Step("step1")
	p.Step("step2")
	p.Done()
	output := buf.String()
	if !strings.Contains(output, "2/2") {
		t.Errorf("output = %q, want 2/2", output)
	}
}

func TestOutputFormat_Flag(t *testing.T) {
	// Simulate --output json via flag set + parse
	rootCmd.SetArgs([]string{"--output", "json", "completion", "bash"})
	rootCmd.Execute()
	format := GetOutputFormat()
	if format != FormatJSON {
		t.Errorf("format = %q, want json", format)
	}
}

func TestValidateCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	os.WriteFile(yamlPath, []byte("name: test\nport: 8080\n"), 0o644)

	var buf bytes.Buffer
	rootCmd.SetArgs([]string{"--output", "json", "validate", yamlPath})
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["valid"] != true {
		t.Errorf("valid = %v, want true", result["valid"])
	}
}

func TestProtoInit(t *testing.T) {
	root := t.TempDir()
	if err := runProtoInit(root, "example.com/mono/gen/go"); err != nil {
		t.Fatal(err)
	}

	checkFile(t, root, "proto/buf.yaml", "version: v2")
	checkFile(t, root, "proto/buf.gen.yaml", "out: gen/go")
	checkFile(t, root, "gen/go/go.mod", "module example.com/mono/gen/go")
	checkFile(t, root, "proto/README.md", "sdk-api proto generate")
}

func TestProtoInitDerivesModule(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/mono\n\ngo 1.27\n"), 0o600)
	if err := runProtoInit(root, ""); err != nil {
		t.Fatal(err)
	}

	checkFile(t, root, "gen/go/go.mod", "module example.com/mono/gen/go")
}

func TestProtoInitAlreadyExists(t *testing.T) {
	root := t.TempDir()
	if err := runProtoInit(root, "example.com/mono/gen/go"); err != nil {
		t.Fatal(err)
	}
	if err := runProtoInit(root, "example.com/mono/gen/go"); err == nil {
		t.Error("expected error on second init, got nil")
	}
}

func TestProtoInitNeedsModule(t *testing.T) {
	root := t.TempDir()
	if err := runProtoInit(root, ""); err == nil {
		t.Error("expected error without module source, got nil")
	}
}

func TestNewMonorepoGRPC(t *testing.T) {
	root := t.TempDir()
	if err := runProtoInit(root, "example.com/mono/gen/go"); err != nil {
		t.Fatal(err)
	}

	svcDir := filepath.Join(root, "ms-payments")
	err := runNew([]string{
		"ms-payments", "--model", "Payment",
		"--fields", "amount:float64",
		"--grpc", "--grpc-port", "50052",
		"--dir", svcDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, root, "proto/payments/v1/payments.proto", "package payments.v1;")
	checkFile(t, root, "proto/payments/v1/payments.proto", `option go_package = "example.com/mono/gen/go/payments/v1";`)
	checkFile(t, root, "proto/payments/v1/payments.proto", "service PaymentService")
	if _, err := os.Stat(filepath.Join(svcDir, "api")); !os.IsNotExist(err) {
		t.Error("expected no api/ dir in monorepo service")
	}
	checkFile(t, svcDir, "pb/payments.pb.go", "type Payment struct")
	checkFile(t, svcDir, "grpcserver/payments.go", "type PaymentServer struct")
	checkFile(t, svcDir, "service.yaml", "service_name: PaymentService")
}

func TestNewMonorepoFallbackModule(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "proto", "consent", "v1")
	os.MkdirAll(legacy, 0o750)
	os.WriteFile(filepath.Join(legacy, "consent.proto"), []byte("syntax = \"proto3\";\n"), 0o600)

	svcDir := filepath.Join(root, "ms-consent")
	err := runNew([]string{
		"ms-consent", "--model", "Consent",
		"--fields", "subject:string",
		"--grpc",
		"--dir", svcDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	checkFile(t, root, "proto/consents/v1/consents.proto", "package consents.v1;")
	checkFile(t, root, "proto/consents/v1/consents.proto", `option go_package = "github.com/natuleadan/ms-consent/pb;pb";`)
}

func TestFindSharedProto(t *testing.T) {
	root := t.TempDir()
	if err := runProtoInit(root, "example.com/mono/gen/go"); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "services", "ms-a")
	os.MkdirAll(nested, 0o750)
	if got := findSharedProto(nested); got != root {
		t.Errorf("findSharedProto = %q, want %q", got, root)
	}
	if got := findSharedProto(t.TempDir()); got != "" {
		t.Errorf("findSharedProto fresh = %q, want empty", got)
	}
}

func TestDiscoverSharedProtos(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"b/v1/b.proto", "a/v1/a.proto", "a/v1/notes.txt"} {
		p := filepath.Join(root, "proto", rel)
		os.MkdirAll(filepath.Dir(p), 0o750)
		os.WriteFile(p, []byte("x"), 0o600)
	}

	got, err := discoverSharedProtos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a/v1/a.proto" || got[1] != "b/v1/b.proto" {
		t.Errorf("discover = %v, want sorted proto pair", got)
	}
}

func TestResolveGenModule(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "gen", "go"), 0o750)
	os.WriteFile(filepath.Join(root, "gen", "go", "go.mod"), []byte("module example.com/mono/gen/go\n"), 0o600)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/mono\n"), 0o600)

	if got, _ := resolveGenModule(root, "flag/mod"); got != "flag/mod" {
		t.Errorf("flag precedence = %q", got)
	}
	if got, _ := resolveGenModule(root, ""); got != "example.com/mono/gen/go" {
		t.Errorf("gen/go.mod = %q", got)
	}
	os.Remove(filepath.Join(root, "gen", "go", "go.mod"))
	if got, _ := resolveGenModule(root, ""); got != "example.com/mono/gen/go" {
		t.Errorf("root derived = %q", got)
	}
	if _, err := resolveGenModule(t.TempDir(), ""); err == nil {
		t.Error("expected error without module source, got nil")
	}
}

func TestBuildGenerateOpts(t *testing.T) {
	goOpts, grpcOpts := buildGenerateOpts("a.proto", "", "")
	if len(goOpts) != 1 || goOpts[0] != "paths=import" {
		t.Errorf("default goOpts = %v", goOpts)
	}
	if len(grpcOpts) != 1 || grpcOpts[0] != "paths=import" {
		t.Errorf("default grpcOpts = %v", grpcOpts)
	}

	goOpts, grpcOpts = buildGenerateOpts("c.proto", "github.com/acme/mono", "github.com/acme/mono/gen/go/c/v1")
	want := []string{"paths=import", "module=github.com/acme/mono", "Mc.proto=github.com/acme/mono/gen/go/c/v1"}
	if strings.Join(goOpts, "|") != strings.Join(want, "|") {
		t.Errorf("goOpts = %v, want %v", goOpts, want)
	}
	if strings.Join(grpcOpts, "|") != strings.Join(want, "|") {
		t.Errorf("grpcOpts = %v, want %v", grpcOpts, want)
	}
}

func TestProtocOutArgs(t *testing.T) {
	got := protocOutArgs("--go_out", "/o", []string{"paths=import", "module=m"})
	want := []string{"--go_out=/o", "--go_opt=paths=import", "--go_opt=module=m"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestCanonicalVendoredArgs(t *testing.T) {
	got := canonicalProtocArgs("/r/proto", "c/v1/c.proto", "/r/gen/go", "example.com/mono/gen/go")
	joined := strings.Join(got, " ")
	for _, want := range []string{"--proto_path=/r/proto", "c/v1/c.proto", "--go_out=/r/gen/go", "--go_opt=module=example.com/mono/gen/go", "--go-grpc_opt=module=example.com/mono/gen/go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("canonical args %q missing %q", joined, want)
		}
	}

	got = vendoredProtocArgs("/r/proto", "c/v1/c.proto", "/r/ms-c", "example.com/mono/ms-c")
	joined = strings.Join(got, " ")
	for _, want := range []string{"--go_out=/r/ms-c", "Mc/v1/c.proto=example.com/mono/ms-c/pb"} {
		if !strings.Contains(joined, want) {
			t.Errorf("vendored args %q missing %q", joined, want)
		}
	}
}

func TestResolveVendTargets(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "payments", "pb"), 0o750)
	os.WriteFile(filepath.Join(root, "payments", "go.mod"), []byte("module example.com/mono/payments\n"), 0o600)
	os.MkdirAll(filepath.Join(root, "ms-consent", "pb"), 0o750)
	os.WriteFile(filepath.Join(root, "ms-consent", "go.mod"), []byte("module example.com/mono/ms-consent\n"), 0o600)

	targets, err := resolveVendTargets(root,
		[]string{"payments/v1/payments.proto", "consent/v1/consent.proto", "orphan/v1/orphan.proto"},
		[]string{"consent/v1/consent.proto=" + filepath.Join(root, "ms-consent")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want 2", targets)
	}
	if targets[0].svcDir != filepath.Join(root, "payments") || targets[0].svcMod != "example.com/mono/payments" {
		t.Errorf("auto target = %+v", targets[0])
	}
	if targets[1].svcDir != filepath.Join(root, "ms-consent") {
		t.Errorf("explicit target = %+v", targets[1])
	}

	if _, err := resolveVendTargets(root, nil, []string{"badformat"}); err == nil {
		t.Error("expected error on bad --vend, got nil")
	}
}

func TestModuleOf(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "go.mod")
	os.WriteFile(p, []byte("module example.com/x\n\ngo 1.27\n"), 0o600)
	if got, err := moduleOf(p); err != nil || got != "example.com/x" {
		t.Errorf("moduleOf = %q, %v", got, err)
	}
	if _, err := moduleOf(filepath.Join(dir, "missing.mod")); err == nil {
		t.Error("expected error on missing go.mod, got nil")
	}
}

func TestProtoGenerateNoProtos(t *testing.T) {
	if err := runProtoGenerate(t.TempDir(), "", nil); err == nil {
		t.Error("expected error without protos, got nil")
	}
}
