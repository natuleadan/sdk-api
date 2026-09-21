package errcode

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/samber/oops"
)

func TestErrCodeConstants(t *testing.T) {
	tests := []struct {
		code   string
		expect string
	}{
		{ErrCodeNotFound, "ERR_NOT_FOUND"},
		{ErrCodeValidation, "ERR_VALIDATION"},
		{ErrCodeUnauthorized, "ERR_UNAUTHORIZED"},
		{ErrCodeForbidden, "ERR_FORBIDDEN"},
		{ErrCodeRateLimited, "ERR_RATE_LIMITED"},
		{ErrCodeTimeout, "ERR_TIMEOUT"},
		{ErrCodeDBConnection, "ERR_DB_CONNECTION"},
		{ErrCodeDBQuery, "ERR_DB_QUERY"},
		{ErrCodeNATS, "ERR_NATS"},
		{ErrCodeInternal, "ERR_INTERNAL"},
	}
	for _, tt := range tests {
		if tt.code != tt.expect {
			t.Errorf("expected %q, got %q", tt.expect, tt.code)
		}
	}
}

func testError(t *testing.T, err error, wantCode string, wantStatus int, wantPublic string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if fe, ok := errors.AsType[*fiber.Error](err); ok {
		if fe.Code != wantStatus {
			t.Errorf("fiber error code: expected %d, got %d", wantStatus, fe.Code)
		}
	} else {
		t.Errorf("expected fiber.Error, got %T", err)
	}

	if oo, ok := errors.AsType[oops.OopsError](err); ok {
		if c := oo.Code(); c != wantCode {
			t.Errorf("oops code: expected %q, got %v", wantCode, c)
		}
		if p := oo.Public(); p != wantPublic {
			t.Errorf("oops public: expected %q, got %q", wantPublic, p)
		}
	} else {
		t.Errorf("expected oops.OopsError, got %T", err)
	}
}

func TestErrNotFound(t *testing.T) {
	err := ErrNotFound("product", 42)
	testError(t, err, ErrCodeNotFound, 404, "resource not found")
}

func TestErrDBQuery(t *testing.T) {
	inner := errors.New("connection timeout")
	err := ErrDBQuery("select", "users", inner)
	testError(t, err, ErrCodeDBQuery, 500, "Database operation failed")
	if !errors.Is(err, inner) {
		t.Error("expected inner error to be in chain")
	}
}

func TestErrValidation(t *testing.T) {
	err := ErrValidation("email", "required", "")
	testError(t, err, ErrCodeValidation, 400, "Validation failed")
}

func TestErrUnauthorized(t *testing.T) {
	err := ErrUnauthorized("invalid token")
	testError(t, err, ErrCodeUnauthorized, 401, "invalid token")
}

func TestErrForbidden(t *testing.T) {
	err := ErrForbidden("admin")
	testError(t, err, ErrCodeForbidden, 403, "Access denied")
}

func TestErrRateLimited(t *testing.T) {
	err := ErrRateLimited(30)
	testError(t, err, ErrCodeRateLimited, 429, "Too many requests")
}

func TestErrTimeout(t *testing.T) {
	err := ErrTimeout("db query")
	testError(t, err, ErrCodeTimeout, 504, "Operation timed out")
}

func TestErrDBConnection(t *testing.T) {
	inner := errors.New("dial tcp refused")
	err := ErrDBConnection(inner)
	testError(t, err, ErrCodeDBConnection, 500, "Database connection failed")
	if !errors.Is(err, inner) {
		t.Error("expected inner error to be in chain")
	}
}

func TestErrNATSPublish(t *testing.T) {
	inner := errors.New("no servers available")
	err := ErrNATSPublish("orders.created", inner)
	testError(t, err, ErrCodeNATS, 500, "Message publishing failed")
	if !errors.Is(err, inner) {
		t.Error("expected inner error to be in chain")
	}
}

func TestErrInternal(t *testing.T) {
	inner := errors.New("unexpected")
	err := ErrInternal(inner)
	testError(t, err, ErrCodeInternal, 500, "internal server error")
	if !errors.Is(err, inner) {
		t.Error("expected inner error to be in chain")
	}
}

func TestErrStatus(t *testing.T) {
	err := ErrStatus(415, "unsupported", "content_type", "text/html")
	testError(t, err, ErrCodeInternal, 415, "unsupported")
}

func TestErrCodeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{fiber.StatusNotFound, ErrCodeNotFound},
		{fiber.StatusBadRequest, ErrCodeValidation},
		{fiber.StatusUnauthorized, ErrCodeUnauthorized},
		{fiber.StatusForbidden, ErrCodeForbidden},
		{fiber.StatusTooManyRequests, ErrCodeRateLimited},
		{fiber.StatusGatewayTimeout, ErrCodeTimeout},
		{999, ErrCodeInternal},
	}
	for _, tt := range tests {
		got := errCodeFromStatus(tt.status)
		if got != tt.want {
			t.Errorf("errCodeFromStatus(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestWriteProblem(t *testing.T) {
	app := fiber.New()
	app.Get("/boom", func(c fiber.Ctx) error {
		return WriteProblem(c, 403, ErrCodeForbidden, "csrf token mismatch")
	})
	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 403 {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q, want application/problem+json", ct)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["type"] != "about:blank" || doc["title"] != "Forbidden" {
		t.Errorf("type/title = %v/%v", doc["type"], doc["title"])
	}
	if doc["status"] != float64(403) || doc["code"] != ErrCodeForbidden {
		t.Errorf("status/code = %v/%v", doc["status"], doc["code"])
	}
	if doc["detail"] != "csrf token mismatch" || doc["instance"] != "/boom" {
		t.Errorf("detail/instance = %v/%v", doc["detail"], doc["instance"])
	}
}

func TestWriteProblemWith(t *testing.T) {
	app := fiber.New()
	app.Get("/bad", func(c fiber.Ctx) error {
		return WriteProblemWith(c, 422, ErrCodeValidation, "validation failed",
			map[string]any{"fields": map[string]string{"email": "required"}})
	})
	req := httptest.NewRequest("GET", "/bad", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	fields, ok := doc["fields"].(map[string]any)
	if !ok || fields["email"] != "required" {
		t.Errorf("fields extension = %v", doc["fields"])
	}
	if doc["status"] != float64(422) || doc["code"] != ErrCodeValidation {
		t.Errorf("status/code = %v/%v", doc["status"], doc["code"])
	}
}
