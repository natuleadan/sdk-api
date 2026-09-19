package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

var rolesHTTP = &http.Client{Timeout: 10 * time.Second}

// rolesReq performs an authenticated JSON request and returns status + body.
func rolesReq(t *testing.T, method, path, token string, payload any) (int, string) {
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
	resp, err := rolesHTTP.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

// resetRole clears any assignment of role for userID (admin token required).
func resetRole(t *testing.T, adminTok, userID, role string) {
	t.Helper()
	_, _ = rolesReq(t, http.MethodDelete, "/admin/users/"+userID+"/roles/"+role, adminTok, map[string]any{})
}

const demoRole = "facturacion-lectura"

func TestDynamicRole_ArbitraryGrantAndRevoke(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", demoRole)
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", demoRole) })

	if code, body := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 403 {
		t.Fatalf("before grant = %d, want 403 (%s)", code, body)
	}
	if code, body := rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok,
		map[string]any{"role": demoRole}); code != 200 {
		t.Fatalf("grant = %d, want 200 (%s)", code, body)
	}
	if code, body := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 200 {
		t.Fatalf("after grant = %d, want 200 (%s)", code, body)
	}
	if code, _ := rolesReq(t, http.MethodDelete, "/admin/users/user-roles/roles/"+demoRole, adminTok, map[string]any{}); code != 200 {
		t.Fatalf("revoke = %d, want 200", code)
	}
	if code, _ := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 403 {
		t.Fatalf("after revoke = %d, want 403", code)
	}
}

func TestDynamicRole_Expiry(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", demoRole)
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", demoRole) })

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok,
		map[string]any{"role": demoRole, "expires_at": past})
	if code, _ := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 403 {
		t.Errorf("expired role = %d, want 403", code)
	}

	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok,
		map[string]any{"role": demoRole, "expires_at": future})
	if code, _ := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 200 {
		t.Errorf("valid role = %d, want 200", code)
	}
}

func TestDynamicRole_PendingAcceptance(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", demoRole)
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", demoRole) })

	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok,
		map[string]any{"role": demoRole, "status": "pending"})
	if code, _ := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 403 {
		t.Errorf("pending role = %d, want 403", code)
	}
	if code, body := rolesReq(t, http.MethodPost, "/me/roles/"+demoRole+"/accept", editorTok, map[string]any{}); code != 200 {
		t.Fatalf("accept = %d, want 200 (%s)", code, body)
	}
	if code, _ := rolesReq(t, http.MethodGet, "/facturacion", editorTok, nil); code != 200 {
		t.Errorf("after accept = %d, want 200", code)
	}
}

func TestDynamicRole_MultipleRoles(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", "viewer")
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", "viewer") })

	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok, map[string]any{"role": "viewer"})
	if code, _ := rolesReq(t, http.MethodGet, "/viewer-data", editorTok, nil); code != 200 {
		t.Errorf("viewer via assignment = %d, want 200", code)
	}
	// The editor role still applies too.
	if code, _ := rolesReq(t, http.MethodPost, "/products", editorTok, map[string]any{"name": "Multi Role", "price": 1.0}); code != 201 {
		t.Errorf("editor create = %d, want 201", code)
	}
}

func TestDynamicRole_PermissionFromRole(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", demoRole)
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", demoRole) })

	if code, _ := rolesReq(t, http.MethodGet, "/admin/manage-users", editorTok, nil); code != 403 {
		t.Fatalf("before grant = %d, want 403", code)
	}
	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok, map[string]any{"role": demoRole})
	if code, body := rolesReq(t, http.MethodGet, "/admin/manage-users", editorTok, nil); code != 200 {
		t.Errorf("permission via role = %d, want 200 (%s)", code, body)
	}
}

func TestDynamicRole_MyRoles(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	resetRole(t, adminTok, "user-roles", demoRole)
	t.Cleanup(func() { resetRole(t, adminTok, "user-roles", demoRole) })

	rolesReq(t, http.MethodPost, "/admin/users/user-roles/roles", adminTok, map[string]any{"role": demoRole})

	code, body := rolesReq(t, http.MethodGet, "/me/roles", editorTok, nil)
	if code != 200 {
		t.Fatalf("GET /me/roles = %d, want 200 (%s)", code, body)
	}
	var out struct {
		Data []struct {
			Role      string `json:"role"`
			Status    string `json:"status"`
			Effective bool   `json:"effective"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, r := range out.Data {
		if r.Role == demoRole {
			found = true
			if !r.Effective || r.Status != "active" {
				t.Errorf("role %s: status=%s effective=%v, want active/true", r.Role, r.Status, r.Effective)
			}
		}
	}
	if !found {
		t.Errorf("role %s not listed in /me/roles", demoRole)
	}
}

func loginTry(user, pass string) (string, int) {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	resp, err := rolesHTTP.Post(baseURL+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", resp.StatusCode
	}
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(b, &out)
	return out.Token, resp.StatusCode
}

// loginRetry tolerates the /login per-user rate limit during tests.
func loginRetry(t *testing.T, user, pass string) string {
	t.Helper()
	for i := 0; i < 20; i++ {
		tok, code := loginTry(user, pass)
		if code == 200 && tok != "" {
			return tok
		}
		if code == 429 {
			time.Sleep(600 * time.Millisecond)
			continue
		}
		t.Fatalf("login %s: status %d", user, code)
	}
	t.Fatalf("login %s: still rate-limited", user)
	return ""
}
