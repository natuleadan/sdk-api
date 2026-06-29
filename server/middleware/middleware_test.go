package middleware

import (
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
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func tokenFor(secret string) string {
	t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "123"}).SignedString([]byte(secret))
	return t
}

func TestJWTValid(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c *fiber.Ctx) error {
		claims := c.Locals("claims")
		return c.JSON(fiber.Map{"claims": claims})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
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
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTInvalid(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(JWT(JWTConfig{Secret: "secret123"}))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
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
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor("old-secret"))
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (prev secret), got %d", resp.StatusCode)
	}
}

func TestCORS(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(CORS(DefaultCORSConfig()))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRecovery(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Recovery())
	app.Get("/panic", func(c *fiber.Ctx) error {
		panic("oops")
	})

	req, _ := http.NewRequest("GET", "/panic", nil)
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

func TestMaxConns(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(MaxConns(5))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for range 5 {
		req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.Send(c.Body())
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"hello":"world"}`))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.Send(c.Body())
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader("plain-text"))
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
	app.Get("/events", SSE(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/events", nil)
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBreaker(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Breaker())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBreakerClientError(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Breaker())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.Status(400).SendString("bad")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	// Client errors should be accepted by breaker (not trip it)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTimeout(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Timeout(100 * time.Millisecond))
	app.Get("/fast", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/fast", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTrace(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(Trace(TraceConfig{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"hello":"world"}`))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"hello":"world"}`))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	body := `{"hello":"world"}`
	sig, err := SignBody(priv, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(body))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	body := `{"hello":"world"}`
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(body))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	plaintext := `{"hello":"world"}`
	encrypted, err := AESEncrypt([]byte(plaintext), key)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(string(encrypted)))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader("not-encoded-raw-data"))
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader("some-body"))
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest("GET", "/metrics", nil)
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
	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})

	for range 3 {
		req, _ := http.NewRequest("GET", "/ping", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}

	req, _ := http.NewRequest("GET", "/metrics", nil)
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
	app.Get("/slow", func(c *fiber.Ctx) error {
		time.Sleep(100 * time.Millisecond)
		return c.SendString("too late")
	})

	req, _ := http.NewRequest("GET", "/slow", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for small body, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"a":"0123456789"}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for large body, got %d", resp2.StatusCode)
	}
}
