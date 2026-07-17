package server

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"testing"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"golang.org/x/crypto/acme/autocert"
)

func testRequest(path string) *http.Request {
	req, err := http.NewRequestWithContext(context.Background(), "GET", path, nil)
	if err != nil {
		panic(err)
	}
	req.Host = "test.com"
	return req
}

func TestHealthEndpoint(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)

	req := testRequest("/health")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestErrorHandler(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/error", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "something went wrong")
	})

	req := testRequest("/error")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != 500 {
		t.Errorf("expected code 500, got %d", errResp.Code)
	}
}

func TestCustomRoute(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/api/v1/hello", func(c fiber.Ctx) error {
		return c.JSON(map[string]string{"message": "hello"})
	})

	req := testRequest("/api/v1/hello")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	json.Unmarshal(body, &result)
	if result["message"] != "hello" {
		t.Errorf("expected message=hello, got %q", result["message"])
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	})

	req := testRequest("/panic")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 (recovered), got %d", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)

	req := testRequest("/nonexistent")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTLS_Disabled(t *testing.T) {
	logx.Disable()
	cfg := DefaultConfig()
	cfg.TLS = nil
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
	// HTTP should work normally
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("pong")
	})
	req := testRequest("/ping")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTLS_Config_Manual(t *testing.T) {
	logx.Disable()
	cfg := DefaultConfig()
	cfg.TLS = &TLSConfig{
		Enabled:    true,
		MinVersion: "1.2",
		MaxVersion: "1.3",
		Manual: &ManualTLS{
			CertFile: "/tmp/test.crt",
			KeyFile:  "/tmp/test.key",
		},
	}
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
	if app.config.TLS == nil {
		t.Fatal("TLS config should not be nil")
	}
	if app.config.TLS.Manual == nil {
		t.Fatal("Manual TLS config should not be nil")
	}
	if app.config.TLS.Manual.CertFile != "/tmp/test.crt" {
		t.Errorf("CertFile = %q", app.config.TLS.Manual.CertFile)
	}
}

func TestTLS_Config_Autocert(t *testing.T) {
	logx.Disable()
	cfg := DefaultConfig()
	cfg.TLS = &TLSConfig{
		Enabled: true,
		Autocert: &AutocertTLS{
			Domains:  []string{"api.example.com"},
			Email:    "admin@example.com",
			CacheDir: "/var/cache/autocert",
		},
	}
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
	if app.config.TLS == nil {
		t.Fatal("TLS config should not be nil")
	}
	if app.config.TLS.Autocert == nil {
		t.Fatal("Autocert config should not be nil")
	}
	if len(app.config.TLS.Autocert.Domains) != 1 || app.config.TLS.Autocert.Domains[0] != "api.example.com" {
		t.Errorf("Domains = %v", app.config.TLS.Autocert.Domains)
	}
}

func TestTLS_ParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  uint16
	}{
		{"1.0", tls.VersionTLS10},
		{"1.1", tls.VersionTLS11},
		{"1.2", tls.VersionTLS12},
		{"1.3", tls.VersionTLS13},
		{"", tls.VersionTLS12},
		{"invalid", tls.VersionTLS12},
	}
	for _, tt := range tests {
		got := parseTLSVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseTLSVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestTLS_ParseCurves(t *testing.T) {
	curves := parseCurves([]string{"X25519", "P-256"})
	if len(curves) != 2 {
		t.Fatalf("expected 2 curves, got %d", len(curves))
	}
	if curves[0] != tls.X25519 {
		t.Errorf("expected X25519")
	}
	if curves[1] != tls.CurveP256 {
		t.Errorf("expected P-256")
	}
}

func TestTLS_ParseCiphers(t *testing.T) {
	ciphers := parseCiphers([]string{
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"TLS_AES_256_GCM_SHA384",
	})
	if len(ciphers) != 2 {
		t.Fatalf("expected 2 ciphers, got %d", len(ciphers))
	}
}

func TestTLS_AutocertManager(t *testing.T) {
	logx.Disable()
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("api.example.com"),
		Email:      "admin@example.com",
	}
	tlsCfg := m.TLSConfig()
	if tlsCfg == nil {
		t.Fatal("autocert TLS config should not be nil")
	}
	if tlsCfg.GetCertificate == nil {
		t.Error("expected GetCertificate callback")
	}
}

// --- Error Sanitization Tests ---

func TestErrorHandler_SanitizesInternalError(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/db-error", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "dial tcp 10.0.0.5:5432: connection refused")
	})

	req := testRequest("/db-error")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	if errResp.Code != 500 {
		t.Errorf("expected code 500, got %d", errResp.Code)
	}
	if errResp.Message != "internal server error" {
		t.Errorf("expected sanitized message, got %q", errResp.Message)
	}
}

func TestErrorHandler_LeavesClientErrors(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/bad-request", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "invalid input")
	})

	req := testRequest("/bad-request")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	if errResp.Code != 400 {
		t.Errorf("expected code 400, got %d", errResp.Code)
	}
	if errResp.Message != "invalid input" {
		t.Errorf("expected original message, got %q", errResp.Message)
	}
}

func TestSanitizeErrorMessage_4xxWithIP(t *testing.T) {
	msg := sanitizeErrorMessage("dial tcp 10.0.0.5:5432: timeout", 400)
	if msg != "dial tcp [redacted]:5432: timeout" {
		t.Errorf("expected IP redacted, got %q", msg)
	}
}

func TestSanitizeErrorMessage_4xxWithConnString(t *testing.T) {
	msg := sanitizeErrorMessage("connection to postgres://admin:pass@db:5432/mydb failed", 400)
	if msg != "connection to postgres://[redacted]@db:5432/mydb failed" {
		t.Errorf("expected conn string redacted, got %q", msg)
	}

	msg = sanitizeErrorMessage("nats://user:secret@nats:4222: connection refused", 400)
	if msg != "nats://[redacted]@nats:4222: connection refused" {
		t.Errorf("expected nats conn string redacted, got %q", msg)
	}
}

func TestSanitizeErrorMessage_4xxWithFilePath(t *testing.T) {
	msg := sanitizeErrorMessage("config not found at /etc/sdk-api/service.yaml", 400)
	if msg != "config not found at [redacted]" {
		t.Errorf("expected file path redacted, got %q", msg)
	}
}

func TestSanitizeErrorMessage_4xxNormal(t *testing.T) {
	msg := sanitizeErrorMessage("invalid email format", 400)
	if msg != "invalid email format" {
		t.Errorf("expected message unchanged, got %q", msg)
	}

	msg = sanitizeErrorMessage("product name is required", 422)
	if msg != "product name is required" {
		t.Errorf("expected message unchanged, got %q", msg)
	}
}

func TestSanitizeErrorMessage_5xxAlways(t *testing.T) {
	msg := sanitizeErrorMessage("dial tcp 10.0.0.5:5432: timeout", 500)
	if msg != "internal server error" {
		t.Errorf("expected internal server error for 5xx, got %q", msg)
	}

	msg = sanitizeErrorMessage("something went wrong", 503)
	if msg != "internal server error" {
		t.Errorf("expected internal server error for 5xx, got %q", msg)
	}
}
