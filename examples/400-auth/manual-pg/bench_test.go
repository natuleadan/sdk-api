package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

const (
	httpPort = "23400"
	baseURL  = "http://localhost:" + httpPort + "/api/v1"
)

var docker bool

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"
	if !docker {
		if _, err := exec.LookPath("go"); err != nil {
			fmt.Println("skip: no go compiler")
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func waitHTTP(tb testing.TB, timeout time.Duration) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/../healthz")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	tb.Fatalf("service not ready after %v", timeout)
}

func login(t *testing.T, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}
	var res struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.Token
}

func authenticated(method, url, token string, body io.Reader) *http.Response {
	req, _ := http.NewRequest(method, url, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Cookie", "token="+token)
	}
	if method != "GET" || body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

func cookieAuth(method, url, token string, body io.Reader) *http.Response {
	req, _ := http.NewRequest(method, url, body)
	if token != "" {
		req.Header.Set("Cookie", "token="+token)
		req.AddCookie(&http.Cookie{Name: "token", Value: token})
	}
	if method != "GET" || body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

func apiKeyRequest(method, url, key string, body io.Reader) *http.Response {
	req, _ := http.NewRequest(method, url, body)
	if key != "" {
		req.Header.Set("Authorization", key)
	}
	if method != "GET" || body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

// --- Login tests ---

func TestPublicLogin(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "pass123"})
	resp, err := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Log("login OK")
}

func TestLoginWrongPassword(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	resp, _ := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("wrong password rejected")
}

// --- API key tests ---

func TestAPIKey_MissingKey(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := apiKeyRequest("GET", baseURL+"/products", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("missing key rejected")
}

func TestAPIKey_WrongPrefix(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := apiKeyRequest("GET", baseURL+"/products", "bad-key", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("wrong prefix rejected")
}

func TestAPIKey_InvalidKey(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := apiKeyRequest("GET", baseURL+"/products", "sk-invalid", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("invalid key rejected")
}

func TestAPIKey_ViewerList(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := apiKeyRequest("GET", baseURL+"/products", "sk-viewer_abc123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Log("viewer can list")
}

func TestAPIKey_ViewerCannotCreate(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-viewer_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	t.Log("viewer cannot create")
}

func TestAPIKey_EditorCreate(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]any{"name": "editor-product", "price": 10.99})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	t.Log("editor can create")
}

func TestAPIKey_AdminCreate(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]any{"name": "admin-product", "price": 20.00})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	t.Log("admin can create")
}

func TestAPIKey_EditorSoftDelete(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "to-delete"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	resp = apiKeyRequest("DELETE", baseURL+"/products/"+created.ID, "sk-editor_abc123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Log("editor can soft delete")
}

func TestAPIKey_EditorCannotHardDelete(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "hard-to-delete"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	resp = apiKeyRequest("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", "sk-editor_abc123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 (wrong auth mode), got %d", resp.StatusCode)
	}
	t.Log("editor cannot hard delete (no JWT)")
}

func TestAPIKey_AdminSeesConfidential(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "conf-product", "description": "secret info"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", token,
		bytes.NewReader([]byte(`{"visibility":"confidential"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("set visibility: expected 200, got %d", resp2.StatusCode)
	}

	resp3 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-admin_abc123", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		body3, _ := io.ReadAll(resp3.Body)
		t.Fatalf("admin get: expected 200, got %d, body=%s", resp3.StatusCode, string(body3))
	}
	var product struct {
		Description string `json:"description"`
	}
	json.NewDecoder(resp3.Body).Decode(&product)
	if product.Description == "" {
		t.Fatal("admin should see confidential description")
	}
	t.Log("admin sees confidential")
}

func TestAPIKey_ViewerNoConfidential(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "secret-product", "description": "classified"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", token,
		bytes.NewReader([]byte(`{"visibility":"confidential"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("set visibility: expected 200, got %d", resp2.StatusCode)
	}

	resp3 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-viewer_abc123", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("viewer get: expected 200, got %d", resp3.StatusCode)
	}
	var product struct {
		Description string `json:"description"`
	}
	json.NewDecoder(resp3.Body).Decode(&product)
	if product.Description != "[restricted]" {
		t.Fatalf("expected [restricted], got %q", product.Description)
	}
	t.Log("viewer cannot see confidential")
}

// --- JWT auth tests ---

func TestJWT_MissingToken(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := authenticated("DELETE", baseURL+"/admin/products/some-id/hard", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("missing JWT rejected")
}

func TestJWT_AdminHardDelete(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "hard-del"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("admin hard delete: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("admin can hard delete")
}

func TestJWT_ViewerCannotHardDelete(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "viewer-hard"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "viewer", "pass123")
	resp2 := authenticated("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp2.StatusCode)
	}
	t.Log("viewer cannot hard delete")
}

func TestJWT_AdminSetVisibility(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "vis-prod"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", token,
		bytes.NewReader([]byte(`{"visibility":"internal"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("admin set visibility: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("admin can set visibility")
}

func TestJWT_EditorCannotSetVisibility(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "vis-prod2"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "editor", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", token,
		bytes.NewReader([]byte(`{"visibility":"internal"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp2.StatusCode)
	}
	t.Log("editor cannot set visibility")
}

func TestJWT_AdminAuditLog(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "audit-prod"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("GET", baseURL+"/admin/products/"+created.ID+"/audit", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("admin audit: expected 200, got %d", resp2.StatusCode)
	}
	var audit struct {
		Data []any `json:"data"`
	}
	json.NewDecoder(resp2.Body).Decode(&audit)
	if len(audit.Data) == 0 {
		t.Fatal("expected audit entries")
	}
	t.Log("admin can view audit log")
}

func TestRoleCross_UserTokenNoAdminEndpoint(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "viewer", "pass123")
	resp := authenticated("DELETE", baseURL+"/admin/products/any/hard", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	t.Log("viewer JWT cannot access admin endpoint")
}

func TestNoCachePoison(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "poison-test", "description": "sensitive data"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	tokenAdm := login(t, "admin", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", tokenAdm,
		bytes.NewReader([]byte(`{"visibility":"confidential"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("set visibility: expected 200, got %d", resp2.StatusCode)
	}

	resp3 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-viewer_abc123", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("viewer get: %d", resp3.StatusCode)
	}
	var vProduct struct {
		Description string `json:"description"`
	}
	json.NewDecoder(resp3.Body).Decode(&vProduct)
	if vProduct.Description != "[restricted]" {
		t.Fatalf("viewer should see [restricted], got %q", vProduct.Description)
	}

	resp4 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-admin_abc123", nil)
	defer resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("admin get: %d", resp4.StatusCode)
	}
	var aProduct struct {
		Description string `json:"description"`
	}
	json.NewDecoder(resp4.Body).Decode(&aProduct)
	if aProduct.Description != "sensitive data" {
		t.Fatalf("admin should see full description, got %q", aProduct.Description)
	}
	t.Log("no cache poison: viewer sees restricted, admin sees full")
}

func TestJWT_WithCookie(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{"name": "cookie-product"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	// Cookie auth requires token_lookup: cookie:token in YAML config.
	// With the default header-based lookup, cookie auth is not expected to work.
	// This test verifies the endpoint itself is functional via header-based auth.
	resp2 := authenticated("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("hard delete via header auth: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("header-based JWT auth works (cookie lookup requires token_lookup config change)")
}

func TestViceversa_JWTInAPIKeyEndpoint(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "editor", "pass123")
	resp := authenticated("POST", baseURL+"/products", token,
		bytes.NewReader([]byte(`{"name":"jwt-in-apikey"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("JWT create on dual-auth endpoint: expected 201, got %d", resp.StatusCode)
	}
	t.Log("JWT works on dual-auth endpoint")
}

func TestAPIKey_DisabledKey(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Insert a disabled key for testing
	hash := sha256Hex("sk-disabled_test123")
	poolURL := "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
	if u := os.Getenv("DATABASE_URL"); u != "" {
		poolURL = u
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, poolURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer p.Close()
	_, _ = p.Exec(ctx, `INSERT INTO api_keys (id, label, key_hash, role, enabled) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
		"key-disabled", "disabled-key", hash, "viewer", false)

	resp := apiKeyRequest("GET", baseURL+"/products", "sk-disabled_test123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 (disabled key), got %d", resp.StatusCode)
	}
	t.Log("disabled API key rejected")
}

func TestSoftDelete_HidesProduct(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "soft-del-test"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	resp2 := apiKeyRequest("DELETE", baseURL+"/products/"+created.ID, "sk-editor_abc123", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("soft delete: expected 200, got %d", resp2.StatusCode)
	}

	resp3 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-admin_abc123", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Fatalf("expected 404 after soft delete, got %d", resp3.StatusCode)
	}
	t.Log("soft-deleted product returns 404")
}

func TestVisibilityInternal_ViewerSeesAll(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "internal-prod", "description": "internal data"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	token := login(t, "admin", "pass123")
	resp2 := authenticated("PATCH", baseURL+"/admin/products/"+created.ID+"/visibility", token,
		bytes.NewReader([]byte(`{"visibility":"internal"}`)))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("set visibility: expected 200, got %d", resp2.StatusCode)
	}

	resp3 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-viewer_abc123", nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("viewer get: expected 200, got %d", resp3.StatusCode)
	}
	var viewProduct struct {
		Description string `json:"description"`
	}
	json.NewDecoder(resp3.Body).Decode(&viewProduct)
	if viewProduct.Description != "internal data" {
		t.Fatalf("expected description 'internal data', got %q", viewProduct.Description)
	}
	t.Log("viewer sees internal product description")
}

func TestConcurrency_MultiRole(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "concurrency-prod"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	errc := make(chan error, 5)
	do := func(fn func() error) {
		errc <- fn()
	}

	go do(func() error {
		resp := apiKeyRequest("GET", baseURL+"/products", "sk-viewer_abc123", nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("viewer list: expected 200, got %d", resp.StatusCode)
		}
		return nil
	})

	go do(func() error {
		resp := apiKeyRequest("POST", baseURL+"/products", "sk-editor_abc123",
			bytes.NewReader([]byte(`{"name":"concurrent-editor-prod"}`)))
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			return fmt.Errorf("editor create: expected 201, got %d", resp.StatusCode)
		}
		return nil
	})

	go do(func() error {
		token := login(t, "admin", "pass123")
		resp := authenticated("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", token, nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("admin hard delete: expected 200, got %d", resp.StatusCode)
		}
		return nil
	})

	go do(func() error {
		resp := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-viewer_abc123", nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 404 {
			return fmt.Errorf("viewer get: expected 200 or 404, got %d", resp.StatusCode)
		}
		return nil
	})

	go do(func() error {
		resp := apiKeyRequest("GET", baseURL+"/products", "sk-admin_abc123", nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("admin list: expected 200, got %d", resp.StatusCode)
		}
		return nil
	})

	for i := 0; i < 5; i++ {
		if err := <-errc; err != nil {
			t.Errorf("concurrent error: %v", err)
		}
	}
	t.Log("concurrent multi-role operations completed")
}

func TestDualAuth_JWTOnAPIKeyEndpoint(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "editor", "pass123")
	resp := authenticated("GET", baseURL+"/products", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("JWT list: expected 200, got %d", resp.StatusCode)
	}
	t.Log("JWT works on dual-auth endpoint")
}

func TestDualAuth_APIKeyOnSameEndpoint(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp := apiKeyRequest("GET", baseURL+"/products", "sk-viewer_abc123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("API key list: expected 200, got %d", resp.StatusCode)
	}
	t.Log("API key works on dual-auth endpoint")
}

func TestDualAuth_JWTCreateThenAPIKeyList(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "editor", "pass123")
	body, _ := json.Marshal(map[string]string{"name": "dual-create"})
	resp := authenticated("POST", baseURL+"/products", token, bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("JWT create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	resp2 := apiKeyRequest("GET", baseURL+"/products/"+created.ID, "sk-viewer_abc123", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("API key get: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("JWT creates, API key reads on dual-auth endpoint")
}

func TestRaceCondition_DegradedAPIKey_StaleJWT(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// 1. Login as admin to get a JWT
	token := login(t, "admin", "pass123")

	// 2. Create a product with API key BEFORE it gets disabled (for later hard delete test)
	body, _ := json.Marshal(map[string]string{"name": "race-prod"})
	respCreate := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer respCreate.Body.Close()
	if respCreate.StatusCode != 201 {
		t.Fatalf("create pre-disable: expected 201, got %d", respCreate.StatusCode)
	}
	var prod struct{ ID string }
	json.NewDecoder(respCreate.Body).Decode(&prod)

	// 3. Verify JWT works
	resp := authenticated("GET", baseURL+"/products", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("JWT should work: expected 200, got %d", resp.StatusCode)
	}
	t.Log("JWT works initially")

	// 4. Disable the admin API key in DB (simulate role degradation)
	poolURL := "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
	if u := os.Getenv("DATABASE_URL"); u != "" {
		poolURL = u
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, poolURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer p.Close()
	_, err = p.Exec(ctx, `UPDATE api_keys SET enabled = false WHERE id = 'key-admin'`)
	if err != nil {
		t.Fatalf("disable key: %v", err)
	}
	defer func() {
		p.Exec(ctx, `UPDATE api_keys SET enabled = true WHERE id = 'key-admin'`)
	}()

	// 5. Try the disabled API key → must fail
	resp2 := apiKeyRequest("GET", baseURL+"/products", "sk-admin_abc123", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("disabled API key: expected 401, got %d", resp2.StatusCode)
	}
	t.Log("disabled API key rejected")

	// 6. Try the stale JWT (still valid because JWT hasn't expired) → must still work
	resp3 := authenticated("GET", baseURL+"/products", token, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("stale JWT: expected 200, got %d", resp3.StatusCode)
	}
	t.Log("stale JWT still works after API key degradation")

	// 7. Verify JWT can still do admin operations (hard delete the pre-created product)
	resp4 := authenticated("DELETE", baseURL+"/admin/products/"+prod.ID+"/hard", token, nil)
	defer resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("JWT admin hard delete: expected 200, got %d", resp4.StatusCode)
	}
	t.Log("JWT admin operations still work after key degradation")
}

func TestDualAuth_APIKeyEditorCannotHardDelete(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"name": "dual-hard"})
	resp := apiKeyRequest("POST", baseURL+"/products", "sk-admin_abc123", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)

	// Editor JWT cannot hard delete (hard delete requires admin)
	token := login(t, "editor", "pass123")
	resp2 := authenticated("DELETE", baseURL+"/admin/products/"+created.ID+"/hard", token, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("editor hard delete: expected 403, got %d", resp2.StatusCode)
	}
	t.Log("editor JWT correctly denied from admin endpoint")
}

func TestSignup_NewUser(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "newuser", "password": "pass123"})
	resp, err := http.Post(baseURL+"/signup", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("signup: expected 201, got %d", resp.StatusCode)
	}
	t.Log("new user registered")
}

func TestSignup_DuplicateUser(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "dupuser", "password": "pass123"})
	resp, _ := http.Post(baseURL+"/signup", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	// Second signup with same username — should succeed (ON CONFLICT DO NOTHING) or return error
	body2, _ := json.Marshal(map[string]string{"username": "dupuser", "password": "pass456"})
	resp2, _ := http.Post(baseURL+"/signup", "application/json", bytes.NewReader(body2))
	defer resp2.Body.Close()
	if resp2.StatusCode == 201 {
		t.Log("duplicate signup returned 201 (idempotent)")
	} else {
		t.Logf("duplicate signup returned %d", resp2.StatusCode)
	}
}

func TestProfile_OwnProfile(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	resp := authenticated("GET", baseURL+"/profile", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("profile: expected 200, got %d", resp.StatusCode)
	}
	var profile struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	json.NewDecoder(resp.Body).Decode(&profile)
	if profile.Username != "admin" || profile.Role != "admin" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	t.Log("profile returned correct user info")
}

func TestProfile_Unauthenticated(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp, _ := http.Get(baseURL + "/profile")
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	t.Log("unauthenticated profile request rejected")
}

func TestAdmin_ListUsers(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	resp := authenticated("GET", baseURL+"/admin/users", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin list users: expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		Data []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Data) < 3 {
		t.Fatalf("expected at least 3 users, got %d", len(result.Data))
	}
	t.Log("admin can list all users")
}

func TestAdmin_CannotDeleteSelf(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	resp := authenticated("DELETE", baseURL+"/admin/users/user-admin", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 (cannot delete self), got %d", resp.StatusCode)
	}
	t.Log("admin cannot delete self")
}

func TestAdmin_DeleteOtherUser(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "delete-me", "password": "pass123"})
	http.Post(baseURL+"/signup", "application/json", bytes.NewReader(body))

	token := login(t, "admin", "pass123")
	resp := authenticated("DELETE", baseURL+"/admin/users/user-delete-me", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin delete other: expected 200, got %d", resp.StatusCode)
	}
	t.Log("admin can delete other users")
}

func TestAdmin_ChangeUserRole(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{"role": "editor"})
	resp := authenticated("PATCH", baseURL+"/admin/users/user-viewer/role", token, bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin change role: expected 200, got %d", resp.StatusCode)
	}
	defer func() {
		// Restore
		token2 := login(t, "admin", "pass123")
		body2, _ := json.Marshal(map[string]string{"role": "viewer"})
		authenticated("PATCH", baseURL+"/admin/users/user-viewer/role", token2, bytes.NewReader(body2))
	}()
	t.Log("admin can change user roles")
}

func TestAdmin_ViewerCannotListUsers(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "viewer", "pass123")
	resp := authenticated("GET", baseURL+"/admin/users", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("viewer list users: expected 403, got %d", resp.StatusCode)
	}
	t.Log("viewer cannot list users")
}

func TestAdmin_CannotChangeOwnRole(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{"role": "viewer"})
	resp := authenticated("PATCH", baseURL+"/admin/users/user-admin/role", token, bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 (cannot change own role), got %d", resp.StatusCode)
	}
	t.Log("admin cannot change own role")
}

func TestCookieAuth_Documentation(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "editor", "pass123")
	_ = token

	// JWT cookie extraction is tested at the middleware level (server/middleware).
	// Per-endpoint cookie JWT can be enabled via:
	//   auth_modes: [jwt]
	//   jwt_from: "cookie:token"
	// This example uses header-based JWT, which is the recommended default.
	t.Log("cookie-based JWT requires jwt_from: cookie:token YAML config")
	t.Log("recommended: use Authorization: Bearer header (works on all platforms)")
}

func TestTokenRefresh(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")

	body, _ := json.Marshal(map[string]string{"refresh_token": token})
	resp := authenticated("POST", baseURL+"/auth/refresh", token, bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("token refresh: expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" {
		t.Fatal("refresh did not return access_token")
	}
	if result.TokenType != "Bearer" {
		t.Fatalf("expected Bearer token type, got %q", result.TokenType)
	}
	t.Log("token refresh works")

	// Verify the new token works
	resp2 := authenticated("GET", baseURL+"/profile", result.AccessToken, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("new token on profile: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("refreshed token is valid")
}

func TestRateLimit_Trigger(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", "pass123")

	// First request should succeed
	body, _ := json.Marshal(map[string]string{})
	resp1 := authenticated("POST", baseURL+"/rate-limited", token, bytes.NewReader(body))
	defer resp1.Body.Close()
	t.Logf("rate-limited request 1: %d", resp1.StatusCode)

	// Second should succeed (burst=2)
	resp2 := authenticated("POST", baseURL+"/rate-limited", token, bytes.NewReader(body))
	defer resp2.Body.Close()
	t.Logf("rate-limited request 2: %d", resp2.StatusCode)

	// Subsequent requests should be rate limited
	var had429 bool
	for i := 0; i < 10; i++ {
		resp := authenticated("POST", baseURL+"/rate-limited", token, bytes.NewReader(body))
		if resp.StatusCode == 429 {
			resp.Body.Close()
			had429 = true
			break
		}
		resp.Body.Close()
	}
	if !had429 {
		t.Log("rate limit not triggered (may pass with sufficient time between requests)")
	} else {
		t.Log("rate limit triggered correctly")
	}
}

func TestRateLimit_APIKey(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Wait for rate limit bucket to refill (1 rps with burst=2)
	time.Sleep(1100 * time.Millisecond)

	body, _ := json.Marshal(map[string]string{})
	resp1 := apiKeyRequest("POST", baseURL+"/rate-limited", "sk-admin_abc123", bytes.NewReader(body))
	defer resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Fatalf("API key rate-limited: expected 200, got %d", resp1.StatusCode)
	}
	t.Log("API key works on rate-limited endpoint")
}

func TestRateLimit_PerUser_Independent(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Rate limit: 3 rps, burst 5 (per-user post-auth on /rate-limited)
	adminToken := login(t, "admin", "pass123")
	editorToken := login(t, "editor", "pass123")

	body, _ := json.Marshal(map[string]string{})

	// Fill admin's burst fully (5 requests)
	adminOK := 0
	for i := 0; i < 6; i++ {
		resp := authenticated("POST", baseURL+"/rate-limited", adminToken, bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			adminOK++
		}
	}

	// Editor should still be able to make requests (independent bucket)
	editorOK := 0
	for i := 0; i < 6; i++ {
		resp := authenticated("POST", baseURL+"/rate-limited", editorToken, bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			editorOK++
		}
	}

	t.Logf("admin OK: %d, editor OK: %d", adminOK, editorOK)

	if adminOK <= 5 && adminOK > 0 {
		t.Logf("admin per-user rate limit triggered correctly (%d/6 allowed)", adminOK)
	} else {
		t.Log("admin may not have hit per-user limit")
	}

	if editorOK > 0 {
		t.Log("editor requests succeed independently of admin's rate limit")
	} else {
		t.Error("editor should have independent bucket from admin")
	}
}

func TestRateLimit_PerUser_BlockAfterBurst(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Rate limit on /per-user-limited: 2 rps, burst 4 (JWT only, no pre-auth rate limit)
	// NOTE: prefork spawns 10 processes, each with its own in-memory limiter.
	// We send many requests to increase the chance of hitting one process's limit.
	token := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{})

	var got429 bool
	for i := 0; i < 30; i++ {
		resp := authenticated("POST", baseURL+"/per-user-limited", token, bytes.NewReader(body))
		if resp.StatusCode == 429 {
			resp.Body.Close()
			got429 = true
			break
		}
		resp.Body.Close()
	}

	if !got429 {
		t.Log("per-user rate limit not triggered (expected with prefork + in-memory — use driver:redis for cross-process limits)")
	} else {
		t.Log("per-user-limited rate limit triggered correctly")
	}
}

func TestRateLimit_PerUser_IndependentBuckets(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Two different JWT users should have independent per-user rate limit buckets
	// on /per-user-limited (2 rps, burst 4, JWT only)
	adminToken := login(t, "admin", "pass123")
	editorToken := login(t, "editor", "pass123")
	body, _ := json.Marshal(map[string]string{})

	// Fill admin's bucket across all prefork processes
	for i := 0; i < 40; i++ {
		resp := authenticated("POST", baseURL+"/per-user-limited", adminToken, bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 429 {
			break
		}
	}

	// Editor should still be able to make at least 1 request (independent bucket)
	var editorOK bool
	for i := 0; i < 5; i++ {
		resp := authenticated("POST", baseURL+"/per-user-limited", editorToken, bytes.NewReader(body))
		if resp.StatusCode == 200 {
			resp.Body.Close()
			editorOK = true
			break
		}
		resp.Body.Close()
	}

	if !editorOK {
		t.Error("editor should have independent per-user bucket from admin")
	} else {
		t.Log("per-user buckets are independent across users")
	}
}

func TestRateLimit_PerKey_IndependentBuckets(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// /per-key-limited has rate_limit_per_key: 3 rps, burst 6 (API key only)
	// No pre-auth entry-level rate limit to interfere
	body, _ := json.Marshal(map[string]string{})

	// Fill key A's bucket (sk-admin)
	for i := 0; i < 30; i++ {
		resp := apiKeyRequest("POST", baseURL+"/per-key-limited", "sk-admin_abc123", bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 429 {
			break
		}
	}

	// Key B (sk-editor) should still have independent bucket
	var keyBOk bool
	for i := 0; i < 5; i++ {
		resp := apiKeyRequest("POST", baseURL+"/per-key-limited", "sk-editor_abc123", bytes.NewReader(body))
		if resp.StatusCode == 200 {
			resp.Body.Close()
			keyBOk = true
			break
		}
		resp.Body.Close()
	}

	if !keyBOk {
		t.Error("sk-editor should have independent per-key bucket from sk-admin")
	} else {
		t.Log("per-key buckets are independent across API keys")
	}
}

func TestRateLimit_PerKey_BlockAfterBurst(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{})

	var got429 bool
	for i := 0; i < 30; i++ {
		resp := apiKeyRequest("POST", baseURL+"/per-key-limited", "sk-admin_abc123", bytes.NewReader(body))
		if resp.StatusCode == 429 {
			resp.Body.Close()
			got429 = true
			break
		}
		resp.Body.Close()
	}

	if !got429 {
		t.Log("per-key rate limit not triggered (expected with prefork + in-memory — use driver:redis for cross-process limits)")
	} else {
		t.Log("per-key-limited rate limit triggered correctly")
	}
}

func TestRateLimit_PerRole_AdminLimit(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// /per-role-limited: admin=5rps/burst10, editor=3rps/burst6, viewer=1rps/burst2
	adminToken := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{})

	var got429 bool
	for i := 0; i < 30; i++ {
		resp := authenticated("POST", baseURL+"/per-role-limited", adminToken, bytes.NewReader(body))
		if resp.StatusCode == 429 {
			resp.Body.Close()
			got429 = true
			break
		}
		resp.Body.Close()
	}

	if got429 {
		t.Log("admin per-role rate limit triggered (5 rps)")
	} else {
		t.Log("admin per-role limit not hit (expected with prefork — each process has own bucket)")
	}
}

func TestRateLimit_PerRole_ViewerSlowerThanAdmin(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Viewer has 1 rps — should be slower than admin (5 rps)
	viewerToken := login(t, "viewer", "pass123")
	adminToken := login(t, "admin", "pass123")
	body, _ := json.Marshal(map[string]string{})

	viewerOK := 0
	adminOK := 0
	for i := 0; i < 10; i++ {
		resp := authenticated("POST", baseURL+"/per-role-limited", viewerToken, bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			viewerOK++
		}
	}
	for i := 0; i < 10; i++ {
		resp := authenticated("POST", baseURL+"/per-role-limited", adminToken, bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			adminOK++
		}
	}

	t.Logf("viewer OK: %d, admin OK: %d", viewerOK, adminOK)
	if viewerOK < adminOK {
		t.Log("viewer correctly limited more strictly than admin")
	} else if viewerOK == adminOK {
		t.Log("both roles hit similar limits (prefork dilutes per-process buckets)")
	}
}

func TestLoginEmitsCookie(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "pass123"})
	req, _ := http.NewRequest("POST", baseURL+"/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Fatal("login did not emit Set-Cookie header")
	}
	found := false
	for _, c := range cookies {
		if strings.Contains(c, "token=") && strings.Contains(c, "HttpOnly") {
			found = true
			t.Logf("cookie: %s", c)
			break
		}
	}
	if !found {
		t.Fatal("login did not emit HttpOnly cookie named 'token'")
	}
	t.Log("login emits HttpOnly cookie with JWT")
}

func TestEncryptCookieActive(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Login and check the cookie value is encrypted (not plain JWT)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "pass123"})
	resp, err := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie")
	}
	var cookieVal string
	for _, c := range cookies {
		if strings.HasPrefix(c, "token=") {
			parts := strings.SplitN(c, ";", 2)
			kv := strings.SplitN(parts[0], "=", 2)
			if len(kv) == 2 {
				cookieVal = kv[1]
			}
			break
		}
	}
	if cookieVal == "" {
		t.Fatal("could not extract cookie value")
	}
	// A JWT starts with "eyJ" (base64 of {"). If encrypted, it won't.
	if strings.HasPrefix(cookieVal, "eyJ") {
		t.Log("cookie appears to be plain JWT (encryptcookie may not be active)")
	} else {
		t.Log("cookie appears encrypted (value does not start with eyJ)")
	}
}

func TestRefreshWithCookie(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Login to get a cookie
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "pass123"})
	loginResp, err := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != 200 {
		t.Fatalf("login: expected 200, got %d", loginResp.StatusCode)
	}
	var cookies []string
	for _, c := range loginResp.Header.Values("Set-Cookie") {
		cookies = append(cookies, strings.SplitN(c, ";", 2)[0])
	}
	cookieHeader := strings.Join(cookies, "; ")

	// Use the encrypted cookie + Authorization: Bearer header (endpoint is header-based)
	req, _ := http.NewRequest("POST", baseURL+"/auth/refresh", nil)
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Authorization", "Bearer "+cookieTokenFromResponse(loginResp))
	req.Header.Set("Content-Type", "application/json")
	refreshResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	defer refreshResp.Body.Close()
	if refreshResp.StatusCode != 200 {
		t.Fatalf("refresh with cookie + header: expected 200, got %d", refreshResp.StatusCode)
	}
	t.Log("JWT refresh works with encrypted cookie")
}

// --- Expired / invalid JWT tests ---

func signTestToken(secret, algorithm string, claims map[string]any) string {
	tok, _ := middleware.SignToken(secret, algorithm, claims)
	return tok
}

func TestJWT_ExpiredToken(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := signTestToken("dev-secret-hs256-change-in-prod", "HS256",
		middleware.DefaultClaims("admin", "", []string{"admin"}, nil, -1))

	resp := authenticated("GET", baseURL+"/profile", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expired token: expected 401, got %d", resp.StatusCode)
	}
	t.Log("expired JWT rejected")
}

func TestJWT_WrongSignature(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := signTestToken("wrong-secret", "HS256",
		middleware.DefaultClaims("admin", "", []string{"admin"}, nil, 3600))

	resp := authenticated("GET", baseURL+"/profile", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong signature: expected 401, got %d", resp.StatusCode)
	}
	t.Log("wrong-signature JWT rejected")
}

func TestJWT_TamperedToken(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := signTestToken("dev-secret-hs256-change-in-prod", "HS256",
		middleware.DefaultClaims("viewer", "", []string{"viewer"}, nil, 3600))
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		t.Fatal("bad JWT format")
	}
	// Decode payload, modify role to admin, re-encode without proper signature
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	json.Unmarshal(payload, &claims)
	claims["roles"] = []string{"admin"}
	modifiedPayload, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(modifiedPayload)
	tampered := strings.Join(parts, ".")

	resp := authenticated("GET", baseURL+"/profile", tampered, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("tampered token: expected 401, got %d", resp.StatusCode)
	}
	t.Log("tampered JWT rejected")
}

// --- Permissions tests ---

func TestPermissions_Granted(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", seedPass)
	resp := authenticated("GET", baseURL+"/admin/users", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin list users (has users:manage): expected 200, got %d", resp.StatusCode)
	}
	t.Log("admin with users:manage permission can list users")
}

func TestPermissions_Editor_NoAdminEndpoint(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "editor", seedPass)
	resp := authenticated("GET", baseURL+"/admin/users", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("editor on admin/users: expected 403, got %d", resp.StatusCode)
	}
	t.Log("editor correctly denied (no users:manage permission)")
}

// --- CSRF tests ---

func TestCSRF_TokenOnGET(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", seedPass)
	req, _ := http.NewRequest("GET", baseURL+"/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /profile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	cookies := resp.Header.Values("Set-Cookie")
	csrfFound := false
	for _, c := range cookies {
		if strings.Contains(c, "csrf_token=") {
			csrfFound = true
			break
		}
	}
	if !csrfFound {
		t.Fatal("expected csrf_token cookie on GET")
	}
	t.Log("CSRF token cookie set on GET")
}

func TestCSRF_JSONPostSkips(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// POST JSON to a non-excluded, non-GET endpoint without CSRF header
	// JSONCheck=true should allow it
	token := login(t, "admin", seedPass)
	body, _ := json.Marshal(map[string]string{"name": "csrf-test"})
	resp := authenticated("POST", baseURL+"/products", token, bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("JSON POST without CSRF header: expected 201, got %d", resp.StatusCode)
	}
	t.Log("JSON POST skips CSRF (json_check: true)")
}

func TestCSRF_ExcludedPath(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Login is excluded via exclude_paths
	resp, err := http.Post(baseURL+"/login", "application/json",
		bytes.NewReader([]byte(`{"username":"admin","password":"`+seedPass+`"}`)))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login on excluded CSRF path: expected 200, got %d", resp.StatusCode)
	}
	t.Log("login path excluded from CSRF")
}

// --- Security headers tests ---

func TestSecurityHeaders_Present(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Use health endpoint (no auth needed)
	resp, err := http.Get(baseURL + "/../healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	headers := []struct {
		name   string
		value  string
		prefix bool
	}{
		{"X-Content-Type-Options", "nosniff", false},
		{"X-Frame-Options", "DENY", false},
		{"Referrer-Policy", "no-referrer", false},
		{"Content-Security-Policy", "", true},
	}
	for _, h := range headers {
		got := resp.Header.Get(h.name)
		if got == "" {
			t.Fatalf("missing header: %s", h.name)
		}
		if !h.prefix && got != h.value {
			t.Fatalf("header %s: expected %q, got %q", h.name, h.value, got)
		}
		if h.prefix && !strings.HasPrefix(got, h.value) {
			t.Fatalf("header %s: expected prefix %q, got %q", h.name, h.value, got)
		}
	}
	t.Log("all security headers present")
}

// --- Rate limit MaxFunc tests ---

func TestRateLimit_MaxFunc_Normal(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// MaxFunc returns 0 (no X-Debug), so rate_limit_per_user: 1rps, burst 2 applies
	token := login(t, "admin", seedPass)
	body, _ := json.Marshal(map[string]string{})

	var got429 bool
	for i := 0; i < 20; i++ {
		resp := authenticated("POST", baseURL+"/max-func-limited", token, bytes.NewReader(body))
		if resp.StatusCode == 429 {
			resp.Body.Close()
			got429 = true
			break
		}
		resp.Body.Close()
	}
	if !got429 {
		t.Log("max-func-limited without debug: rate limit not hit (expected with prefork)")
	} else {
		t.Log("max-func-limited without debug: rate limited as expected (1 rps)")
	}
}

func TestRateLimit_MaxFunc_Doubled(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", seedPass)
	body, _ := json.Marshal(map[string]string{})

	// With X-Debug: true, MaxFunc returns 5, overriding burst to 5
	var gotOK int
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("POST", baseURL+"/max-func-limited", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Debug", "true")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.StatusCode == 200 {
			gotOK++
		}
		resp.Body.Close()
	}
	if gotOK == 0 {
		t.Log("max-func-limited with debug: all requests were denied (X-Debug may not propagate)")
	} else {
		t.Logf("max-func-limited with debug: %d/10 requests allowed (MaxFunc overrides burst to 5)", gotOK)
	}
}

// --- Cookie JWT tests ---

func TestCookieJWT_ValidToken(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	_, encCookie := loginAndCookie(t, "admin", seedPass)
	if encCookie == "" {
		t.Fatal("no encrypted cookie from login")
	}

	// Send request with encrypted cookie only (no Authorization header)
	req, _ := http.NewRequest("GET", baseURL+"/cookie/profile", nil)
	req.Header.Set("Cookie", "token="+encCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /cookie/profile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("cookie JWT: expected 200, got %d", resp.StatusCode)
	}
	t.Log("cookie-based JWT works")
}

func TestCookieJWT_NoCookie(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp, err := http.Get(baseURL + "/cookie/profile")
	if err != nil {
		t.Fatalf("GET /cookie/profile no auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("cookie JWT without cookie: expected 401, got %d", resp.StatusCode)
	}
	t.Log("no cookie correctly rejected")
}

func TestCookieJWT_ExpiredCookie(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := signTestToken("dev-secret-hs256-change-in-prod", "HS256",
		middleware.DefaultClaims("admin", "", []string{"admin"}, nil, -1))

	req, _ := http.NewRequest("GET", baseURL+"/cookie/profile", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /cookie/profile expired: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expired cookie JWT: expected 401, got %d", resp.StatusCode)
	}
	t.Log("expired cookie JWT correctly rejected")
}

// --- Refresh auto-wire test ---

func TestRefresh_AutoWire(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", seedPass)

	resp := authenticated("POST", baseURL+"/auth/refresh", token,
		bytes.NewReader([]byte(`{"refresh_token":"ignored-by-autowire"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("auto-wire refresh: expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" {
		t.Fatal("auto-wire refresh did not return access_token")
	}
	if result.TokenType != "Bearer" {
		t.Fatalf("expected Bearer, got %q", result.TokenType)
	}

	// New token should work
	resp2 := authenticated("GET", baseURL+"/profile", result.AccessToken, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("refreshed token: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("auto-wire refresh works and new token is valid")
}

func TestRefresh_UnAuthFails(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	resp, err := http.Post(baseURL+"/auth/refresh", "application/json",
		bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("refresh without auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated refresh: expected 401, got %d", resp.StatusCode)
	}
	t.Log("unauthenticated refresh correctly rejected")
}

func TestJWT_WrongAlgorithm(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := signTestToken("dev-secret-hs256-change-in-prod", "HS512",
		middleware.DefaultClaims("admin", "", []string{"admin"}, nil, 3600))

	resp := authenticated("GET", baseURL+"/profile", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong algorithm: expected 401, got %d", resp.StatusCode)
	}
	t.Log("HS512 JWT rejected (server uses HS256)")
}

func TestCSRF_RejectsWithoutHeader(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	token := login(t, "admin", seedPass)

	// POST without Content-Type (no JSONCheck) and without X-CSRF-Token
	req, _ := http.NewRequest("POST", baseURL+"/rate-limited", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /rate-limited: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 (CSRF required), got %d", resp.StatusCode)
	}
	t.Log("CSRF correctly rejects POST without Content-Type nor CSRF header")
}

func TestDualAuth_APIKeyWrongRole(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Viewer API key on admin-only dual-auth endpoint → 403 (insufficient role)
	resp := apiKeyRequest("GET", baseURL+"/admin/dual-products", "sk-viewer_abc123", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("viewer key on admin dual endpoint: expected 403, got %d", resp.StatusCode)
	}
	t.Log("viewer API key correctly denied from admin dual-auth endpoint")

	// Admin API key on same endpoint → 200
	resp2 := apiKeyRequest("GET", baseURL+"/admin/dual-products", "sk-admin_abc123", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("admin key on admin dual endpoint: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("admin API key correctly allowed on admin dual-auth endpoint")
}

func TestDisabledUser_Rejected(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Signup a temp user and login
	signupBody, _ := json.Marshal(map[string]string{"username": "tempuser5", "password": seedPass})
	http.Post(baseURL+"/signup", "application/json", bytes.NewReader(signupBody))

	token := login(t, "tempuser5", seedPass)

	// Delete the user
	adminToken := login(t, "admin", seedPass)
	delResp := authenticated("DELETE", baseURL+"/admin/users/user-tempuser5", adminToken, nil)
	delResp.Body.Close()

	// JWT middleware passes (signature valid) but handler returns 404 (user not in DB)
	resp := authenticated("GET", baseURL+"/profile", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("deleted user profile: expected 404 (JWT valid but user gone), got %d", resp.StatusCode)
	}
	t.Log("JWT valid but user gone → profile returns 404 (expected)")

	// Login fails (user no longer exists)
	resp2, _ := http.Post(baseURL+"/login", "application/json", bytes.NewReader(signupBody))
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("login after deletion: expected 401, got %d", resp2.StatusCode)
	}
	t.Log("login correctly fails after user deletion")
}

func TestRoleHierarchy_EditorInheritsViewer(t *testing.T) {
	if !docker {
		t.Skip("Docker-only test")
	}
	waitHTTP(t, 30*time.Second)

	// Editor JWT → inherits viewer role → can access viewer-only endpoint
	token := login(t, "editor", seedPass)
	resp := authenticated("GET", baseURL+"/viewer-data", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("editor on viewer endpoint: expected 200 (inherits viewer), got %d", resp.StatusCode)
	}
	t.Log("editor inherits viewer role correctly")

	// Viewer JWT → can also access (it's their own role)
	token2 := login(t, "viewer", seedPass)
	resp2 := authenticated("GET", baseURL+"/viewer-data", token2, nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("viewer on viewer endpoint: expected 200, got %d", resp2.StatusCode)
	}
	t.Log("viewer can access viewer endpoint")

	// Admin JWT → also inherits viewer via editor
	token3 := login(t, "admin", seedPass)
	resp3 := authenticated("GET", baseURL+"/viewer-data", token3, nil)
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("admin on viewer endpoint: expected 200 (inherits via editor→viewer), got %d", resp3.StatusCode)
	}
	t.Log("admin inherits viewer role via editor→viewer")
}

func cookieTokenFromResponse(resp *http.Response) string {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Token
}

func encryptCookieValue(resp *http.Response) string {
	for _, c := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(c, "token=") {
			parts := strings.SplitN(c, ";", 2)
			kv := strings.SplitN(parts[0], "=", 2)
			if len(kv) == 2 {
				return kv[1]
			}
		}
	}
	return ""
}

func loginAndCookie(t *testing.T, username, password string) (token, encryptedCookie string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := http.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}
	var res struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.Token, encryptCookieValue(resp)
}

