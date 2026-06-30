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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)

	checks := map[string]string{
		"X-Content-Type-Options":         "nosniff",
		"X-Frame-Options":                "DENY",
		"Referrer-Policy":                "strict-origin-when-cross-origin",
		"Permissions-Policy":             "camera=(), microphone=()",
		"Strict-Transport-Security":      "max-age=31536000",
		"Content-Security-Policy":        "default-src 'self'",
		"Cross-Origin-Opener-Policy":     "same-origin",
		"Cross-Origin-Embedder-Policy":   "require-corp",
		"Cross-Origin-Resource-Policy":   "same-origin",
		"Cache-Control":                  "no-store",
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
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// GET to get token
	req1, _ := http.NewRequest("GET", "/test", nil)
	resp1, _ := app.Test(req1)
	cookie := resp1.Header.Get("Set-Cookie")

	// POST with matching token
	token := extractCSRFToken(cookie)
	req2, _ := http.NewRequest("POST", "/test", nil)
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
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/test", nil)
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
	app.Post("/webhooks/stripe", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("POST", "/webhooks/stripe", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for excluded path, got %d", resp.StatusCode)
	}
}

func extractCSRFToken(setCookie string) string {
	for _, part := range strings.Split(setCookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "csrf_token=") {
			return strings.TrimPrefix(part, "csrf_token=")
		}
		if strings.HasPrefix(part, "csrf_test=") {
			return strings.TrimPrefix(part, "csrf_test=")
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
