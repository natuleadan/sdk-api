package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

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
	cfg := DefaultConfig()
	cfg.Breaker = false
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
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
	cfg := DefaultConfig()
	cfg.Breaker = false
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
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

func TestJoinOrStar(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "*", joinOrStar(nil))
	assert.Equal(t, "*", joinOrStar([]string{}))
	assert.Equal(t, "a", joinOrStar([]string{"a"}))
	assert.Equal(t, "a, b", joinOrStar([]string{"a", "b"}))
}

func TestJoinOrDefault(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "def", joinOrDefault(nil, "def"))
	assert.Equal(t, "def", joinOrDefault([]string{}, "def"))
	assert.Equal(t, "a", joinOrDefault([]string{"a"}, "def"))
	assert.Equal(t, "a, b", joinOrDefault([]string{"a", "b"}, "def"))
}

func TestDuration(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5*time.Second, Duration("5s", time.Second))
	assert.Equal(t, time.Second, Duration("invalid", time.Second))
	assert.Equal(t, time.Second, Duration("", time.Second))
	assert.Equal(t, 100*time.Millisecond, Duration("100ms", time.Second))
}

func TestOopsCodeToHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code   string
		expect int
	}{
		{errcode.ErrCodeNotFound, 404},
		{errcode.ErrCodeValidation, 400},
		{errcode.ErrCodeUnauthorized, 401},
		{errcode.ErrCodeForbidden, 403},
		{errcode.ErrCodeRateLimited, 429},
		{errcode.ErrCodeTimeout, 504},
		{errcode.ErrCodeInternal, 500},
		{errcode.ErrCodeDBQuery, 500},
		{"UNKNOWN", 500},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, oopsCodeToHTTP(tt.code))
	}
}

func TestNew_ZeroPortUsesDefault(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(Config{}, TelemetryConfig{}, SecurityConfig{}, nil)
	assert.Equal(t, 8080, app.config.Port)
}

func TestNew_DefaultConfigValues(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 4*1024*1024, cfg.BodyLimit)
	assert.Equal(t, "/metrics", cfg.MetricsPath)
	assert.Equal(t, "/health", cfg.HealthPath)
	assert.True(t, cfg.Logger)
	assert.True(t, cfg.LoadShedding)
	assert.True(t, cfg.Breaker)
}

func TestTelemetry_Enabled(t *testing.T) {
	t.Parallel()
	logx.Disable()
	cfg := DefaultConfig()
	tel := TelemetryConfig{
		Enabled:             true,
		Name:                "test-svc",
		Endpoint:            "localhost:4317",
		Sampler:             1.0,
		Batcher:             "otlpgrpc",
		TraceResponseHeader: "X-Trace-Id",
		SkipPaths:           []string{"/health"},
	}
	app := New(cfg, tel, SecurityConfig{}, nil)
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest("/ping")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Trace-Id"))
}

func TestTelemetry_Disabled(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{Enabled: false}, SecurityConfig{}, nil)
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest("/ping")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	// No X-Trace-Id header when telemetry is disabled
	assert.Empty(t, resp.Header.Get("X-Trace-Id"))
}

func TestCorrelationID_Enabled(t *testing.T) {
	t.Parallel()
	logx.Disable()
	cfg := DefaultConfig()
	cfg.Correlation = &CorrelationConfig{
		Enabled:        true,
		RequestHeader:  "X-Correlation-ID",
		ResponseHeader: "X-Correlation-ID",
	}
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest("/ping")
	req.Header.Set("X-Correlation-ID", "my-custom-id")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "my-custom-id", resp.Header.Get("X-Correlation-ID"))
}

func TestCorrelationID_Generated(t *testing.T) {
	t.Parallel()
	logx.Disable()
	cfg := DefaultConfig()
	cfg.Correlation = &CorrelationConfig{
		Enabled:        true,
		ResponseHeader: "X-Correlation-ID",
	}
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest("/ping")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Correlation-ID"))
}

func TestCorrelationID_Disabled(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest("/ping")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("X-Correlation-ID"))
}

func TestCorrelationID_SkipPaths(t *testing.T) {
	t.Parallel()
	logx.Disable()
	cfg := DefaultConfig()
	cfg.Correlation = &CorrelationConfig{
		Enabled:        true,
		ResponseHeader: "X-Correlation-ID",
		SkipPaths:      []string{"/health"},
	}
	app := New(cfg, TelemetryConfig{}, SecurityConfig{}, nil)

	// Skipped path should not have correlation ID
	req := testRequest("/health")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("X-Correlation-ID"))
}

func TestErrorHandler_5xxAlwaysSanitizes(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/error-5xx", func(_ fiber.Ctx) error {
		return errcode.ErrDBQuery("select", "table", fmt.Errorf("real error detail"))
	})

	req := testRequest("/error-5xx")
	resp, _ := app.app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	json.Unmarshal(body, &errResp)
	assert.Equal(t, 500, errResp.Code)
	assert.Equal(t, errcode.ErrCodeDBQuery, errResp.Error)
	assert.Equal(t, "internal server error", errResp.Message)
}

func TestErrorHandler_NilErrorDoesNotPanic(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/nil-error", func(_ fiber.Ctx) error {
		return nil
	})
	req := testRequest("/nil-error")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCORS_NilConfig(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest("/test")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCORS_WithOrigins(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, &CORSConfig{
		Origins: []string{"https://example.com"},
	})
	app.app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest("/test")
	req.Header.Set("Origin", "https://example.com")
	resp, err := app.app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "https://example.com", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestErrorHandler_Forbidden(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/forbidden", func(_ fiber.Ctx) error {
		return errcode.ErrForbidden("admin")
	})
	req := testRequest("/forbidden")
	resp, _ := app.app.Test(req)
	var errResp ErrorResponse
	json.Unmarshal(mustReadBody(resp), &errResp)
	assert.Equal(t, 403, errResp.Code)
	assert.Equal(t, errcode.ErrCodeForbidden, errResp.Error)
}

func TestErrorHandler_Timeout(t *testing.T) {
	t.Parallel()
	logx.Disable()
	app := New(DefaultConfig(), TelemetryConfig{}, SecurityConfig{}, nil)
	app.app.Get("/timeout", func(_ fiber.Ctx) error {
		return errcode.ErrTimeout("db query")
	})
	req := testRequest("/timeout")
	resp, _ := app.app.Test(req)
	var errResp ErrorResponse
	json.Unmarshal(mustReadBody(resp), &errResp)
	assert.Equal(t, 504, errResp.Code)
	assert.Equal(t, errcode.ErrCodeTimeout, errResp.Error)
}

func mustReadBody(resp *http.Response) []byte {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return body
}
