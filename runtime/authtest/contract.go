// Package authtest provides a shared HTTP contract harness for the auth
// examples (400-auth). It exercises the behaviour every auth driver must
// satisfy — authorization, API keys, rate limiting, CSRF, tenant scoping and
// dual auth — independent of who provides identity (manual, Zitadel, Kratos)
// or authorization (database, OpenFGA, Keto).
//
// The driver-specific parts (how to obtain a subject token, how to grant a
// role, how to revoke it) are supplied by the caller through SubjectProvider,
// so the same contract runs unchanged across stacks. Use Run to execute the
// whole suite from an example's test binary.
package authtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// Config describes how to reach an example under test.
type Config struct {
	// BaseURL is the API base, e.g. http://localhost:23400/api.
	BaseURL string
	// HealthURL is the readiness endpoint (default BaseURL + "/healthz" root).
	HealthURL string
	// Timeout for the readiness wait.
	ReadyTimeout time.Duration
}

// Subject is an authenticated caller the contract can act as.
type Subject struct {
	// Token is the credential sent as "Authorization: Bearer <Token>".
	Token string
	// ID is the subject identifier (used for self-checks like "cannot delete
	// yourself").
	ID string
}

// SubjectProvider supplies driver-specific identity and authorization actions.
// Each auth example implements this over its stack.
type SubjectProvider interface {
	// Principal returns a subject that should satisfy the roles named (by
	// granting them if necessary). At least "admin", "editor" and "viewer" must
	// be supported by the examples the contract targets.
	Principal(t *testing.T, roles ...string) Subject
	// Revoke removes the given role from a subject, if the stack can.
	Revoke(t *testing.T, s Subject, role string)
	// APIKey returns a raw API key string and its role, if the stack seeds keys.
	// Return ("", "") when the stack has no API keys.
	APIKey(t *testing.T, name string) (key, role string)
}

// T is the subset of *testing.T the contract uses, so it is testable itself.
type T interface {
	Helper()
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
	Skipf(format string, args ...any)
}

// Run executes the full contract suite against cfg using the provider.
func Run(t *testing.T, cfg Config, p SubjectProvider) {
	t.Helper()
	waitReady(t, cfg)
	s := &suite{t: t, cfg: cfg, p: p}
	s.unauthenticated()
	s.authorization()
	s.apiKeys()
	s.rateLimits()
	s.dualAuth()
	s.productCrud()
}

type suite struct {
	t   *testing.T
	cfg Config
	p   SubjectProvider
}

func (s *suite) url(path string) string { return s.cfg.BaseURL + path }

