package server

import (
	"github.com/goccy/go-json"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
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
