package server

import (
	"crypto/tls"
	"github.com/goccy/go-json"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
	"golang.org/x/crypto/acme/autocert"
)

func TestHealthEndpoint(t *testing.T) {
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)

	req, _ := http.NewRequest("GET", "/health", nil)
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
	app.app.Get("/error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "something went wrong")
	})

	req, _ := http.NewRequest("GET", "/error", nil)
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
	app.app.Get("/api/v1/hello", func(c *fiber.Ctx) error {
		return c.JSON(map[string]string{"message": "hello"})
	})

	req, _ := http.NewRequest("GET", "/api/v1/hello", nil)
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
	app.app.Get("/panic", func(c *fiber.Ctx) error {
		panic("test panic")
	})

	req, _ := http.NewRequest("GET", "/panic", nil)
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

	req, _ := http.NewRequest("GET", "/nonexistent", nil)
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
	app.app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})
	req, _ := http.NewRequest("GET", "/ping", nil)
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
		Enabled:  true,
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
	app.app.Get("/db-error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "dial tcp 10.0.0.5:5432: connection refused")
	})

	req, _ := http.NewRequest("GET", "/db-error", nil)
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
	app.app.Get("/bad-request", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "invalid input")
	})

	req, _ := http.NewRequest("GET", "/bad-request", nil)
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
