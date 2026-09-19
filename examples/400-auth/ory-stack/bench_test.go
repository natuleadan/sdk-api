package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

const (
	httpPort = "23400"
	baseURL  = "http://localhost:" + httpPort + "/api"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func kratosPublicURL() string {
	if v := os.Getenv("KRATOS_PUBLIC_URL"); v != "" {
		return v
	}
	return "http://localhost:14433"
}

func ketoReadURL() string {
	if v := os.Getenv("KETO_READ_URL"); v != "" {
		return v
	}
	return "http://localhost:4466"
}

func ketoWriteURL() string {
	if v := os.Getenv("KETO_WRITE_URL"); v != "" {
		return v
	}
	return "http://localhost:4467"
}

// orySession logs into Ory Kratos with the bootstrapped identity and returns
// the session token plus the Kratos identity id (the Keto subject).
func orySession(t *testing.T) (token, identityID string) {
	t.Helper()
	email := os.Getenv("ORY_IDENTITY")
	if email == "" {
		email = "admin@example.com"
	}
	password := os.Getenv("ORY_PASSWORD")
	if password == "" {
		password = "Admin123!"
	}

	flowResp, err := httpClient.Get(kratosPublicURL() + "/self-service/login/api")
	if err != nil {
		t.Skipf("kratos not available: %v", err)
	}
	defer flowResp.Body.Close()
	var flow struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(flowResp.Body).Decode(&flow); err != nil || flow.ID == "" {
		t.Fatalf("login flow: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"method":     "password",
		"identifier": email,
		"password":   password,
	})
	req, _ := http.NewRequest(http.MethodPost, kratosPublicURL()+"/self-service/login?flow="+flow.ID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("login submit: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		SessionToken string `json:"session_token"`
		Session      struct {
			Identity struct {
				ID string `json:"id"`
			} `json:"identity"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.SessionToken == "" {
		t.Fatalf("login (%d): %s", resp.StatusCode, string(body))
	}
	return result.SessionToken, result.Session.Identity.ID
}

// grantRole writes a Keto role tuple (roles:<role>#assignee@user:<id>).
func grantRole(t *testing.T, subject, role string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"namespace": "roles", "object": role, "relation": "assignee",
		"subject_id": "user:" + subject,
	})
	req, _ := http.NewRequest(http.MethodPut, ketoWriteURL()+"/admin/relation-tuples", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("grant role %s: %v", role, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("grant role %s: status %d", role, resp.StatusCode)
	}
	t.Cleanup(func() { revokeRole(t, subject, role) })
}

func revokeRole(t *testing.T, subject, role string) {
	t.Helper()
	q := url.Values{}
	q.Set("namespace", "roles")
	q.Set("object", role)
	q.Set("relation", "assignee")
	q.Set("subject_id", "user:"+subject)
	req, _ := http.NewRequest(http.MethodDelete, ketoWriteURL()+"/admin/relation-tuples?"+q.Encode(), nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// ketoCheck performs a Keto permission check directly (test oracle).
func ketoCheck(t *testing.T, namespace, object, relation, subject string) bool {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"namespace": namespace, "object": object, "relation": relation,
		"subject_id": "user:" + subject,
	})
	resp, err := httpClient.Post(ketoReadURL()+"/relation-tuples/check", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("keto check: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Allowed bool `json:"allowed"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Allowed
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

func TestOrySessionAndProducts(t *testing.T) {
	token, _ := orySession(t)
	if code := status(t, http.MethodGet, "/products", token, nil); code != 200 {
		t.Fatalf("GET /products = %d, want 200", code)
	}
}

func TestUnauthenticated(t *testing.T) {
	if code := status(t, http.MethodGet, "/products", "", nil); code != 401 {
		t.Fatalf("GET /products without token = %d, want 401", code)
	}
}

func TestInvalidSession(t *testing.T) {
	if code := status(t, http.MethodGet, "/products", "not-a-real-session", nil); code != 401 {
		t.Fatalf("GET /products with bogus token = %d, want 401", code)
	}
}

func TestViewerRole(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "viewer")

	if code := status(t, http.MethodGet, "/viewer-data", token, nil); code != 200 {
		t.Errorf("viewer GET /viewer-data = %d, want 200", code)
	}
	if code := status(t, http.MethodGet, "/admin/users", token, nil); code != 403 {
		t.Errorf("viewer GET /admin/users = %d, want 403", code)
	}
}

func TestEditorRole(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "editor")

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
	token, subj := orySession(t)
	grantRole(t, subj, "admin")

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
	revokeRole(t, subj, "admin")
	if code := status(t, http.MethodGet, "/admin/users", token, nil); code != 403 {
		t.Errorf("after revoke GET /admin/users = %d, want 403", code)
	}
}

func TestSetUserRoleWritesKeto(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "admin")

	target := "runtime-target"
	revokeRole(t, target, "viewer")
	code := status(t, http.MethodPatch, "/admin/users/"+target+"/role", token, map[string]any{"role": "viewer"})
	if code != 200 {
		t.Fatalf("set role = %d, want 200", code)
	}
	if !ketoCheck(t, "roles", "viewer", "assignee", target) {
		t.Error("expected runtime-target to be viewer in Keto after set-role")
	}
	revokeRole(t, target, "viewer")
}

func TestPermissionThroughSubjectSet(t *testing.T) {
	// admin has users:manage through the seeded subject set
	// users:manage#perform@roles:admin#assignee.
	_, subj := orySession(t)
	grantRole(t, subj, "admin")
	if !ketoCheck(t, "users", "manage", "perform", subj) {
		t.Error("expected admin to inherit users:manage via the Keto subject set")
	}
}

func TestArbitraryRoleFromPermissions(t *testing.T) {
	// A free-form role declared in auth.role_permissions inherits its grant.
	_, subj := orySession(t)
	grantRole(t, subj, "facturacion-lectura")
	if !ketoCheck(t, "products", "read", "perform", subj) {
		t.Error("expected facturacion-lectura to inherit products:read")
	}
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

func TestRateLimitedEndpoint(t *testing.T) {
	token, _ := orySession(t)
	ok := 0
	for i := 0; i < 3; i++ {
		if code := status(t, http.MethodPost, "/rate-limited", token, map[string]any{}); code == 200 {
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
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz = %d, want 200", resp.StatusCode)
	}
}

func TestAPIKeyInvalid(t *testing.T) {
	if code := apiKeyStatus(t, http.MethodGet, "/products", "sk-does-not-exist"); code != 401 {
		t.Fatalf("invalid api key = %d, want 401", code)
	}
}

func TestAPIKeyMissing(t *testing.T) {
	if code := status(t, http.MethodGet, "/admin/dual-products", "", nil); code != 401 {
		t.Fatalf("missing credential = %d, want 401", code)
	}
}

func TestAPIKeyEditorCreatesProduct(t *testing.T) {
	// An API key is a machine subject; its role comes from Keto. Grant the
	// editor role to the key subject, then the role-gated route allows it.
	grantRole(t, "key-editor", "editor")
	body, _ := json.Marshal(map[string]any{"name": "Key Item", "price": 5.0})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "sk-editor_abc123")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("api key create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("editor api key POST /products = %d, want 201", resp.StatusCode)
	}
}

func TestPerRoleLimited(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "viewer")
	ok := 0
	for i := 0; i < 2; i++ {
		if code := status(t, http.MethodPost, "/per-role-limited", token, map[string]any{}); code == 200 {
			ok++
		}
	}
	if ok == 0 {
		t.Error("expected /per-role-limited to allow at least one request")
	}
}

func TestDualAuthSessionAndKey(t *testing.T) {
	// Session works on the dual-auth route.
	token, subj := orySession(t)
	grantRole(t, subj, "admin")
	if code := status(t, http.MethodGet, "/admin/dual-products", token, nil); code != 200 {
		t.Errorf("session GET /admin/dual-products = %d, want 200", code)
	}
	// A valid API key without the role is denied.
	if code := apiKeyStatus(t, http.MethodGet, "/admin/dual-products", "sk-editor_abc123"); code != 403 {
		t.Errorf("api key GET /admin/dual-products = %d, want 403", code)
	}
}

// TestKratosIdentityManagement validates the admin endpoints talk to Kratos:
// listing identities returns the bootstrapped identity.
func TestKratosIdentityManagement(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "admin")
	resp := do(t, http.MethodGet, "/admin/users", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list users = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte(subj)) {
		t.Errorf("expected the bootstrapped identity %s in the Kratos listing: %s", subj, body)
	}
}

// TestKetoRevocationIsImmediate validates Keto is the source of truth: after
// deleting the role tuple, a previously allowed request is denied at once.
func TestKetoRevocationIsImmediate(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "editor")
	if code := status(t, http.MethodPost, "/products", token, map[string]any{"name": "Rev", "price": 1.0}); code != 201 {
		t.Fatalf("editor POST = %d, want 201", code)
	}
	revokeRole(t, subj, "editor")
	if code := status(t, http.MethodPost, "/products", token, map[string]any{"name": "Rev2", "price": 1.0}); code != 403 {
		t.Fatalf("after revoke POST = %d, want 403", code)
	}
}

