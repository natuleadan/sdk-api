package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
)

const (
	httpPort = "23400"
	baseURL  = "http://localhost:" + httpPort + "/api"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func zitadelURL() string {
	if v := os.Getenv("ZITADEL_URL"); v != "" {
		return v
	}
	return "http://localhost:18082"
}

// machineToken returns an opaque Zitadel access token for the example project
// audience, plus the FGA subject. The service validates it via introspection.
func machineToken(t *testing.T) (token, subject string) {
	t.Helper()
	mkPath := os.Getenv("ZITADEL_MACHINEKEY")
	if mkPath == "" {
		mkPath = ".machinekey/machinekey.json"
	}
	raw, err := os.ReadFile(mkPath)
	if err != nil {
		t.Skipf("machine key not available (%s): %v", mkPath, err)
	}
	var mk struct {
		KeyID  string `json:"keyId"`
		Key    string `json:"key"`
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(raw, &mk); err != nil || mk.Key == "" || mk.UserID == "" {
		t.Fatalf("machine key invalid (%s): %v", mkPath, err)
	}

	bootPath := os.Getenv("ZITADEL_BOOTSTRAP_FILE")
	if bootPath == "" {
		bootPath = ".runtime/zitadel.json"
	}
	braw, err := os.ReadFile(bootPath)
	if err != nil {
		t.Skipf("bootstrap info not available (%s): %v", bootPath, err)
	}
	var boot struct {
		Issuer    string `json:"issuer"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(braw, &boot); err != nil || boot.ProjectID == "" {
		t.Fatalf("bootstrap info invalid (%s): %v", bootPath, err)
	}
	issuer := boot.Issuer
	if issuer == "" {
		issuer = zitadelURL()
	}

	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(mk.Key))
	if err != nil {
		t.Fatalf("machine key parse pem: %v", err)
	}
	now := time.Now()
	claimsTok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": mk.UserID, "sub": mk.UserID, "aud": issuer,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	claimsTok.Header["kid"] = mk.KeyID
	signed, err := claimsTok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", signed)
	form.Set("scope", "openid urn:zitadel:iam:org:project:id:"+boot.ProjectID+":aud")
	resp, err := httpClient.PostForm(issuer+"/oauth/v2/token", form)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		t.Fatalf("token response (%d): %s", resp.StatusCode, string(body))
	}
	return out.AccessToken, "user:" + mk.UserID
}

func fgaStoreID(t *testing.T) string {
	t.Helper()
	base := os.Getenv("OPENFGA_URL")
	if base == "" {
		base = "http://localhost:18080"
	}
	resp, err := httpClient.Get(base + "/stores")
	if err != nil {
		t.Skipf("openfga not available: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var list struct {
		Stores []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stores"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list stores: %v", err)
	}
	for _, s := range list.Stores {
		if s.Name == "auth-openfga-zitadel" {
			return s.ID
		}
	}
	t.Skip("openfga store not created yet (is the service running?)")
	return ""
}

func fgaClient(t *testing.T) *openfga.Client {
	t.Helper()
	base := os.Getenv("OPENFGA_URL")
	if base == "" {
		base = "http://localhost:18080"
	}
	c, err := openfga.NewClient(openfga.Config{APIURL: base, StoreID: fgaStoreID(t)})
	if err != nil {
		t.Fatalf("fga client: %v", err)
	}
	return c
}

func grantRole(t *testing.T, client *openfga.Client, subject, role string) {
	t.Helper()
	if err := client.AssignRole(context.Background(), subject, role); err != nil {
		t.Fatalf("assign role %s: %v", role, err)
	}
	t.Cleanup(func() { _ = client.DeleteTuple(context.Background(), subject, "member", "role:"+role) })
}

func do(t *testing.T, method, path, token string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	return resp
}

func status(t *testing.T, method, path, token string, payload any) int {
	t.Helper()
	resp := do(t, method, path, token, payload)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// ---- tests ----

func TestZitadelTokenAndProducts(t *testing.T) {
	token, _ := machineToken(t)
	if code := status(t, http.MethodGet, "/products", token, nil); code != 200 {
		t.Fatalf("GET /products = %d, want 200", code)
	}
}

func TestUnauthenticated(t *testing.T) {
	if code := status(t, http.MethodGet, "/products", "", nil); code != 401 {
		t.Fatalf("GET /products without token = %d, want 401", code)
	}
}

func TestViewerRole(t *testing.T) {
	token, subj := machineToken(t)
	fga := fgaClient(t)
	grantRole(t, fga, subj, "viewer")

	if code := status(t, http.MethodGet, "/viewer-data", token, nil); code != 200 {
		t.Errorf("viewer GET /viewer-data = %d, want 200", code)
	}
	if code := status(t, http.MethodGet, "/admin/users", token, nil); code != 403 {
		t.Errorf("viewer GET /admin/users = %d, want 403", code)
	}
}

func TestEditorRole(t *testing.T) {
	token, subj := machineToken(t)
	fga := fgaClient(t)
	grantRole(t, fga, subj, "editor")

	code := status(t, http.MethodPost, "/products", token, map[string]any{"name": "Editor Item", "price": 5.0})
	if code != 201 {
		t.Errorf("editor POST /products = %d, want 201", code)
	}
	// editor cannot hard-delete (admin only)
	if code := status(t, http.MethodDelete, "/admin/products/prod-1/hard", token, nil); code != 403 {
		t.Errorf("editor hard delete = %d, want 403", code)
	}
}

func TestAdminRoleAndPermission(t *testing.T) {
	token, subj := machineToken(t)
	fga := fgaClient(t)
	grantRole(t, fga, subj, "admin")

	if code := status(t, http.MethodGet, "/admin/users", token, nil); code != 200 {
		t.Errorf("admin GET /admin/users = %d, want 200", code)
	}
	resp := do(t, http.MethodPatch, "/admin/products/prod-2/visibility", token, map[string]any{"visibility": "public"})
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("admin set visibility = %d, want 200", resp.StatusCode)
	}
	// Withdraw the role: authorization must immediately deny.
	if err := fga.DeleteTuple(context.Background(), subj, "member", "role:admin"); err != nil {
		t.Fatalf("revoke admin: %v", err)
	}
	if code := status(t, http.MethodGet, "/admin/users", token, nil); code != 403 {
		t.Errorf("after revoke GET /admin/users = %d, want 403", code)
	}
}

func TestSetUserRoleWritesFGA(t *testing.T) {
	token, subj := machineToken(t)
	fga := fgaClient(t)
	grantRole(t, fga, subj, "admin")

	target := "user:runtime-target"
	if err := fga.DeleteTuple(context.Background(), target, "member", "role:viewer"); err != nil {
		t.Logf("pre-clean: %v", err)
	}
	code := status(t, http.MethodPatch, "/admin/users/runtime-target/role", token, map[string]any{"role": "viewer"})
	if code != 200 {
		t.Fatalf("set role = %d, want 200", code)
	}
	allowed, err := fga.Check(context.Background(), openfga.CheckRequest{
		User: target, Relation: "member", Object: "role:viewer",
	})
	if err != nil {
		t.Fatalf("check target role: %v", err)
	}
	if !allowed {
		t.Error("expected runtime-target to be viewer after set-role")
	}
	_ = fga.DeleteTuple(context.Background(), target, "member", "role:viewer")
}

func TestAPIKeyWithoutRole(t *testing.T) {
	// API keys are sent raw (the router detects them by the "sk-" prefix); a
	// valid key can read public products, but role-gated routes deny it.
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/products", nil)
	req.Header.Set("Authorization", "sk-reader_abc123")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("api key request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("api key GET /products = %d, want 200", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, baseURL+"/admin/dual-products", nil)
	req.Header.Set("Authorization", "sk-reader_abc123")
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("api key request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("api key GET /admin/dual-products = %d, want 403", resp.StatusCode)
	}
}

func TestArbitraryRoleFromPermissions(t *testing.T) {
	// A free-form role declared in auth.role_permissions inherits its grant.
	_, subj := machineToken(t)
	fga := fgaClient(t)
	grantRole(t, fga, subj, "facturacion-lectura")

	allowed, err := fga.Check(context.Background(), openfga.CheckRequest{
		User: subj, Relation: "can_read", Object: "products:read",
	})
	if err != nil {
		t.Fatalf("check arbitrary role: %v", err)
	}
	if !allowed {
		t.Error("expected facturacion-lectura to inherit products:read")
	}
}

func TestRateLimitedEndpoint(t *testing.T) {
	token, _ := machineToken(t)
	ok := 0
	for i := 0; i < 3; i++ {
		if code := status(t, http.MethodPost, "/rate-limited", token, nil); code == 200 {
			ok++
		}
	}
	if ok == 0 {
		t.Error("expected /rate-limited to allow at least one request")
	}
}

func TestHealthz(t *testing.T) {
	resp, err := httpClient.Get("http://localhost:" + httpPort + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("healthz = %d, want 200", resp.StatusCode)
	}
}

var _ = fmt.Sprintf
var _ = strings.TrimSpace
