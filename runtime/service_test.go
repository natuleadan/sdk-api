package runtime

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/events"

	"github.com/natuleadan/sdk-api/infra/logx"
)

func TestNew_LoadsConfig(t *testing.T) {
	svc, err := New("testdata/service_v2.yaml")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.config.Name != "order-service" {
		t.Errorf("Name = %q", svc.config.Name)
	}
	if svc.config.Port != 9090 {
		t.Errorf("Port = %d", svc.config.Port)
	}
}

func TestService_WithCRUD(t *testing.T) {
	svc := &Service{
		config:   &ServiceConfig{Name: "test", Port: 19001},
		handlers: &EntryHandlers{},
	}
	provider := &mockCRUDProvider{}
	svc.WithCRUD("Product", provider)

	if svc.handlers.CRUD == nil {
		t.Fatal("CRUD handlers nil")
	}
	if svc.handlers.CRUD["Product"] == nil {
		t.Error("Product not registered")
	}
}

func TestService_WithRest(t *testing.T) {
	svc := &Service{
		config:   &ServiceConfig{Name: "test", Port: 19002},
		handlers: &EntryHandlers{},
	}
	fn := func(c *RestCtx) error { return nil }
	svc.WithRest("myHandler", fn)

	if svc.handlers.Rest["myHandler"] == nil {
		t.Error("handler not registered")
	}
}

func TestService_WithWS(t *testing.T) {
	svc := &Service{
		config:   &ServiceConfig{Name: "test", Port: 19003},
		handlers: &EntryHandlers{},
	}
	svc.WithWS("onChat", func(ctx context.Context, conn *websocket.Conn) error { return nil })

	if svc.handlers.WS["onChat"] == nil {
		t.Error("WS handler not registered")
	}
}

func TestService_WithSSE(t *testing.T) {
	svc := &Service{
		config:   &ServiceConfig{Name: "test", Port: 19004},
		handlers: &EntryHandlers{},
	}
	svc.WithSSE("onStream", func(ctx context.Context, send func(data string)) error { return nil })

	if svc.handlers.SSE["onStream"] == nil {
		t.Error("SSE handler not registered")
	}
}

func TestService_Run_Minimal(t *testing.T) {
	app := fiber.New()
	app.Get("/ping", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/ping", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("App.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	t.Logf("response: %s", body)
}

func TestService_Run_WithCRUDEntry(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: crud-test
port: 19011
entry:
  - type: crud
    model: Product
    db: pg
    table: products
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithCRUD("Product", &mockCRUDProvider{})

	// Init server and register routes directly (skip DB/NATS init)
	svc.config.Server = ServerConf{
		Host:      "0.0.0.0",
		APIPrefix: "/api/v1",
		MaxConns:  1000,
	}
	svc.initServer()
	err = RegisterEntries(svc.srv.App(), svc.config, svc.handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// Test via httptest
	app := svc.srv.App()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/products", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestService_Run_MixedEntryTypes(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: mixed
port: 19012
entry:
  - type: crud
    model: Product
    db: pg
    table: products
  - type: rest
    method: GET
    path: /hello
    handler: sayHello
  - type: webhook
    path: /webhooks/test
    handler: onWebhook
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithCRUD("Product", &mockCRUDProvider{})
	svc.WithRest("sayHello", func(c *RestCtx) error {
		return c.JSON(fiber.Map{"hello": "world"})
	})
	svc.WithRest("onWebhook", func(c *RestCtx) error {
		return c.JSON(fiber.Map{"webhook": true})
	})

	svc.config.Server = ServerConf{
		Host:      "0.0.0.0",
		APIPrefix: "/api/v1",
		MaxConns:  1000,
	}
	svc.initServer()
	err = RegisterEntries(svc.srv.App(), svc.config, svc.handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	app := svc.srv.App()

	// Test CRUD
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/products", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("CRUD GET = %d", resp.StatusCode)
	}

	// Test REST
	req = httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/hello", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("REST GET = %d", resp.StatusCode)
	}

	// Test Webhook
	req = httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/webhooks/test", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("Webhook POST = %d", resp.StatusCode)
	}
}

func TestService_PoolResolution(t *testing.T) {
	svc := &Service{
		config: &ServiceConfig{Name: "test", Port: 19050},
		pools:  map[string]any{"pg-main": "pool-ref"},
	}

	if svc.Pool("pg-main") != "pool-ref" {
		t.Error("pool not found")
	}
	if svc.Pool("nonexistent") != nil {
		t.Error("nonexistent pool should be nil")
	}
}

func TestService_WithoutDB(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: no-db
port: 19020
server:
  max_conns: 1000
  middleware:
    - path: "/*"
      apply: []
entry:
  - type: rest
    method: GET
    path: /status
    handler: status
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithRest("status", func(c *RestCtx) error {
		return c.SendString("no-db works")
	})

	if len(svc.config.Databases) > 0 {
		t.Error("should have no databases")
	}

	svc.config.Server = ServerConf{
		Host:      "0.0.0.0",
		APIPrefix: "/api/v1",
		MaxConns:  1000,
	}
	svc.initServer()
	RegisterEntries(svc.srv.App(), svc.config, svc.handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)

	app := svc.srv.App()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/status", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestService_Shutdown(t *testing.T) {
	svc := &Service{
		config:    &ServiceConfig{Name: "test", Port: 19099},
		pools:     make(map[string]any),
		natsConns: make(map[string]events.EventBroker),
	}

	// shutdown on empty state should not panic
	svc.shutdown()
}