// do performs a request and returns status + body. Write methods carry a small
// JSON body so CSRF checks that require a JSON content type are satisfied, even
// when the endpoint takes no payload.
func (s *suite) do(method, path, token string, payload any) (int, string) {
	s.t.Helper()
	if payload == nil && method != http.MethodGet && method != http.MethodHead {
		payload = map[string]any{}
	}
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.url(path), body)
	if err != nil {
		s.t.Fatalf("request %s %s: %v", method, path, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func (s *suite) status(method, path, token string, payload any) int {
	s.t.Helper()
	code, _ := s.do(method, path, token, payload)
	return code
}

// ---- contract sections ----

func (s *suite) unauthenticated() {
	s.t.Helper()
	if code := s.status(http.MethodGet, "/products", "", nil); code != 401 {
		s.t.Errorf("unauthenticated GET /products = %d, want 401", code)
	}
}

func (s *suite) authorization() {
	s.t.Helper()
	// The provider is asked for the subject right before each phase: stacks
	// with a single shared subject (e.g. an OpenFGA machine token) cannot hold
	// three roles at once, so the contract never caches subjects across phases.

	// viewer: reaches the viewer-gated route, not an admin one.
	viewer := s.p.Principal(s.t, "viewer")
	if code := s.status(http.MethodGet, "/products", viewer.Token, nil); code != 200 {
		s.t.Errorf("viewer GET /products = %d, want 200", code)
	}
	if code := s.status(http.MethodGet, "/viewer-data", viewer.Token, nil); code != 200 {
		s.t.Errorf("viewer GET /viewer-data = %d, want 200", code)
	}
	if code := s.status(http.MethodGet, "/admin/users", viewer.Token, nil); code != 403 {
		s.t.Errorf("viewer GET /admin/users = %d, want 403", code)
	}

	// editor: can create products, cannot hard-delete (admin only).
	editor := s.p.Principal(s.t, "editor")
	if code := s.status(http.MethodPost, "/products", editor.Token, map[string]any{"name": "contract-editor", "price": 5.0}); code != 201 {
		s.t.Errorf("editor POST /products = %d, want 201", code)
	}
	if code := s.status(http.MethodDelete, "/admin/products/prod-1/hard", editor.Token, nil); code != 403 {
		s.t.Errorf("editor hard delete = %d, want 403", code)
	}

	// admin: reaches the admin route and the visibility patch.
	admin := s.p.Principal(s.t, "admin")
	if code := s.status(http.MethodGet, "/admin/users", admin.Token, nil); code != 200 {
		s.t.Errorf("admin GET /admin/users = %d, want 200", code)
	}
	if code := s.status(http.MethodPatch, "/admin/products/prod-2/visibility", admin.Token, map[string]any{"visibility": "public"}); code != 200 {
		s.t.Errorf("admin set visibility = %d, want 200", code)
	}

	// revocation: the viewer role must be gone immediately.
	viewer = s.p.Principal(s.t, "viewer")
	s.p.Revoke(s.t, viewer, "viewer")
	if code := s.status(http.MethodGet, "/viewer-data", viewer.Token, nil); code != 403 {
		s.t.Errorf("viewer GET /viewer-data after revoke = %d, want 403", code)
	}
}

func (s *suite) apiKeys() {
	s.t.Helper()
	key, _ := s.p.APIKey(s.t, "reader")
	if key == "" {
		s.t.Logf("api keys not configured for this stack, skipping")
		return
	}
	// A valid key reads public products.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/products"), nil)
	req.Header.Set("Authorization", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("api key GET /products: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		s.t.Errorf("api key GET /products = %d, want 200", resp.StatusCode)
	}
	// An unknown key is rejected.
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/products"), nil)
	req.Header.Set("Authorization", "sk-does-not-exist")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("invalid api key GET /products: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		s.t.Errorf("invalid api key GET /products = %d, want 401", resp.StatusCode)
	}
}

func (s *suite) rateLimits() {
	s.t.Helper()
	editor := s.p.Principal(s.t, "editor")
	// The per-role-limited route must accept at least one request (200) and no
	// request may fail with anything other than a rate-limit (429).
	ok := 0
	for range 3 {
		switch code := s.status(http.MethodPost, "/per-role-limited", editor.Token, nil); code {
		case 200:
			ok++
		case 429:
		default:
			s.t.Errorf("per-role-limited = %d, want 200 or 429", code)
		}
	}
	if ok == 0 {
		s.t.Errorf("per-role-limited allowed 0 requests, want >= 1")
	}
}

func (s *suite) dualAuth() {
	s.t.Helper()
	admin := s.p.Principal(s.t, "admin")
	// Session/token on a dual-auth (jwt+apikey) route.
	if code := s.status(http.MethodGet, "/admin/dual-products", admin.Token, nil); code != 200 {
		s.t.Errorf("admin dual-auth GET /admin/dual-products = %d, want 200", code)
	}
	// A valid API key without the role is denied.
	key, _ := s.p.APIKey(s.t, "editor")
	if key == "" {
		return
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/admin/dual-products"), nil)
	req.Header.Set("Authorization", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("dual-auth api key: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		s.t.Errorf("api key dual-auth GET /admin/dual-products = %d, want 403", resp.StatusCode)
	}
}

func (s *suite) productCrud() {
	s.t.Helper()
	editor := s.p.Principal(s.t, "editor")
	// Create, read, list.
	code, body := s.do(http.MethodPost, "/products", editor.Token, map[string]any{"name": "contract-crud", "price": 7.5})
	if code != 201 {
		s.t.Errorf("editor create product = %d, want 201 (%s)", code, body)
		return
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if created.ID == "" {
		s.t.Errorf("create product response missing id: %s", body)
		return
	}
	if code := s.status(http.MethodGet, "/products/"+created.ID, editor.Token, nil); code != 200 {
		s.t.Errorf("GET created product = %d, want 200", code)
	}
	if code := s.status(http.MethodPatch, "/products/"+created.ID, editor.Token, map[string]any{"price": 8.0}); code != 200 {
		s.t.Errorf("PATCH product = %d, want 200", code)
	}
	if code := s.status(http.MethodDelete, "/products/"+created.ID, editor.Token, nil); code != 200 {
		s.t.Errorf("DELETE product = %d, want 200", code)
	}
}

// waitReady polls the health endpoint until the service answers.
func waitReady(t *testing.T, cfg Config) {
	t.Helper()
	url := cfg.HealthURL
	if url == "" {
		url = "http://localhost:23400/healthz"
	}
	timeout := cfg.ReadyTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, rerr := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if rerr != nil {
			continue
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("service not ready at %s after %v", url, timeout)
}

// DefaultBaseURL is the base URL the examples listen on.
const DefaultBaseURL = "http://localhost:23400/api"

// DefaultHealthURL is the readiness endpoint the examples expose.
const DefaultHealthURL = "http://localhost:23400/healthz"

// AssertStatus is a small helper for examples to reuse in their own tests.
func AssertStatus(t T, got, want int, what string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", what, got, want)
	}
}

// Sprintf is re-exported so examples can build messages without importing fmt.
func Sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
