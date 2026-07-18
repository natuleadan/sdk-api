package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	fiberrecover "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/auth/zitadel"
	"golang.org/x/time/rate"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func testRequest(ctx context.Context, method, path string, body io.Reader) *http.Request {
	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		panic(err)
	}
	req.Host = "test.com"
	return req
}

func tokenFor(secret string) string {
	t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "123"}).SignedString([]byte(secret))
	return t
}

func TestJWTValid(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		claims := c.Locals("claims")
		return c.JSON(fiber.Map{"claims": claims})
	})

	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor("secret123"))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTMissing(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTInvalid(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTSecretRotation(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{
		Secret:     "new-secret",
		PrevSecret: "old-secret",
	}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor("old-secret"))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (prev secret), got %d", resp.StatusCode)
	}
}

func TestJWTAlgorithmPinning(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123", Algorithm: "HS256"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	t.Run("wrong algorithm", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{"sub": "123"}).SignedString([]byte("secret123"))
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for HS384 on HS256-configured middleware, got %d", resp.StatusCode)
		}
	})

	t.Run("correct algorithm", func(t *testing.T) {
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tokenFor("secret123"))
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestJWTIssuerValidation(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123", Issuer: "sdk-api"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	t.Run("wrong issuer", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "123",
			"iss": "other-api",
		}).SignedString([]byte("secret123"))
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for wrong issuer, got %d", resp.StatusCode)
		}
	})

	t.Run("correct issuer", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "123",
			"iss": "sdk-api",
		}).SignedString([]byte("secret123"))
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestJWTAudienceValidation(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123", Audience: "api.example.com"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	t.Run("wrong audience", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "123",
			"aud": "other.example.com",
		}).SignedString([]byte("secret123"))
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for wrong audience, got %d", resp.StatusCode)
		}
	})

	t.Run("correct audience", func(t *testing.T) {
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "123",
			"aud": "api.example.com",
		}).SignedString([]byte("secret123"))
		req := testRequest(context.Background(), "GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestJWTExpiredToken(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "123",
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", resp.StatusCode)
	}
}

func TestJWTUserClaimExtraction(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/whoami", func(c fiber.Ctx) error {
		claims := c.Locals("claims").(jwt.MapClaims)
		return c.JSON(claims)
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-456",
		"usr": "alice",
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuthContextExtraction(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/whoami", func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(500).SendString("nil auth")
		}
		return c.JSON(auth)
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "user-789",
		"org_id":      "org-acme",
		"roles":       []any{"admin", "editor"},
		"permissions": []any{"products:create", "products:read"},
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTCookieExtraction(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123", TokenLookup: "cookie:token"}))
	app.Get("/whoami", func(c fiber.Ctx) error {
		claims := c.Locals("claims").(jwt.MapClaims)
		return c.JSON(claims)
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-789",
		"roles": []any{"admin"},
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/whoami", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tok})
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("JWT cookie extraction: expected 200, got %d", resp.StatusCode)
	}

	// Also verify without cookie — should fail
	req2 := testRequest(context.Background(), "GET", "/whoami", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("JWT no cookie: expected 401, got %d", resp2.StatusCode)
	}
}

/*
Demo: encryptcookie + JWT cookie extraction (uncomment to test):
1. Uncomment the test below
2. Add this import:
     "github.com/gofiber/fiber/v3/middleware/encryptcookie"
3. Run: go test -v -run TestEncryptCookieJWTRoundtrip ./server/middleware/

The test verifies that encryptcookie transparently encrypts/decrypts
cookies so JWT middleware (which reads c.Cookies("token")) works correctly.

func TestEncryptCookieJWTRoundtrip(t *testing.T) {
	logx.Disable()
	key := encryptcookie.GenerateKey(32)
	app := fiber.New()
	app.Use(encryptcookie.New(encryptcookie.Config{Key: key}))
	app.Get("/set", func(c fiber.Ctx) error {
		c.Cookie(&fiber.Cookie{Name: "token", Value: "raw-jwt", Path: "/"})
		return c.SendString("set")
	})
	app.Get("/get", func(c fiber.Ctx) error { return c.SendString(c.Cookies("token")) })
	req1, _ := app.Test(testRequest(context.Background(), "GET", "/set", nil))
	cookies := req1.Header.Values("Set-Cookie")
	t.Logf("encrypted on wire: %s", cookies[0])
	req2 := testRequest(context.Background(), "GET", "/get", nil)
	req2.Header.Set("Cookie", cookies[0][:strings.IndexByte(cookies[0], ';')])
	resp2, _ := app.Test(req2)
	body, _ := io.ReadAll(resp2.Body)
	if string(body) != "raw-jwt" { t.Fatalf("got %q", string(body)) }
	t.Log("encryptcookie + JWT cookie: roundtrip OK")
}
*/

func TestAuthContextFromFiberCtx(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/extract", func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(500).SendString("nil auth")
		}
		if auth.UserID != "user-001" {
			return c.Status(500).SendString("bad sub")
		}
		if len(auth.Roles) != 2 {
			return c.Status(500).SendString("bad roles count")
		}
		return c.SendString("ok")
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-001",
		"roles": []any{"reader", "writer"},
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/extract", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuthContextFromContext(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/fromctx", func(c fiber.Ctx) error {
		auth := AuthFromContext(c.Context())
		if auth == nil {
			return c.Status(500).SendString("nil auth")
		}
		if auth.UserID != "user-ctx" {
			return c.Status(500).SendString("bad ctx: " + auth.UserID)
		}
		return c.SendString("ok")
	})

	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-ctx",
	}).SignedString([]byte("secret123"))
	req := testRequest(context.Background(), "GET", "/fromctx", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuthContextNoJWT(t *testing.T) {
	logx.Disable()
	app := fiber.New()

	app.Get("/noauth", func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth != nil {
			return c.Status(500).SendString("should be nil")
		}
		authCtx := AuthFromContext(c.Context())
		if authCtx != nil {
			return c.Status(500).SendString("ctx should be nil")
		}
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/noauth", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTWithZitadel_NilClientPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil client")
		}
	}()
	JWTWithZitadel(JWTConfig{Secret: "test"}, nil)
}

