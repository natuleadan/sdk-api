package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/runtime/authtest"
)

// orySubjects implements authtest.SubjectProvider for the ory-stack: identity
// is a Kratos session and authorization is Keto. The stack supports org-scoped
// identities (the org_id trait) and cookie transport (ory_session), so it also
// implements TenantSubjectProvider, OrgSubjectProvider and CookieSubjectProvider.
type orySubjects struct {
	token    string
	identity string
}

var oryKnownRoles = []string{"viewer", "editor", "admin", "facturacion-lectura"}

func (p *orySubjects) ensure(t *testing.T) {
	t.Helper()
	if p.token != "" {
		return
	}
	tok, id := orySession(t)
	p.token = tok
	p.identity = id
}

func (p *orySubjects) setRole(t *testing.T, role string) {
	t.Helper()
	p.ensure(t)
	for _, r := range oryKnownRoles {
		revokeRole(t, p.identity, r)
	}
	if role != "" {
		grantRole(t, p.identity, role)
	}
}

func (p *orySubjects) Principal(t *testing.T, roles ...string) authtest.Subject {
	t.Helper()
	role := "viewer"
	if len(roles) > 0 {
		role = roles[0]
	}
	p.setRole(t, role)
	return authtest.Subject{Token: p.token, ID: p.identity}
}

func (p *orySubjects) Revoke(t *testing.T, _ authtest.Subject, _ string) {
	t.Helper()
	p.setRole(t, "")
}

// PrincipalWithTenant returns a session whose identity carries the org trait.
// The seeded identity lives in org-alfa; other orgs get a throwaway identity
// created through the Kratos admin API (and removed on cleanup).
func (p *orySubjects) PrincipalWithTenant(t *testing.T, org string, roles ...string) authtest.Subject {
	t.Helper()
	if org == "org-alfa" {
		return p.Principal(t, roles...)
	}
	tok, id := oryIdentityInOrg(t, org)
	role := "viewer"
	if len(roles) > 0 {
		role = roles[0]
	}
	if role != "" {
		grantRole(t, id, role)
	}
	return authtest.Subject{Token: tok, ID: id}
}

// PrincipalInOrg is the multi-org variant used for cross-tenant isolation.
func (p *orySubjects) PrincipalInOrg(t *testing.T, org string, roles ...string) authtest.Subject {
	t.Helper()
	return p.PrincipalWithTenant(t, org, roles...)
}

// Cookie returns the Kratos session cookie the app's session middleware reads.
func (p *orySubjects) Cookie(t *testing.T) (string, string) {
	t.Helper()
	p.ensure(t)
	return "ory_session", p.token
}

func (p *orySubjects) APIKey(t *testing.T, name string) (string, string) {
	t.Helper()
	switch name {
	case "reader":
		return "sk-reader_abc123", "viewer"
	case "editor":
		return "sk-editor_abc123", "editor"
	default:
		return "", ""
	}
}

// oryIdentityInOrg creates (idempotently) a Kratos identity with the given org
// trait, logs in, and schedules the identity's deletion. This is the stack's
// own admin API, not a simulation: a real Kratos identity backs the session.
func oryIdentityInOrg(t *testing.T, org string) (token, identityID string) {
	t.Helper()
	email := fmt.Sprintf("contract-%s@example.com", org)
	password := "Contract123!"
	admin := kratosAdminURL()

	if id := kratosFindIdentity(t, admin, email); id == "" {
		body := map[string]any{
			"schema_id": "default",
			"traits":    map[string]any{"email": email, "org_id": org},
			"credentials": map[string]any{
				"password": map[string]any{"config": map[string]any{"password": password}},
			},
			"state": "active",
		}
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, admin+"/admin/identities", bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("create identity request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("create identity: %v", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("create identity (%d): %s", resp.StatusCode, respBody)
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &created); err != nil || created.ID == "" {
			t.Fatalf("create identity decode: %s", respBody)
		}
		t.Cleanup(func() { kratosDeleteIdentity(admin, created.ID) })
	}
	tok, id := kratosLoginAs(t, email, password)
	return tok, id
}

func kratosAdminURL() string {
	if v := os.Getenv("KRATOS_ADMIN_URL"); v != "" {
		return v
	}
	return "http://localhost:14434"
}

func kratosFindIdentity(t *testing.T, admin, email string) string {
	t.Helper()
	resp, err := httpClient.Get(admin + "/admin/identities?page_size=250")
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var list []struct {
		ID     string `json:"id"`
		Traits struct {
			Email string `json:"email"`
		} `json:"traits"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("list identities decode: %v", err)
	}
	for _, id := range list {
		if id.Traits.Email == email {
			return id.ID
		}
	}
	return ""
}

func kratosDeleteIdentity(admin, id string) {
	req, err := http.NewRequest(http.MethodDelete, admin+"/admin/identities/"+id, nil)
	if err != nil {
		return
	}
	if resp, err := httpClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

func kratosLoginAs(t *testing.T, email, password string) (token, identityID string) {
	t.Helper()
	flowResp, err := httpClient.Get(kratosPublicURL() + "/self-service/login/api")
	if err != nil {
		t.Fatalf("login flow: %v", err)
	}
	defer func() { _ = flowResp.Body.Close() }()
	var flow struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(flowResp.Body).Decode(&flow); err != nil || flow.ID == "" {
		t.Fatalf("login flow decode: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{
		"method":     "password",
		"identifier": email,
		"password":   password,
	})
	req, err := http.NewRequest(http.MethodPost, kratosPublicURL()+"/self-service/login?flow="+flow.ID, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("login submit request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("login submit: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
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
		t.Fatalf("login as %s (%d): %s", email, resp.StatusCode, body)
	}
	return result.SessionToken, result.Session.Identity.ID
}

// TestAuthContract runs the shared auth contract for driver "ory".
func TestAuthContract(t *testing.T) {
	if os.Getenv("DOCKER_TEST") != "1" {
		t.Skip("Docker-only test")
	}
	authtest.Run(t, authtest.Config{BaseURL: baseURL}, &orySubjects{})
}