// TestTenantScopedByKratosOrg validates the CRUD tenant scope reads org_id from
// the Kratos session traits.
func TestTenantScopedByKratosOrg(t *testing.T) {
	token, _ := orySession(t)
	resp := do(t, http.MethodGet, "/tenant-products", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("tenant list = %d, want 200 (org_id comes from the Kratos session)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("org-alfa")) {
		t.Errorf("expected org-alfa scoped rows, got %s", body)
	}
}

// TestSessionTokenHeaderAccepted validates the X-Session-Token header path
// (Kratos accepts it for native clients alongside Bearer).
func TestSessionTokenHeaderAccepted(t *testing.T) {
	token, _ := orySession(t)
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/products", nil)
	req.Header.Set("X-Session-Token", token)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("session token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /products with X-Session-Token = %d, want 200", resp.StatusCode)
	}
}

// apiKeyStatus sends an API key (raw, no Bearer) and returns the status.
func apiKeyStatus(t *testing.T, method, path, key string) int {
	t.Helper()
	req, _ := http.NewRequest(method, baseURL+path, nil)
	req.Header.Set("Authorization", key)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("api key request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestGrantRoleViaKeto proves a delegated role is written to Keto and takes
// effect immediately: an editor grant unlocks an editor-gated route.
func TestGrantRoleViaKeto(t *testing.T) {
	token, subj := orySession(t)
	// admin can reach the grant endpoint
	grantRole(t, subj, "admin")
	// delegate "editor" to a synthetic subject
	code := status(t, http.MethodPost, "/auth/grants", token, map[string]any{"grantee": "delegated-user", "role": "editor"})
	if code != 200 {
		t.Fatalf("grant role = %d, want 200", code)
	}
	if !ketoCheck(t, "roles", "editor", "assignee", "delegated-user") {
		t.Error("expected delegated-user to be editor in Keto")
	}
	// revoke
	if code := status(t, http.MethodDelete, "/auth/grants/delegated-user/editor", token, map[string]any{}); code != 200 {
		t.Fatalf("revoke role = %d, want 200", code)
	}
	if ketoCheck(t, "roles", "editor", "assignee", "delegated-user") {
		t.Error("expected delegated-user to lose editor after revoke")
	}
}

// TestMyRolesFromKeto lists the caller's roles resolved through Keto.
func TestMyRolesFromKeto(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "viewer")
	resp := do(t, http.MethodGet, "/me/roles", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /me/roles = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("viewer")) {
		t.Errorf("expected viewer in /me/roles: %s", body)
	}
}

// TestTeamsOry creates and lists a team.
func TestTeamsOry(t *testing.T) {
	token, subj := orySession(t)
	grantRole(t, subj, "admin")
	if code := status(t, http.MethodPost, "/teams", token, map[string]any{"name": "Equipo Ory"}); code != 201 {
		t.Fatalf("create team = %d, want 201", code)
	}
	resp := do(t, http.MethodGet, "/teams", token, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list teams = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("Equipo Ory")) {
		t.Errorf("created team missing: %s", body)
	}
}

// ---- Hydra OAuth2/OIDC integration ----

func hydraPublicURL() string {
	if v := os.Getenv("HYDRA_PUBLIC_URL"); v != "" {
		return v
	}
	return "http://localhost:4444"
}

func hydraAdminURL() string {
	if v := os.Getenv("HYDRA_ADMIN_URL"); v != "" {
		return v
	}
	return "http://localhost:4445"
}

// TestHydraDiscovery validates the OIDC discovery document is served.
func TestHydraDiscovery(t *testing.T) {
	resp, err := httpClient.Get(hydraPublicURL() + "/.well-known/openid-configuration")
	if err != nil {
		t.Skipf("hydra not available: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("discovery = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("token_endpoint")) {
		t.Errorf("discovery missing token_endpoint: %s", body)
	}
}

// TestHydraClientCredentials validates a real client_credentials token flow.
func TestHydraClientCredentials(t *testing.T) {
	// ensure the client exists (idempotent)
	client := map[string]any{
		"client_id": "test-client", "client_secret": "test-client-secret",
		"grant_types": []string{"client_credentials"}, "response_types": []string{"token"},
		"scope": "openid", "token_endpoint_auth_method": "client_secret_post",
	}
	b, _ := json.Marshal(client)
	req, _ := http.NewRequest(http.MethodPost, hydraAdminURL()+"/admin/clients", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := httpClient.Do(req); err == nil {
		resp.Body.Close()
	} else {
		t.Skipf("hydra admin not available: %v", err)
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", "test-client")
	form.Set("client_secret", "test-client-secret")
	form.Set("scope", "openid")
	resp, err := httpClient.PostForm(hydraPublicURL()+"/oauth2/token", form)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("token = %d, want 200 (%s)", resp.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		t.Fatalf("token response: %s", body)
	}
	t.Logf("hydra client_credentials token issued (type=%s)", out.TokenType)
}

// TestHydraJWKS validates the JWKS endpoint serves keys.
func TestHydraJWKS(t *testing.T) {
	resp, err := httpClient.Get(hydraPublicURL() + "/.well-known/jwks.json")
	if err != nil {
		t.Skipf("hydra not available: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("jwks = %d, want 200", resp.StatusCode)
	}
}

// ---- Kratos native flows (TOTP, WebAuthn, recovery, settings) ----

// TestKratosSettingsExposesTOTP starts a settings flow with the caller's
// session and checks TOTP is offered (TOTP is enabled and configured).
func TestKratosSettingsExposesTOTP(t *testing.T) {
	token, _ := orySession(t)
	req, _ := http.NewRequest(http.MethodGet, kratosPublicURL()+"/self-service/settings/api", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("settings flow: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("settings flow = %d, want 200 (%s)", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte("totp")) {
		t.Errorf("settings flow does not offer totp: %s", body)
	}
	if !bytes.Contains(body, []byte("totp_secret_key")) {
		t.Errorf("settings flow missing TOTP enrollment secret: %s", body)
	}
}
// TestKratosRecoveryFlow validates the native recovery flow is enabled.
func TestKratosRecoveryFlow(t *testing.T) {
	resp, err := httpClient.Get(kratosPublicURL() + "/self-service/recovery/api")
	if err != nil {
		t.Fatalf("recovery api: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("recovery api = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("recovery")) {
		t.Errorf("recovery flow response unexpected: %s", body)
	}
}

// TestKratosVerificationFlow validates the native verification flow is enabled.
func TestKratosVerificationFlow(t *testing.T) {
	resp, err := httpClient.Get(kratosPublicURL() + "/self-service/verification/api")
	if err != nil {
		t.Fatalf("verification api: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("verification api = %d, want 200", resp.StatusCode)
	}
}

// TestKratosTOTPEnrolledLogin ensures a session obtained with password works on
// a TOTP-enabled identity (TOTP is optional until enrolled, so login succeeds
// at aal1 and the session is valid).
func TestKratosSessionAAL(t *testing.T) {
	token, _ := orySession(t)
	req, _ := http.NewRequest(http.MethodGet, kratosPublicURL()+"/sessions/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("whoami = %d, want 200 (%s)", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte("authenticator_assurance_level")) {
		t.Errorf("session missing AAL: %s", body)
	}
}
