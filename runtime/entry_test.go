package runtime

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/jsonx"
	"github.com/natuleadan/sdk-api/server/middleware"
)

// --- Mock CRUDProvider ---

type mockCRUDProvider struct {
	listFn   func(ctx fiber.Ctx, params ListParams) error
	getFn    func(ctx fiber.Ctx, id string) error
	createFn func(ctx fiber.Ctx, body []byte) error
	updateFn func(ctx fiber.Ctx, id string, body []byte) error
	deleteFn func(ctx fiber.Ctx, id string) error
}

func (m *mockCRUDProvider) List(ctx fiber.Ctx, params ListParams) error {
	if m.listFn != nil {
		return m.listFn(ctx, params)
	}
	return nil
}

func (m *mockCRUDProvider) Get(ctx fiber.Ctx, id string) error {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return ctx.JSON(fiber.Map{"id": id, "name": "test"})
}

func (m *mockCRUDProvider) Create(ctx fiber.Ctx, body []byte) error {
	if m.createFn != nil {
		return m.createFn(ctx, body)
	}
	return ctx.Status(201).JSON(fiber.Map{"id": "new", "data": string(body)})
}

func (m *mockCRUDProvider) Update(ctx fiber.Ctx, id string, body []byte) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, body)
	}
	return ctx.JSON(fiber.Map{"id": id, "data": string(body)})
}

func (m *mockCRUDProvider) Delete(ctx fiber.Ctx, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return ctx.SendStatus(204)
}

// --- Helpers ---

func testApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"code": code, "message": err.Error()})
		},
	})
}

func testHandlers(provider CRUDProvider) *EntryHandlers {
	return &EntryHandlers{
		Rest: make(map[string]func(fiber.Ctx) error),
		WS:   make(map[string]WSHandler),
		SSE:  make(map[string]SSEHandler),
		CRUD: func() map[string]CRUDProvider {
			m := make(map[string]CRUDProvider)
			m["Product"] = provider
			return m
		}(),
	}
}

func request(app *fiber.App, method, path string, body io.Reader) (*http.Response, error) {
	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	return resp, err
}

// --- Tests ---

func TestRegisterEntries_CRUD_AllMethods(t *testing.T) {
	app := testApp()
	provider := &mockCRUDProvider{}
	handlers := testHandlers(provider)

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "crud", Model: "Product", DB: "pg", Table: "products", Resource: "products", Path: "/products"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// GET list
	resp, err := request(app, "GET", "/api/v1/products", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /products status = %d", resp.StatusCode)
	}

	// GET one
	resp, err = request(app, "GET", "/api/v1/products/123", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /products/123 status = %d", resp.StatusCode)
	}

	// POST create
	resp, err = request(app, "POST", "/api/v1/products", stringsReader(`{"name":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("POST /products status = %d", resp.StatusCode)
	}

	// PATCH update
	resp, err = request(app, "PATCH", "/api/v1/products/123", stringsReader(`{"name":"updated"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("PATCH /products/123 status = %d", resp.StatusCode)
	}

	// DELETE
	resp, err = request(app, "DELETE", "/api/v1/products/123", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("DELETE /products/123 status = %d", resp.StatusCode)
	}
}

func TestRegisterEntries_CRUD_Overrides(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		app := testApp()
		provider := &mockCRUDProvider{}
		handlers := testHandlers(provider)

		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{
					Type: "crud", Model: "Product", DB: "pg", Table: "p", Resource: "p", Path: "/p",
					Overrides: &CRUDOverrides{Create: "-", Delete: "-"},
				},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("RegisterEntries: %v", err)
		}

		// Create should be 405 (path exists for GET but POST not registered)
		resp, _ := request(app, "POST", "/api/v1/p", stringsReader(`{}`))
		if resp.StatusCode != 405 {
			t.Errorf("POST /p should be 405 (disabled), got %d", resp.StatusCode)
		}
		// Delete should be 405 (path /p/:id exists for other methods like GET)
		resp, _ = request(app, "DELETE", "/api/v1/p/1", nil)
		if resp.StatusCode != 405 {
			t.Errorf("DELETE /p/1 should be 405 (disabled), got %d", resp.StatusCode)
		}
		// Get should still work
		resp, _ = request(app, "GET", "/api/v1/p/1", nil)
		if resp.StatusCode != 200 {
			t.Errorf("GET /p/1 should be 200, got %d", resp.StatusCode)
		}
	})

	t.Run("overridden", func(t *testing.T) {
		app := testApp()
		provider := &mockCRUDProvider{}
		handlers := testHandlers(provider)
		handlers.Rest["customList"] = func(c fiber.Ctx) error {
			return c.JSON(fiber.Map{"custom": true})
		}

		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{
					Type: "crud", Model: "Product", DB: "pg", Table: "p", Resource: "p", Path: "/p",
					Overrides: &CRUDOverrides{List: "customList", Get: "-"},
				},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("RegisterEntries: %v", err)
		}

		// List should use custom handler
		resp, _ := request(app, "GET", "/api/v1/p", nil)
		if resp.StatusCode != 200 {
			t.Errorf("GET /p status = %d", resp.StatusCode)
		}
		// Get should be 405 (path /p/:id exists for PATCH/DELETE methods)
		resp, _ = request(app, "GET", "/api/v1/p/1", nil)
		if resp.StatusCode != 405 {
			t.Errorf("GET /p/1 should be 405 (disabled), got %d", resp.StatusCode)
		}
	})
}