func TestTableFor(t *testing.T) {
	// Test error case — pool not found
	pools := map[string]any{}
	_, err := TableFor[map[string]any](pools, "missing", "test")
	if err == nil {
		t.Error("expected error for missing pool")
	}

	// Test wrong type — sql.DB instead of pgxpool
	var sqlDB any
	pools["sql"] = sqlDB
	_, err = TableFor[map[string]any](pools, "sql", "test")
	if err == nil {
		t.Error("expected error for non-pgx pool")
	}
}

func TestInitLogger_NilConfig(t *testing.T) {
	logx.Disable()
	cfg := &ServiceConfig{Name: "test-svc"}
	s, err := newFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.initLogger(); err != nil {
		t.Errorf("expected nil error for nil Log config, got %v", err)
	}
}

func TestInitLogger_WithConfig(t *testing.T) {
	logx.Disable()
	cfg := &ServiceConfig{
		Name: "my-app",
		Log: &logx.LogConf{
			Level:    "error",
			Encoding: "plain",
		},
	}
	s, err := newFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.initLogger(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cfg.Log.ServiceName != "my-app" {
		t.Errorf("expected ServiceName to be filled from Name, got %q", cfg.Log.ServiceName)
	}
}

func TestSetCORSOriginsFunc_Global(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: cors-test
port: 19012
server:
  cors:
    origins: ["https://app.example.com"]
entry:
  - type: rest
    method: GET
    path: /ping
    handler: ping
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithRest("ping", func(c *RestCtx) error { return c.SendString("ok") })
	svc.SetCORSOriginsFunc("", func(origin string) bool {
		return origin == "https://dynamic.example.com"
	})
	svc.config.Server.Host = "0.0.0.0"
	svc.config.Server.APIPrefix = "/api"
	svc.config.Server.MaxConns = 1000
	svc.initServer()

	app := svc.srv.App()
	// Preflight from the dynamic origin (allowed by func)
	req := httptest.NewRequestWithContext(context.Background(), "OPTIONS", "/api/ping", nil)
	req.Header.Set("Origin", "https://dynamic.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://dynamic.example.com" {
		t.Errorf("global func: want dynamic origin echo, got %q", got)
	}

	// Preflight from a random origin (rejected by func)
	req2 := httptest.NewRequestWithContext(context.Background(), "OPTIONS", "/api/ping", nil)
	req2.Header.Set("Origin", "https://evil.com")
	req2.Header.Set("Access-Control-Request-Method", "GET")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request2: %v", err)
	}
	if got := resp2.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("global func: evil origin must be rejected, got %q", got)
	}
}

func TestSetCORSOriginsFunc_Group(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: cors-test
port: 19013
server:
  cors_groups:
    - name: app
      origins: ["https://app.example.com"]
      methods: [GET]
entry:
  - type: rest
    method: GET
    path: /ping
    handler: ping
    cors: app
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithRest("ping", func(c *RestCtx) error { return c.SendString("ok") })
	svc.SetCORSOriginsFunc("app", func(origin string) bool {
		return strings.HasSuffix(origin, ".trusted.example.com")
	})
	// Preserve YAML-loaded CORSGroups; only fix the fields initServer needs.
	svc.config.Server.Host = "0.0.0.0"
	svc.config.Server.APIPrefix = "/api"
	svc.config.Server.MaxConns = 1000
	svc.initServer()
	if err := RegisterEntries(svc.srv.App(), svc.config, svc.handlers, "/api", nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	app := svc.srv.App()
	// Allowed by YAML origin
	req := httptest.NewRequestWithContext(context.Background(), "OPTIONS", "/api/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("group yaml origin: want app echo, got %q", got)
	}

	// Allowed by func (trusted subdomain)
	req2 := httptest.NewRequestWithContext(context.Background(), "OPTIONS", "/api/ping", nil)
	req2.Header.Set("Origin", "https://x.trusted.example.com")
	req2.Header.Set("Access-Control-Request-Method", "GET")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request2: %v", err)
	}
	if got := resp2.Header.Get("Access-Control-Allow-Origin"); got != "https://x.trusted.example.com" {
		t.Errorf("group func: want trusted echo, got %q", got)
	}

	// Rejected (neither YAML nor func)
	req3 := httptest.NewRequestWithContext(context.Background(), "OPTIONS", "/api/ping", nil)
	req3.Header.Set("Origin", "https://evil.com")
	req3.Header.Set("Access-Control-Request-Method", "GET")
	resp3, err := app.Test(req3)
	if err != nil {
		t.Fatalf("request3: %v", err)
	}
	if got := resp3.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("group: evil origin must be rejected, got %q", got)
	}
}

func TestSetCORSOriginsFunc_UnknownGroup_NoPanic(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "service.yaml")
	content := `name: cors-test
port: 19014
server:
  cors_groups:
    - name: app
      origins: ["https://app.example.com"]
`
	os.WriteFile(yamlPath, []byte(content), 0o644)

	svc, err := New(yamlPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Group "missing" does not exist -> must not panic, just log
	svc.SetCORSOriginsFunc("missing", func(origin string) bool { return false })
	svc.config.Server = ServerConf{Host: "0.0.0.0", MaxConns: 1000}
	svc.initServer() // should not panic
}
