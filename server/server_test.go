package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime/errcode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)

	req := testRequest("/health")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestErrorHandler(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/error", func(_ fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "something went wrong")
	})

	req := testRequest("/error")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	assert.Equal(t, 500, errResp.Code)
}

func TestCustomRoute(t *testing.T) {
	t.Parallel()
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
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	json.Unmarshal(body, &result)
	if result["message"] != "hello" {
		t.Errorf("expected message=hello, got %q", result["message"])
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.Equal(t, 8080, cfg.Port)
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/panic", func(_ fiber.Ctx) error {
		panic("test panic")
	})

	req := testRequest("/panic")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)

	req := testRequest("/nonexistent")
	resp, err := app.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTLS_Disabled(t *testing.T) {
	t.Parallel()
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
	assert.Equal(t, 200, resp.StatusCode)
}

func TestTLS_Config_Manual(t *testing.T) {
	t.Parallel()
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
		require.FailNow(t, "TLS config should not be nil")
	}
	if app.config.TLS.Manual == nil {
		require.FailNow(t, "Manual TLS config should not be nil")
	}
	if app.config.TLS.Manual.CertFile != "/tmp/test.crt" {
		t.Errorf("CertFile = %q", app.config.TLS.Manual.CertFile)
	}
}

func TestTLS_Config_Autocert(t *testing.T) {
	t.Parallel()
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
		require.FailNow(t, "TLS config should not be nil")
	}
	if app.config.TLS.Autocert == nil {
		require.FailNow(t, "Autocert config should not be nil")
	}
	if len(app.config.TLS.Autocert.Domains) != 1 || app.config.TLS.Autocert.Domains[0] != "api.example.com" {
		t.Errorf("Domains = %v", app.config.TLS.Autocert.Domains)
	}
}

func TestTLS_ParseVersion(t *testing.T) {
	t.Parallel()
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
		assert.Equal(t, tt.want, got)
	}
}

func TestTLS_ParseCurves(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ciphers := parseCiphers([]string{
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		"TLS_AES_256_GCM_SHA384",
	})
	if len(ciphers) != 2 {
		t.Fatalf("expected 2 ciphers, got %d", len(ciphers))
	}
}

func TestTLS_AutocertManager(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/db-error", func(_ fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "dial tcp 10.0.0.5:5432: connection refused")
	})

	req := testRequest("/db-error")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 500, errResp.Code)
	if errResp.Message != "internal server error" {
		t.Errorf("expected sanitized message, got %q", errResp.Message)
	}
}

func TestErrorHandler_LeavesClientErrors(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/bad-request", func(_ fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "invalid input")
	})

	req := testRequest("/bad-request")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 400, errResp.Code)
	if errResp.Message != "invalid input" {
		t.Errorf("expected original message, got %q", errResp.Message)
	}
}

func TestOopsErrorHandler_4xxWithCode(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/unauthorized", func(_ fiber.Ctx) error {
		return errcode.ErrUnauthorized("missing token")
	})

	req := testRequest("/unauthorized")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 401, errResp.Code)
	assert.Equal(t, errcode.ErrCodeUnauthorized, errResp.Error)
	if errResp.Message != "missing token" {
		t.Errorf("expected message %q, got %q", "missing token", errResp.Message)
	}
}

func TestOopsErrorHandler_5xxWithCode(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/db-error", func(_ fiber.Ctx) error {
		return errcode.ErrDBQuery("select", "users", fmt.Errorf("connection refused"))
	})

	req := testRequest("/db-error")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 500, errResp.Code)
	assert.Equal(t, errcode.ErrCodeDBQuery, errResp.Error)
	if errResp.Message != "internal server error" {
		t.Errorf("expected 'internal server error', got %q", errResp.Message)
	}
}

func TestOopsErrorHandler_404WithCode(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/not-found", func(_ fiber.Ctx) error {
		return errcode.ErrNotFound("product", "abc-123")
	})

	req := testRequest("/not-found")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 404, errResp.Code)
	assert.Equal(t, errcode.ErrCodeNotFound, errResp.Error)
	if errResp.Message != "resource not found" {
		t.Errorf("expected 'resource not found', got %q", errResp.Message)
	}
}

func TestOopsErrorHandler_FallbackToFiberError(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/fiber-err", func(_ fiber.Ctx) error {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "invalid input")
	})

	req := testRequest("/fiber-err")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 422, errResp.Code)
	if errResp.Message != "invalid input" {
		t.Errorf("expected message 'invalid input', got %q", errResp.Message)
	}
}

func TestOopsErrorHandler_RateLimited(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/rate-limited", func(_ fiber.Ctx) error {
		return errcode.ErrRateLimited(5)
	})

	req := testRequest("/rate-limited")
	resp, _ := app.app.Test(req)

	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, errcode.ErrCodeRateLimited, errResp.Error)
}