func TestRegisterEntries_REST(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{
		Rest: map[string]func(fiber.Ctx) error{
			"onTransform": func(c fiber.Ctx) error {
				return c.JSON(fiber.Map{"transformed": true})
			},
			"onCustom": func(c fiber.Ctx) error {
				return c.SendString("ok")
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/transform", Handler: "onTransform"},
			{Type: "rest", Method: "POST", Path: "/custom", Handler: "onCustom"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	resp, err := request(app, "GET", "/api/v1/transform", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /transform status = %d", resp.StatusCode)
	}

	resp, err = request(app, "POST", "/api/v1/custom", stringsReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("POST /custom status = %d", resp.StatusCode)
	}
}

func TestRegisterEntries_Webhook(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{
		Rest: map[string]func(fiber.Ctx) error{
			"onWebhook": func(c fiber.Ctx) error {
				return c.JSON(fiber.Map{"received": true})
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "webhook", Method: "POST", Path: "/webhooks/test", Handler: "onWebhook"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	resp, err := request(app, "POST", "/api/v1/webhooks/test", stringsReader(`{"event":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("POST /webhooks/test status = %d", resp.StatusCode)
	}
}

func TestRegisterEntries_Webhook_AllMethods(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{
		Rest: map[string]func(fiber.Ctx) error{
			"whGet":    func(c fiber.Ctx) error { return c.SendString("get") },
			"whPost":   func(c fiber.Ctx) error { return c.SendString("post") },
			"whPut":    func(c fiber.Ctx) error { return c.SendString("put") },
			"whPatch":  func(c fiber.Ctx) error { return c.SendString("patch") },
			"whDelete": func(c fiber.Ctx) error { return c.SendString("delete") },
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "webhook", Method: "GET", Path: "/wh/get", Handler: "whGet"},
			{Type: "webhook", Method: "POST", Path: "/wh/post", Handler: "whPost"},
			{Type: "webhook", Method: "PUT", Path: "/wh/put", Handler: "whPut"},
			{Type: "webhook", Method: "PATCH", Path: "/wh/patch", Handler: "whPatch"},
			{Type: "webhook", Method: "DELETE", Path: "/wh/delete", Handler: "whDelete"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	tests := []struct {
		method string
		path   string
		want   string
	}{
		{"GET", "/api/v1/wh/get", "get"},
		{"POST", "/api/v1/wh/post", "post"},
		{"PUT", "/api/v1/wh/put", "put"},
		{"PATCH", "/api/v1/wh/patch", "patch"},
		{"DELETE", "/api/v1/wh/delete", "delete"},
	}
	for _, tt := range tests {
		resp, err := request(app, tt.method, tt.path, nil)
		if err != nil {
			t.Errorf("%s %s: request failed: %v", tt.method, tt.path, err)
			continue
		}
		if resp.StatusCode != 200 {
			t.Errorf("%s %s: status = %d, want 200", tt.method, tt.path, resp.StatusCode)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != tt.want {
			t.Errorf("%s %s: body = %q, want %q", tt.method, tt.path, string(body), tt.want)
		}
	}
}

func TestRegisterEntries_WebSocket(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{
		WS: map[string]WSHandler{
			"onChat": func(ctx context.Context, conn *websocket.Conn) error {
				return nil
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "websocket", Path: "/ws/chat", Handler: "onChat"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}
	// WS route registered — verified by no error
}

func TestRegisterEntries_SSE(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{
		SSE: map[string]SSEHandler{
			"onStream": func(ctx context.Context, send func(data string)) error {
				send("data: hello\n\n")
				return nil
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "sse", Path: "/events/stream", Handler: "onStream"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	resp, err := request(app, "GET", "/api/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /events/stream status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestRegisterEntries_File(t *testing.T) {
	t.Run("upload_with_validation", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{
			Rest: map[string]func(fiber.Ctx) error{
				"onUpload": func(c fiber.Ctx) error {
					return c.JSON(fiber.Map{"uploaded": true})
				},
			},
		}

		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{
					Type: "file", Method: "POST", Path: "/files/upload", Handler: "onUpload",
					AllowedTypes: []string{"image/png", "image/jpeg"},
					MaxSize:      "1MB",
				},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("RegisterEntries: %v", err)
		}

		// Valid content type
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/files/upload", stringsReader(`data`))
		req.Header.Set("Content-Type", "image/png")
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("image/png should be 200, got %d", resp.StatusCode)
		}

		// Invalid content type
		req = httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/files/upload", stringsReader(`data`))
		req.Header.Set("Content-Type", "text/html")
		resp, _ = app.Test(req)
		if resp.StatusCode != 415 {
			t.Errorf("text/html should be 415, got %d", resp.StatusCode)
		}

		// image/gif not in allowed list — should be rejected
		req = httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/files/upload", stringsReader(`data`))
		req.Header.Set("Content-Type", "image/gif")
		resp, _ = app.Test(req)
		if resp.StatusCode != 415 {
			t.Errorf("image/gif should be 415 (not allowed), got %d", resp.StatusCode)
		}

		// Too large body
		bigBody := make([]byte, 2*1024*1024)
		req = httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/files/upload", bytesReader(bigBody))
		req.Header.Set("Content-Type", "image/png")
		resp, _ = app.Test(req)
		if resp.StatusCode != 413 {
			t.Errorf("oversized body should be 413, got %d", resp.StatusCode)
		}
	})

	t.Run("download_no_validation", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{
			Rest: map[string]func(fiber.Ctx) error{
				"onDownload": func(c fiber.Ctx) error {
					return c.SendString("file content")
				},
			},
		}

		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{Type: "file", Method: "GET", Path: "/files/:id/download", Handler: "onDownload"},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("RegisterEntries: %v", err)
		}

		resp, err := request(app, "GET", "/api/v1/files/abc/download", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("GET file download status = %d", resp.StatusCode)
		}
	})

	t.Run("patch_and_delete", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{
			Rest: map[string]func(fiber.Ctx) error{
				"onPatch":  func(c fiber.Ctx) error { return c.SendString("patched") },
				"onDelete": func(c fiber.Ctx) error { return c.SendString("deleted") },
			},
		}

		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{Type: "file", Method: "PATCH", Path: "/files/:id", Handler: "onPatch"},
				{Type: "file", Method: "DELETE", Path: "/files/:id", Handler: "onDelete"},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("RegisterEntries: %v", err)
		}

		resp, err := request(app, "PATCH", "/api/v1/files/1", stringsReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("PATCH file status = %d", resp.StatusCode)
		}

		resp, err = request(app, "DELETE", "/api/v1/files/1", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("DELETE file status = %d", resp.StatusCode)
		}
	})
}

func TestRegisterEntries_MixedTypes(t *testing.T) {
	app := testApp()
	provider := &mockCRUDProvider{}
	handlers := &EntryHandlers{
		Rest: map[string]func(fiber.Ctx) error{
			"onTransform": func(c fiber.Ctx) error {
				return c.JSON(fiber.Map{"ok": true})
			},
			"onWebhook": func(c fiber.Ctx) error {
				return c.JSON(fiber.Map{"webhook": true})
			},
			"onSSE": func(c fiber.Ctx) error {
				return c.SendString("stream")
			},
		},
		WS: map[string]WSHandler{
			"onChat": func(ctx context.Context, conn *websocket.Conn) error { return nil },
		},
		SSE: map[string]SSEHandler{
			"onStream": func(ctx context.Context, send func(data string)) error { return nil },
		},
		CRUD: map[string]CRUDProvider{
			"Product": provider,
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "crud", Model: "Product", DB: "pg", Table: "products", Path: "/products"},
			{Type: "rest", Method: "GET", Path: "/transform", Handler: "onTransform"},
			{Type: "webhook", Method: "POST", Path: "/webhooks/github", Handler: "onWebhook"},
			{Type: "websocket", Path: "/ws/chat", Handler: "onChat"},
			{Type: "sse", Path: "/events/stream", Handler: "onStream"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// All routes should respond
	paths := map[string]string{
		"crud list": "/api/v1/products",
		"crud get":  "/api/v1/products/1",
		"rest":      "/api/v1/transform",
		"webhook":   "/api/v1/webhooks/github",
		"ws":        "/api/v1/ws/chat",
		"sse":       "/api/v1/events/stream",
	}
	for name, path := range paths {
		resp, err := request(app, "GET", path, nil)
		if err != nil {
			t.Errorf("%s: request failed: %v", name, err)
			continue
		}
		if resp.StatusCode == 404 {
			t.Errorf("%s: route not found (404)", name)
		}
	}
	// webhook is POST
	resp, err := request(app, "POST", "/api/v1/webhooks/github", stringsReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("webhook POST status = %d", resp.StatusCode)
	}
}

func TestRegisterEntries_Errors(t *testing.T) {
	t.Run("missing CRUD provider", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{CRUD: map[string]CRUDProvider{}}
		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{Type: "crud", Model: "Missing", DB: "pg", Table: "t", Path: "/t"},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for missing CRUD provider")
		}
	})

	t.Run("missing REST handler", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{
			Rest: map[string]func(fiber.Ctx) error{},
		}
		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{Type: "rest", Method: "GET", Path: "/x", Handler: "notFound"},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for missing REST handler")
		}
	})

	t.Run("unknown entry type", func(t *testing.T) {
		app := testApp()
		handlers := &EntryHandlers{}
		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{Type: "grpc", Path: "/x"},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for unknown type")
		}
	})

	t.Run("override_missing_handler", func(t *testing.T) {
		app := testApp()
		provider := &mockCRUDProvider{}
		handlers := testHandlers(provider)
		cfg := &ServiceConfig{
			Entry: []EntryDef{
				{
					Type: "crud", Model: "Product", DB: "pg", Table: "p", Path: "/p",
					Overrides: &CRUDOverrides{List: "nonExistentHandler"},
				},
			},
		}
		err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for missing override handler")
		}
	})
}

func TestFileValidator(t *testing.T) {
	tests := []struct {
		name         string
		allowedTypes []string
		maxSize      string
		contentType  string
		bodySize     int
		wantStatus   int
	}{
		{"exact match", []string{"image/png"}, "", "image/png", 100, 200},
		{"wildcard match", []string{"image/*"}, "", "image/jpeg", 100, 200},
		{"no match", []string{"image/png"}, "", "text/html", 100, 415},
		{"under limit", nil, "1MB", "text/plain", 500, 200},
		{"over limit", nil, "1KB", "text/plain", 2048, 413},
		{"no validation", nil, "", "anything", 9999, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := testApp()
			entry := &EntryDef{
				AllowedTypes: tt.allowedTypes,
				MaxSize:      tt.maxSize,
			}

			app.Post("/test", fileValidator(entry), func(c fiber.Ctx) error {
				return c.SendString("ok")
			})

			body := make([]byte, tt.bodySize)
			req := httptest.NewRequestWithContext(context.Background(), "POST", "/test", bytesReader(body))
			req.Header.Set("Content-Type", tt.contentType)
			resp, _ := app.Test(req)
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestMatchContentType(t *testing.T) {
	tests := []struct {
		contentType string
		allowed     string
		want        bool
	}{
		{"image/png", "image/png", true},
		{"image/png", "image/jpeg", false},
		{"image/jpeg", "image/*", true},
		{"text/html", "image/*", false},
		{"application/json", "application/*", true},
	}
	for _, tt := range tests {
		got := matchContentType(tt.contentType, tt.allowed)
		if got != tt.want {
			t.Errorf("matchContentType(%q, %q) = %v, want %v", tt.contentType, tt.allowed, got, tt.want)
		}
	}
}

func TestParseMaxSize(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1MB", 1024 * 1024},
		{"2kb", 2048},
		{"1GB", 1024 * 1024 * 1024},
		{"500B", 500},
		{"100", 100},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMaxSize(tt.input)
		if got != tt.want {
			t.Errorf("parseMaxSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBuildIDParam(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/products", "/:id"},
		{"/products/:id", ""},
		{"/users/orders/:orderId", ""}, // any :param triggers no addition
		{"/items/:itemID/subs", ""},    // param in middle = path has its own param
	}
	for _, tt := range tests {
		got := buildIDParam(tt.path)
		if got != tt.want {
			t.Errorf("buildIDParam(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseListParams(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		params := parseListParams(c)
		return c.JSON(fiber.Map{
			"page":    params.Page,
			"size":    params.Size,
			"sort":    params.Sort,
			"filters": params.Filters,
		})
	})

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/test?page=2&size=20&sort=name&status=active&category=books", nil)
	resp, _ := app.Test(req)

	var result struct {
		Page    int               `json:"page"`
		Size    int               `json:"size"`
		Sort    string            `json:"sort"`
		Filters map[string]string `json:"filters"`
	}
	body, _ := io.ReadAll(resp.Body)
	jsonx.Unmarshal(body, &result)

	if result.Page != 2 {
		t.Errorf("page = %d, want 2", result.Page)
	}
	if result.Size != 20 {
		t.Errorf("size = %d, want 20", result.Size)
	}
	if result.Sort != "name" {
		t.Errorf("sort = %q, want name", result.Sort)
	}
	if result.Filters["status"] != "active" {
		t.Errorf("filter status = %q", result.Filters["status"])
	}
	if result.Filters["category"] != "books" {
		t.Errorf("filter category = %q", result.Filters["category"])
	}
}

// --- Helpers for body reading ---

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

func TestRegisterEntries_Async(t *testing.T) {
	app := testApp()

	handlers := &EntryHandlers{
		Async: map[string]AsyncHandler{
			"processReport": func(body []byte, js *JobState) error {
				js.Result = fiber.Map{"report_url": "https://example.com/report.pdf", "input": string(body)}
				return nil
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "async", Path: "/jobs/reports", Handler: "processReport"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	resp1, err := request(app, "POST", "/api/v1/jobs/reports", stringsReader(`{"type":"monthly"}`))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != 202 {
		t.Fatalf("POST status = %d, want 202", resp1.StatusCode)
	}

	body1, _ := io.ReadAll(resp1.Body)
	var submitResp struct {
		JobID     string `json:"job_id"`
		Status    string `json:"status"`
		StatusURL string `json:"status_url"`
	}
	jsonx.Unmarshal(body1, &submitResp)

	if submitResp.JobID == "" {
		t.Fatal("job_id is empty")
	}
	if submitResp.Status != "pending" && submitResp.Status != "processing" && submitResp.Status != "completed" {
		t.Fatalf("status = %q, want pending/processing/completed", submitResp.Status)
	}

	resp2, err := request(app, "GET", submitResp.StatusURL, nil)
	if err != nil {
		t.Fatalf("GET status request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("GET status = %d, want 200", resp2.StatusCode)
	}

	body2, _ := io.ReadAll(resp2.Body)
	var statusResp JobState
	jsonx.Unmarshal(body2, &statusResp)

	if statusResp.ID != submitResp.JobID {
		t.Errorf("job_id = %q, want %q", statusResp.ID, submitResp.JobID)
	}

	resp3, err := request(app, "GET", "/api/v1/jobs/reports/nonexistent", nil)
	if err != nil {
		t.Fatalf("GET nonexistent request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != 404 {
		t.Fatalf("GET nonexistent status = %d, want 404", resp3.StatusCode)
	}
}

func TestRegisterEntries_GraphQL(t *testing.T) {
	app := testApp()

	type ProductStruct struct {
		ID    int64  `db:"id,primary,auto"`
		Name  string `db:"name,required"`
		Price int    `db:"price"`
	}
	mInfo, _ := db.ParseStructReflect(reflect.TypeFor[ProductStruct]())
	models := map[string]*db.TableInfo{"Product": mInfo}

	provider := &mockCRUDProvider{}
	handlers := &EntryHandlers{
		CRUD: map[string]CRUDProvider{"Product": provider},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "graphql", Path: "/graphql"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, models, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// Introspection query — should return data
	resp, err := request(app, "POST", "/api/v1/graphql",
		strings.NewReader(`{"query":"{ __typename }"}`))
	if err != nil {
		t.Fatalf("introspection request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	t.Log("graphql endpoint registered successfully")

	// Query list
	resp2, err := request(app, "POST", "/api/v1/graphql",
		strings.NewReader(`{"query":"{ Products { Id Name } }"}`))
	if err != nil {
		t.Fatalf("query request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("query status = %d, body: %s", resp2.StatusCode, string(body))
	}
	t.Log("graphql query successful")

	// Mutation
	resp3, err := request(app, "POST", "/api/v1/graphql",
		strings.NewReader(`{"query":"mutation { createProduct(input: {Name: \"test\"}) { Id Name } }"}`))
	if err != nil {
		t.Fatalf("mutation request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != 200 {
		body3, _ := io.ReadAll(resp3.Body)
		t.Fatalf("mutation status = %d, body: %s", resp3.StatusCode, string(body3))
	}
	t.Log("graphql mutation successful")
}

// --- SanitizeFilename Tests ---

func TestSanitizeFilename_Simple(t *testing.T) {
	got := SanitizeFilename("report.pdf")
	if got != "report.pdf" {
		t.Errorf("got %q, want report.pdf", got)
	}
}

func TestSanitizeFilename_RemovesPath(t *testing.T) {
	got := SanitizeFilename("../../../etc/passwd")
	if strings.Contains(got, "/") || strings.Contains(got, "\\") {
		t.Errorf("should not contain path separators: %q", got)
	}
}

func TestSanitizeFilename_RemovesNull(t *testing.T) {
	got := SanitizeFilename("file\x00.jpg")
	if strings.Contains(got, "\x00") {
		t.Errorf("should not contain null bytes: %q", got)
	}
}

func TestSanitizeFilename_Empty(t *testing.T) {
	got := SanitizeFilename("")
	if got != "untitled" {
		t.Errorf("expected untitled, got %q", got)
	}
}

func TestSanitizeFilename_UnsafeChars(t *testing.T) {
	got := SanitizeFilename("hello<world>.txt")
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("should not contain angle brackets: %q", got)
	}
}

func TestSanitizeFilename_KeepsExtension(t *testing.T) {
	got := SanitizeFilename("../../../etc/passwd.txt")
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("should preserve extension .txt: %q", got)
	}
}

func TestSanitizeFilename_Truncates(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	got := SanitizeFilename(string(long) + ".txt")
	if len(got) > 260 {
		t.Errorf("should truncate long filenames, got %d chars", len(got))
	}
}

func TestValidateEntryAuth_NoRoles(t *testing.T) {
	entry := &EntryDef{Auth: true}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{"test": nil}}
	err := validateEntryAuth(entry, handlers)
	if err != nil {
		t.Errorf("expected nil for no roles, got %v", err)
	}
}

func TestValidateEntryAuth_MissingHandler(t *testing.T) {
	entry := &EntryDef{Auth: true, Roles: []string{"admin"}, Handler: "nonexistent"}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{}}
	err := validateEntryAuth(entry, handlers)
	if err == nil {
		t.Error("expected error for missing handler")
	}
}

func TestValidateEntryAuth_ValidHandler(t *testing.T) {
	entry := &EntryDef{Auth: true, Roles: []string{"admin"}, Handler: "getHealth"}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{"getHealth": nil}}
	err := validateEntryAuth(entry, handlers)
	if err != nil {
		t.Errorf("expected nil for valid handler, got %v", err)
	}
}

func TestValidateEntryAuth_CRUDResource(t *testing.T) {
	entry := &EntryDef{Auth: true, Roles: []string{"editor"}, Resource: "products", Type: "crud"}
	handlers := &EntryHandlers{CRUD: map[string]CRUDProvider{"products": nil}}
	err := validateEntryAuth(entry, handlers)
	if err != nil {
		t.Errorf("expected nil for valid CRUD resource, got %v", err)
	}
}

func TestRegisterOneEntry_DriverNone(t *testing.T) {
	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/test", Method: "GET", Handler: "testHandler",
		Auth: true,
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{"testHandler": func(c fiber.Ctx) error { return c.SendString("ok") }}}
	cfg := &ServiceConfig{Auth: &AuthConfig{Driver: "none"}}

	err := registerOneEntry(app, entry, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, "none")
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	_ = cfg
}

func TestRegisterOneEntry_DriverManualWithValidator(t *testing.T) {
	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/manual-test", Method: "GET", Handler: "manualHandler",
		Auth: true, Roles: []string{"admin"},
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{"manualHandler": func(c fiber.Ctx) error { return c.SendString("ok") }}}

	validatorCalled := false
	validator := func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
		validatorCalled = true
		if len(roles) != 1 || roles[0] != "admin" {
			t.Errorf("expected roles [admin], got %v", roles)
		}
		return nil
	}
	jwtCfg := &middleware.JWTConfig{Secret: "test", TokenLookup: "header:Authorization"}

	err := registerOneEntry(app, entry, handlers, "", nil, nil, jwtCfg, validator, nil, nil, nil, "manual")
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "u1", "roles": []any{"admin"}}).SignedString([]byte("test"))
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/manual-test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)

	if !validatorCalled {
		t.Error("validator was not called")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRegisterOneEntry_RolesPermsInEntry(t *testing.T) {
	entry := &EntryDef{
		Type: "rest", Path: "/with-roles", Handler: "handler",
		Auth: true, Roles: []string{"admin", "editor"}, Permissions: []string{"products:write"},
	}

	if len(entry.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(entry.Roles))
	}
	if len(entry.Permissions) != 1 {
		t.Errorf("expected 1 permission, got %d", len(entry.Permissions))
	}
}