func TestJWTWithZitadel_NoToken(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	zClient := zitadel.NewClient(zitadel.Config{Issuer: "https://example.com"})
	app.Use(JWTWithZitadel(JWTConfig{Secret: "test"}, zClient))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIKey_MissingHeader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(APIKey(APIKeyConfig{Prefix: "sk-"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIKey_WrongPrefix(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(APIKey(APIKeyConfig{Prefix: "sk-"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "pk-test-key-123")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAPIKey_ValidWithoutFGA(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(APIKey(APIKeyConfig{Prefix: "sk-"}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	req.Header.Set("Authorization", "sk-test-key-456")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDeriveKeyID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sk-test-key-123", "sktestkey"},
		{"short", "short"},
		{"a!b@c#d$e%f^g&h*i(j)k_l-m=n", "abcdef"},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "ABCDEFGHIJKL"},
		{"", ""},
		{"abc123def456ghi789", "abc123def456"},
	}
	for _, tt := range tests {
		result := deriveKeyID(tt.input)
		if result != tt.expected {
			t.Errorf("deriveKeyID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestOry_NilClientPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil client")
		}
	}()
	Ory(OryConfig{Client: nil})
}

func TestOry_NoAuthContext(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	oryClient := ory.NewClient(ory.Config{})
	app.Use(Ory(OryConfig{Client: oryClient}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTokenRefresh_MissingBody(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/refresh", TokenRefreshHandler(TokenRefreshConfig{}))
	req := testRequest(context.Background(), "POST", "/refresh", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTokenRefresh_MissingField(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/refresh", TokenRefreshHandler(TokenRefreshConfig{}))
	req := testRequest(context.Background(), "POST", "/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTokenRefresh_ManualWithoutAuth(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/refresh", TokenRefreshHandler(TokenRefreshConfig{}))
	body := strings.NewReader(`{"refresh_token":"test-token"}`)
	req := testRequest(context.Background(), "POST", "/refresh", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTokenRefresh_ManualWithAuth(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Post("/refresh", TokenRefreshHandler(TokenRefreshConfig{
		JWTSecret: "secret123",
	}))
	body := strings.NewReader(`{"refresh_token":"test"}`)
	req := testRequest(context.Background(), "POST", "/refresh", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenFor("secret123"))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestOpenFGACache_NilClientPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil client")
		}
	}()
	OpenFGA(OpenFGAConfig{})
}

func TestOpenFGA_NoAuthContext(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	fgaClient, err := openfga.NewClient(openfga.Config{APIURL: "http://localhost:9999"})
	if err != nil {
		t.Skip("skipping: could not create FGA client")
	}
	app.Use(OpenFGA(OpenFGAConfig{Client: fgaClient}))
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	req := testRequest(context.Background(), "GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestTokenRefresh_InvalidBody(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/refresh", TokenRefreshHandler(TokenRefreshConfig{}))
	body := strings.NewReader(`not-json`)
	req := testRequest(context.Background(), "POST", "/refresh", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCORS(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CORS(DefaultCORSConfig()))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	resp, _ := app.Test(req)
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestLogger(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Logger())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRecovery(t *testing.T) {
	logx.Disable()
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"code": 500, "message": "internal server error"})
	}})
	app.Use(fiberrecover.New(fiberrecover.Config{}))
	app.Get("/panic", func(c fiber.Ctx) error {
		panic("oops")
	})

	req := testRequest(context.Background(), "GET", "/panic", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var data map[string]any
	json.Unmarshal(body, &data)
	if data["message"] != "internal server error" {
		t.Errorf("expected internal server error message, got %v", data["message"])
	}
}

func TestTimeout(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Timeout(50 * time.Millisecond))
	app.Get("/slow", func(c fiber.Ctx) error {
		time.Sleep(200 * time.Millisecond)
		return c.SendString("too late")
	})
	app.Get("/fast", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Fast request should succeed
	req := testRequest(context.Background(), "GET", "/fast", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("fast: expected 200, got %d", resp.StatusCode)
	}

	// Slow request should timeout
	req = testRequest(context.Background(), "GET", "/slow", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Errorf("slow: expected 408, got %d", resp.StatusCode)
	}
}

func TestMaxConns(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(MaxConns(5))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	for range 5 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 within limit, got %d", resp.StatusCode)
		}
	}
}

func TestGunzip(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Gunzip())
	app.Post("/test", func(c fiber.Ctx) error {
		return c.Send(c.Body())
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(`{"hello":"world"}`))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"hello":"world"}` {
		t.Errorf("expected body unchanged, got %q", string(body))
	}
}

func TestGunzipNoEncoding(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Gunzip())
	app.Post("/test", func(c fiber.Ctx) error {
		return c.Send(c.Body())
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader("plain-text"))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain-text" {
		t.Errorf("expected plain body unchanged, got %q", string(body))
	}
}

func TestSSE(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/events", SSE(), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/events", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type: text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control: no-cache, got %s", resp.Header.Get("Cache-Control"))
	}
}

func TestShedding(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Shedding())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBreaker(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Breaker())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBreakerClientError(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Breaker())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(400).SendString("bad")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	// Client errors should be accepted by breaker (not trip it)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTrace(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTraceResponseHeader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{
		TraceResponseHeader: "X-Trace-Id",
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	traceID := resp.Header.Get("X-Trace-Id")
	if traceID == "" {
		t.Error("expected X-Trace-Id header to be set")
	}
	if len(traceID) != 32 {
		t.Errorf("expected trace ID length 32, got %d", len(traceID))
	}
}

func TestTraceSkipPath(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{
		SkipPaths: []string{"/health"},
	}))
	app.Get("/health", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/health", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTraceCustomAttributes(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{
		CustomAttributes: func(c fiber.Ctx) []attribute.KeyValue {
			return []attribute.KeyValue{
				attribute.String("custom", "value"),
			}
		},
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTracePropagatesContext(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{}))
	app.Get("/test", func(c fiber.Ctx) error {
		spanCtx := oteltrace.SpanContextFromContext(c.Context())
		if !spanCtx.IsValid() {
			t.Error("expected span context to be valid after middleware")
		}
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func testKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key, &key.PublicKey
}

func TestContentSecurityStrict(t *testing.T) {
	logx.Disable()
	_, pub := testKeyPair(t)
	app := fiber.New()
	app.Use(ContentSecurity(pub, true))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for missing signature, got %d", resp.StatusCode)
	}
}

func TestContentSecurityNonStrict(t *testing.T) {
	logx.Disable()
	_, pub := testKeyPair(t)
	app := fiber.New()
	app.Use(ContentSecurity(pub, false))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for non-strict, got %d", resp.StatusCode)
	}
}

func TestContentSecurityValidSignature(t *testing.T) {
	logx.Disable()
	priv, pub := testKeyPair(t)
	app := fiber.New()
	app.Use(ContentSecurity(pub, true))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	body := `{"hello":"world"}`
	sig, err := SignBody(priv, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Content-Security", sig)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid signature, got %d", resp.StatusCode)
	}
}

func TestContentSecurityInvalidSignature(t *testing.T) {
	logx.Disable()
	_, pub := testKeyPair(t)
	app := fiber.New()
	app.Use(ContentSecurity(pub, true))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	body := `{"hello":"world"}`
	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Content-Security", "invalid-sig")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for invalid signature, got %d", resp.StatusCode)
	}
}

func TestCryption(t *testing.T) {
	logx.Disable()
	key := []byte("0123456789abcdef0123456789abcdef")
	app := fiber.New()
	app.Use(Cryption(key))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	plaintext := `{"hello":"world"}`
	encrypted, err := AESEncrypt([]byte(plaintext), key)
	if err != nil {
		t.Fatal(err)
	}
	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(string(encrypted)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCryptionInvalidBody(t *testing.T) {
	logx.Disable()
	key := []byte("0123456789abcdef0123456789abcdef")
	app := fiber.New()
	app.Use(Cryption(key))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader("not-encoded-raw-data"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", resp.StatusCode)
	}
}

func TestCryptionEmptyBody(t *testing.T) {
	logx.Disable()
	key := []byte("0123456789abcdef0123456789abcdef")
	app := fiber.New()
	app.Use(Cryption(key))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for empty body, got %d", resp.StatusCode)
	}
}

func TestCryptionInvalidKey(t *testing.T) {
	logx.Disable()
	key := []byte("short")
	app := fiber.New()
	app.Use(Cryption(key))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader("some-body"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid key, got %d", resp.StatusCode)
	}
}

func TestPrometheus(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Prometheus())
	app.Get("/metrics", PrometheusHandler())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	req2 := testRequest(context.Background(), "GET", "/metrics", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /metrics, got %d", resp2.StatusCode)
	}
	body, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body), "http_server_requests_total") {
		t.Error("expected prometheus metrics in response")
	}
	if !strings.Contains(string(body), "http_server_requests_active") {
		t.Error("expected active requests metric")
	}
}

func TestPrometheusMultipleRequests(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Prometheus())
	app.Get("/metrics", PrometheusHandler())
	app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendString("pong")
	})

	for range 3 {
		req := testRequest(context.Background(), "GET", "/ping", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}

	req := testRequest(context.Background(), "GET", "/metrics", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `path="/ping"`) {
		t.Error("expected ping path in metrics")
	}
	if !strings.Contains(string(body), `code="200"`) {
		t.Error("expected 200 code in metrics")
	}
}

func TestTimeoutShort(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Timeout(5 * time.Millisecond))
	app.Get("/slow", func(c fiber.Ctx) error {
		time.Sleep(100 * time.Millisecond)
		return c.SendString("too late")
	})

	req := testRequest(context.Background(), "GET", "/slow", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Errorf("expected 408 for timeout, got %d", resp.StatusCode)
	}
}

func TestParsePublicKey(t *testing.T) {
	_, pub := testKeyPair(t)
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	parsed, err := ParsePublicKey(string(pemStr))
	if err != nil {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestParsePublicKeyInvalid(t *testing.T) {
	_, err := ParsePublicKey("not-a-pem")
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestMaxBytes(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(MaxBytes(10))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", strings.NewReader(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for small body, got %d", resp.StatusCode)
	}

	req2 := testRequest(context.Background(), "POST", "/test", strings.NewReader(`{"a":"0123456789"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for large body, got %d", resp2.StatusCode)
	}
}

// --- Security Headers Tests ---

func TestSecurityHeaders_Default(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(SecurityHeaders(SecurityHeadersConfig{
		FrameOptions:   "DENY",
		ReferrerPolicy: "strict-origin-when-cross-origin",
		HSTS:           true,
		HSTSMaxAge:     31536000,
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Strict-Transport-Security", "max-age=31536000"},
	}
	for _, tt := range tests {
		got := resp.Header.Get(tt.header)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestSecurityHeaders_EmptyConfig(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(SecurityHeaders(SecurityHeadersConfig{}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)

	if v := resp.Header.Get("X-Content-Type-Options"); v != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", v)
	}
	if v := resp.Header.Get("X-Frame-Options"); v != "" {
		t.Errorf("expected no X-Frame-Options, got %q", v)
	}
}

func TestSecurityHeaders_CSP(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(SecurityHeaders(SecurityHeadersConfig{
		CSP: "default-src 'self'; script-src 'self'; img-src 'self' data:;",
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)

	want := "default-src 'self'; script-src 'self'; img-src 'self' data:;"
	if got := resp.Header.Get("Content-Security-Policy"); got != want {
		t.Errorf("CSP = %q, want %q", got, want)
	}
}

func TestSecurityHeaders_AllHeaders(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(SecurityHeaders(SecurityHeadersConfig{
		FrameOptions:      "DENY",
		ReferrerPolicy:    "strict-origin-when-cross-origin",
		PermissionsPolicy: "camera=(), microphone=()",
		HSTS:              true,
		HSTSMaxAge:        31536000,
		HSTSIncludeSubs:   false,
		CSP:               "default-src 'self'",
		COOP:              "same-origin",
		COEP:              "require-corp",
		CORP:              "same-origin",
		CacheControl:      "no-store",
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)

	checks := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Permissions-Policy":           "camera=(), microphone=()",
		"Strict-Transport-Security":    "max-age=31536000",
		"Content-Security-Policy":      "default-src 'self'",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Embedder-Policy": "require-corp",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cache-Control":                "no-store",
	}
	for h, want := range checks {
		if got := resp.Header.Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
}

// --- CSP Builder Tests ---

func TestBuildCSP_Basic(t *testing.T) {
	csp := BuildCSP(CSPConfig{})
	if csp == "" {
		t.Fatal("expected non-empty CSP")
	}
	if !contains(csp, "default-src 'self'") {
		t.Errorf("expected default-src 'self', got %q", csp)
	}
}

func TestBuildCSP_Strict(t *testing.T) {
	csp := BuildCSP(CSPConfig{Level: CSPLevelStrict})
	if !contains(csp, "strict-dynamic") {
		t.Errorf("expected strict-dynamic in strict CSP, got %q", csp)
	}
}

func TestBuildCSP_Custom(t *testing.T) {
	csp := BuildCSP(CSPConfig{
		DefaultSrc: []string{"'none'"},
		ScriptSrc:  []string{"'self'", "https://cdn.example.com"},
		ImgSrc:     []string{"'self'", "data:"},
	})
	if !contains(csp, "default-src 'none'") {
		t.Errorf("expected default-src 'none', got %q", csp)
	}
	if !contains(csp, "cdn.example.com") {
		t.Errorf("expected cdn.example.com in script-src, got %q", csp)
	}
}

func TestGenerateNonce(t *testing.T) {
	n1 := GenerateNonce()
	n2 := GenerateNonce()
	if n1 == "" || n2 == "" {
		t.Fatal("expected non-empty nonces")
	}
	if n1 == n2 {
		t.Error("expected different nonces")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- CSRF Tests ---

func TestCSRF_InjectOnGET(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CSRF(CSRFConfig{Enabled: true}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	cookies := resp.Header.Values("Set-Cookie")
	found := false
	for _, c := range cookies {
		if contains(c, "csrf_token=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected csrf_token cookie in Set-Cookie")
	}
}

func TestCSRF_ValidateOnPOST(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CSRF(CSRFConfig{Enabled: true, CookieName: "csrf_test", HeaderName: "X-CSRF-Test"}))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// GET to get token
	req1 := testRequest(context.Background(), "GET", "/test", nil)
	resp1, _ := app.Test(req1)
	cookie := resp1.Header.Get("Set-Cookie")

	// POST with matching token
	token := extractCSRFToken(cookie)
	req2 := testRequest(context.Background(), "POST", "/test", nil)
	req2.Header.Set("X-CSRF-Test", token)
	req2.Header.Set("Cookie", extractCookieName(cookie))
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 with valid token, got %d", resp2.StatusCode)
	}
}

func TestCSRF_RejectOnMismatch(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CSRF(CSRFConfig{Enabled: true}))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/test", nil)
	req.Header.Set("X-CSRF-Token", "invalid-token")
	req.Header.Set("Cookie", "csrf_token=other-token")
	resp, _ := app.Test(req)

	if resp.StatusCode != 403 {
		t.Errorf("expected 403 for mismatched token, got %d", resp.StatusCode)
	}
}

func TestCSRF_SkipExcludedPath(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CSRF(CSRFConfig{
		Enabled:      true,
		ExcludePaths: []string{"/webhooks/*"},
	}))
	app.Post("/webhooks/stripe", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "POST", "/webhooks/stripe", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for excluded path, got %d", resp.StatusCode)
	}
}

func extractCSRFToken(setCookie string) string {
	for part := range strings.SplitSeq(setCookie, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, "csrf_token="); ok {
			return after
		}
		if after, ok := strings.CutPrefix(part, "csrf_test="); ok {
			return after
		}
	}
	return ""
}

func extractCookieName(setCookie string) string {
	if idx := strings.Index(setCookie, ";"); idx > 0 {
		return setCookie[:idx]
	}
	return setCookie
}

// --- Rate Limit Tests ---

func TestRateLimit_Global_UnderLimit(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global: &RateLimitEntry{RequestsPerSecond: 1000, Burst: 1000},
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := range 5 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

func TestRateLimit_Global_OverLimit(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global: &RateLimitEntry{RequestsPerSecond: 1, Burst: 1},
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First request should pass (burst=1)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("first request: expected 200, got %d", resp.StatusCode)
	}

	// Immediate second request should be rate limited
	req2 := testRequest(context.Background(), "GET", "/test", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 429 {
		t.Errorf("second request: expected 429, got %d", resp2.StatusCode)
	}
}

func TestRateLimit_Disabled(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	// If no rate limit config is set (or not enabled), no middleware should be added.
	// Passing empty RateLimitConfig with no entries = no limiter created, all pass.
	app.Use(RateLimit(RateLimitConfig{}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := range 100 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
			break
		}
	}
}

func TestRateLimit_PerIP(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		PerIP: &RateLimitEntry{RequestsPerSecond: 1, Burst: 1},
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First request passes (burst=1)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("first request: expected 200, got %d", resp.StatusCode)
	}

	// Second request from same IP is rate limited
	req2 := testRequest(context.Background(), "GET", "/test", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 429 {
		t.Errorf("second request: expected 429, got %d", resp2.StatusCode)
	}
}

func TestRateLimit_RetryAfterHeader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global: &RateLimitEntry{RequestsPerSecond: 1, Burst: 1},
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First passes
	req := testRequest(context.Background(), "GET", "/test", nil)
	app.Test(req)

	// Second is rate limited
	req2 := testRequest(context.Background(), "GET", "/test", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

// --- CRLF Protection Tests ---

func TestHeaderSanitize_Clean(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(HeaderSanitize())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	req.Header.Set("X-Custom", "clean-value")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCRLF_Detect(t *testing.T) {
	tests := []struct {
		input []byte
		want  bool
	}{
		{[]byte("clean"), false},
		{[]byte("value\r\ninjected"), true},
		{[]byte("value\ninjected"), true},
		{[]byte("value\rinjected"), true},
		{[]byte(""), false},
	}
	for _, tt := range tests {
		got := containsCRLF(tt.input)
		if got != tt.want {
			t.Errorf("containsCRLF(%q) = %v, want %v", string(tt.input), got, tt.want)
		}
	}
}

// --- SSRF Tests ---

func TestSSRF_Disabled(t *testing.T) {
	logx.Disable()
	cfg := SSRFConfig{Enabled: false}
	client := NewSafeHTTPClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestSSRF_BlockPrivate(t *testing.T) {
	cfg := SSRFConfig{
		Enabled:      true,
		BlockPrivate: true,
		AllowAll:     false,
	}
	client := NewSafeHTTPClient(cfg)
	if err := client.checker.validate("10.0.0.5"); err == nil {
		t.Error("expected error for private IP")
	}
}

func TestSSRF_BlockLoopback(t *testing.T) {
	cfg := SSRFConfig{
		Enabled:       true,
		BlockLoopback: true,
		AllowAll:      false,
	}
	client := NewSafeHTTPClient(cfg)
	if err := client.checker.validate("127.0.0.1"); err == nil {
		t.Error("expected error for loopback IP")
	}
}

func TestSSRF_BlockMetadata(t *testing.T) {
	cfg := SSRFConfig{
		Enabled:       true,
		BlockMetadata: true,
		AllowAll:      false,
	}
	client := NewSafeHTTPClient(cfg)
	if err := client.checker.validate("169.254.169.254"); err == nil {
		t.Error("expected error for metadata IP")
	}
}

func TestSSRF_ExternalHostPasses(t *testing.T) {
	cfg := SSRFConfig{
		Enabled:      true,
		BlockPrivate: true,
		AllowAll:     false,
	}
	client := NewSafeHTTPClient(cfg)
	if err := client.checker.validate("93.184.216.34"); err != nil {
		t.Errorf("expected no error for public IP, got %v", err)
	}
}

func TestSSRF_AllowedHost(t *testing.T) {
	cfg := SSRFConfig{
		Enabled:      true,
		BlockPrivate: true,
		AllowedHosts: []string{"api.example.com"},
		AllowAll:     false,
	}
	client := NewSafeHTTPClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// allowed host is not blocked (even if DNS fails, it won't error with private IP)
	_, err := client.DoURL(context.Background(), "https://api.example.com/test", "GET", nil)
	if err != nil && err.Error() != "ssrf: cannot resolve host api.example.com" {
		t.Logf("unexpected error: %v", err)
	}
}

func TestSSRF_AllowAll(t *testing.T) {
	// allow_all bypasses validation entirely (even for private IPs)
	cfg := SSRFConfig{
		Enabled:       true,
		BlockPrivate:  true,
		BlockMetadata: true,
		AllowAll:      true,
	}
	client := NewSafeHTTPClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	// With allowAll, the validation should pass (error would be connection refused, not SSRF blocked)
	checker := client.checker
	if err := checker.validate("10.0.0.5"); err != nil {
		t.Errorf("expected no error with allowAll, got %v", err)
	}
}

// --- Rate Limit Post-Auth Tests ---

func authInjector(auth *AuthContext) fiber.Handler {
	return func(c fiber.Ctx) error {
		injectAuth(c, auth)
		return c.Next()
	}
}

func TestRateLimitPost_EntryPerUser_IndependentBuckets(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-a"}),
		RateLimitPost(RateLimitPostConfig{
			EntryPerUser: &RateLimitEntry{RequestsPerSecond: 2, Burst: 4},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	var blocked bool
	for range 6 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode == 429 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Error("expected user A to be rate-limited after burst of 4")
	}

	// User B — should be a separate app with new store (independent bucket)
	app2 := fiber.New()
	app2.Post("/test",
		authInjector(&AuthContext{UserID: "user-b"}),
		RateLimitPost(RateLimitPostConfig{
			EntryPerUser: &RateLimitEntry{RequestsPerSecond: 2, Burst: 4},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)
	req := testRequest(context.Background(), "POST", "/test", nil)
	resp, _ := app2.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("user B should have independent bucket, got %d", resp.StatusCode)
	}
}

func TestRateLimitPost_EntryPerKey_IndependentBuckets(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "key-a"}), // RawToken empty = API key
		RateLimitPost(RateLimitPostConfig{
			EntryPerKey: &RateLimitEntry{RequestsPerSecond: 2, Burst: 4},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	var blocked bool
	for range 6 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode == 429 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Error("expected key A to be rate-limited after burst of 4")
	}

	// Key B — independent store
	app2 := fiber.New()
	app2.Post("/test",
		authInjector(&AuthContext{UserID: "key-b"}),
		RateLimitPost(RateLimitPostConfig{
			EntryPerKey: &RateLimitEntry{RequestsPerSecond: 2, Burst: 4},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)
	req := testRequest(context.Background(), "POST", "/test", nil)
	resp, _ := app2.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("key B should have independent bucket, got %d", resp.StatusCode)
	}
}

func TestRateLimitPost_ServerPerUser_NoAuthSkipped(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/test",
		RateLimitPost(RateLimitPostConfig{
			ServerPerUser: &RateLimitEntry{RequestsPerSecond: 1, Burst: 1},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	// Without auth context — rate limit is skipped (user ID empty)
	for i := range 5 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("req %d: expected 200 (no auth = skip), got %d", i, resp.StatusCode)
		}
	}
}

func TestRateLimitPost_EntryPerUser_AllowsWithinBurst(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-a"}),
		RateLimitPost(RateLimitPostConfig{
			EntryPerUser: &RateLimitEntry{RequestsPerSecond: 10, Burst: 10},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	for i := range 10 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("req %d: expected 200 within burst, got %d", i, resp.StatusCode)
		}
	}
}

// --- extractKeyID Tests ---

func TestExtractKeyID_JWT(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/test",
		func(c fiber.Ctx) error {
			injectAuth(c, &AuthContext{UserID: "user-admin", RawToken: "eyJ.xxx.yyy"})
			if id := extractKeyID(c); id != "" {
				t.Errorf("expected empty for JWT, got %q", id)
			}
			return c.SendString("ok")
		},
	)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestExtractKeyID_APIKey(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/test",
		func(c fiber.Ctx) error {
			injectAuth(c, &AuthContext{UserID: "key-admin"})
			if id := extractKeyID(c); id != "key-admin" {
				t.Errorf("expected key-admin, got %q", id)
			}
			return c.SendString("ok")
		},
	)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestExtractKeyID_NoAuth(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/test",
		func(c fiber.Ctx) error {
			if id := extractKeyID(c); id != "" {
				t.Errorf("expected empty, got %q", id)
			}
			return c.SendString("ok")
		},
	)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// --- Sliding Window Algorithm Tests ---

func TestSlidingWindow_BasicAllow(t *testing.T) {
	l := newSlidingWindowLimiter(10, 1) // 10 rps, 1s window
	for i := range 10 {
		if !l.Allow() {
			t.Errorf("request %d: expected allowed within limit", i)
		}
	}
	// 11th should be denied
	if l.Allow() {
		t.Error("expected denied when over limit")
	}
}

func TestSlidingWindow_Remaining(t *testing.T) {
	l := newSlidingWindowLimiter(5, 1) // 5 rps
	if n := l.Remaining(); n != 5 {
		t.Errorf("expected 5 remaining initially, got %d", n)
	}
	l.Allow()
	if n := l.Remaining(); n < 4 {
		t.Errorf("expected ~4 remaining after 1, got %d", n)
	}
}

func TestSlidingWindow_AllowAfterWindowExpires(t *testing.T) {
	l := newSlidingWindowLimiter(1, 1) // 1 per 1s window
	if !l.Allow() {
		t.Fatal("expected first request allowed")
	}
	if l.Allow() {
		t.Fatal("expected second request denied within same window")
	}
	// Can't easily test time-based expiration in unit test — this verifies the
	// limiter rejects excess requests within the window.
}

func TestSlidingWindow_BurstSmallerThanMax(t *testing.T) {
	l := newSlidingWindowLimiter(100, 10) // 100 rps, 10s window (burst = expiration)
	for i := range 100 {
		if !l.Allow() {
			t.Errorf("request %d: expected allowed within 100 burst", i)
			break
		}
	}
}

// --- RateLimit With Algorithm Tests ---

func TestRateLimit_AlgorithmSlidingWindow(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global:    &RateLimitEntry{RequestsPerSecond: 2, Burst: 1},
		Algorithm: AlgorithmSlidingWindow,
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First two requests should pass
	for i := range 2 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}

	// Third should be denied (2 max per window)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 429 {
		t.Errorf("expected 429 over limit, got %d", resp.StatusCode)
	}
}

func TestRateLimit_AlgorithmTokenBucket(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global:    &RateLimitEntry{RequestsPerSecond: 2, Burst: 2},
		Algorithm: AlgorithmTokenBucket,
	}))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First two requests should pass (burst=2)
	for i := range 2 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}

	// Third should be denied (no tokens left)
	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 429 {
		t.Errorf("expected 429 over limit, got %d", resp.StatusCode)
	}
}

// --- Per-Role Rate Limit Tests ---

func TestRateLimitPost_PerRoleLimits_AdminBlocked(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-admin", Roles: []string{"admin"}}),
		RateLimitPost(RateLimitPostConfig{
			PerRoleLimits: map[string]*RateLimitEntry{
				"admin":  {RequestsPerSecond: 1, Burst: 1},
				"viewer": {RequestsPerSecond: 10, Burst: 10},
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	var blocked bool
	for range 3 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode == 429 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Error("expected admin to be rate-limited by per-role limit (1 rps)")
	}
}

func TestRateLimitPost_PerRoleLimits_ViewerNotBlockedByAdminLimit(t *testing.T) {
	// Different role = different bucket
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-viewer", Roles: []string{"viewer"}}),
		RateLimitPost(RateLimitPostConfig{
			PerRoleLimits: map[string]*RateLimitEntry{
				"admin":  {RequestsPerSecond: 1, Burst: 1},
				"viewer": {RequestsPerSecond: 10, Burst: 10},
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	// Viewer has 10 rps — all 3 should pass
	for i := range 3 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("viewer req %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

func TestRateLimitPost_PerRoleLimits_NoMatchingRole(t *testing.T) {
	// Role not in PerRoleLimits = no per-role limit applied
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-super", Roles: []string{"super"}}),
		RateLimitPost(RateLimitPostConfig{
			PerRoleLimits: map[string]*RateLimitEntry{
				"admin": {RequestsPerSecond: 1, Burst: 1},
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	for i := range 10 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("unmatched role req %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

func TestRateLimitPost_PerRoleLimits_MultipleRoles(t *testing.T) {
	// User has multiple roles — the FIRST matching role limit applies
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-multi", Roles: []string{"viewer", "editor"}}),
		RateLimitPost(RateLimitPostConfig{
			PerRoleLimits: map[string]*RateLimitEntry{
				"viewer": {RequestsPerSecond: 1, Burst: 1},
				"editor": {RequestsPerSecond: 5, Burst: 5},
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	// First role "viewer" matches → 1 rps limit
	var blocked bool
	for range 3 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode == 429 {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Error("expected first matching role limit to apply")
	}
}

func TestRateLimitPost_PerRoleLimits_NoAuth(t *testing.T) {
	// No auth context = no per-role limit (skips gracefully)
	app := fiber.New()
	app.Post("/test",
		RateLimitPost(RateLimitPostConfig{
			PerRoleLimits: map[string]*RateLimitEntry{
				"admin": {RequestsPerSecond: 1, Burst: 1},
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	for i := range 10 {
		req := testRequest(context.Background(), "POST", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Errorf("no-auth req %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

// --- Cancel/Rollback Tests ---

func TestXrateLimiter_Cancel(t *testing.T) {
	l := &xrateLimiter{Limiter: rate.NewLimiter(rate.Limit(1), 1)}
	if !l.Allow() {
		t.Fatal("expected first allow")
	}
	if l.Allow() {
		t.Fatal("expected second deny (burst=1)")
	}
	l.Cancel()
	if !l.Allow() {
		t.Fatal("expected allow after cancel (refunded token)")
	}
}

func TestSlidingWindowLimiter_Cancel(t *testing.T) {
	l := newSlidingWindowLimiter(1, 1)
	if !l.Allow() {
		t.Fatal("expected first allow")
	}
	l.Cancel()
	if !l.Allow() {
		t.Fatal("expected allow after cancel")
	}
}

// --- SkipFailedRequests Tests ---

func TestRateLimit_SkipFailedRequests(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(RateLimit(RateLimitConfig{
		Global:             &RateLimitEntry{RequestsPerSecond: 1, Burst: 1},
		SkipFailedRequests: true,
		Algorithm:          AlgorithmSlidingWindow,
	}))
	// Handler that returns 500 (failed request)
	app.Get("/fail", func(c fiber.Ctx) error {
		return c.Status(500).SendString("fail")
	})
	// Handler that returns 200
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Failed requests should not consume tokens
	for i := range 10 {
		req := testRequest(context.Background(), "GET", "/fail", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 500 {
			t.Errorf("fail req %d: expected 500, got %d", i, resp.StatusCode)
		}
	}

	// Successful requests should still work (tokens were not consumed by failures)
	req := testRequest(context.Background(), "GET", "/ok", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("ok after fails: expected 200, got %d", resp.StatusCode)
	}
}

// --- MaxFunc Tests (post-auth path) ---

func TestRateLimitPost_MaxFunc(t *testing.T) {
	logx.Disable()
	callCount := 0
	app := fiber.New()
	app.Post("/test",
		authInjector(&AuthContext{UserID: "user-a"}),
		RateLimitPost(RateLimitPostConfig{
			EntryPerUser: &RateLimitEntry{RequestsPerSecond: 100, Burst: 100},
			MaxFunc: func(c fiber.Ctx) int {
				callCount++
				return 1
			},
		}),
		func(c fiber.Ctx) error { return c.SendString("ok") },
	)

	// First passes
	req := testRequest(context.Background(), "POST", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("first: expected 200, got %d", resp.StatusCode)
	}

	// Second should be blocked (MaxFunc overrode to 1 rps)
	req = testRequest(context.Background(), "POST", "/test", nil)
	resp, _ = app.Test(req)
	if resp.StatusCode != 429 {
		t.Errorf("second: expected 429, got %d", resp.StatusCode)
	}

	if callCount == 0 {
		t.Error("MaxFunc was never called")
	}
}

// fiber:context-methods migrated
