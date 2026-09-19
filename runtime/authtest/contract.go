// Package authtest provides a shared HTTP contract harness for the auth
// examples (400-auth). It exercises the behaviour every auth driver must
// satisfy — authorization, API keys, CSRF, tenant scoping, rate limiting,
// dual auth, security headers, CRUD/audit, admin self-protection and cookie
// transport — independent of who provides identity (manual, Zitadel, Kratos)
// or authorization (database, OpenFGA, Keto).
//
// The driver-specific parts (how to obtain a subject token, how to grant a
// role, how to revoke it) are supplied by the caller through SubjectProvider,
// so the same contract runs unchanged across stacks. Optional capabilities
// (cross-org subjects, cookie transport) are negotiated through the
// OrgSubjectProvider and CookieSubjectProvider interfaces: stacks whose IdP
// cannot supply them skip those checks with an explicit log line instead of
// simulating an identity they do not have. Use Run to execute the whole
// suite from an example's test binary.
package authtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// OrgSubjectProvider is an optional extension for stacks that can place
// subjects in different organizations (tenant scoping). Stacks whose IdP
// issues a single-org identity (a Zitadel machine key, one Kratos session)
// do not implement it: the cross-tenant checks are skipped with a log line.
type OrgSubjectProvider interface {
	SubjectProvider
	// PrincipalInOrg returns a subject authenticated inside the named org.
	PrincipalInOrg(t *testing.T, org string, roles ...string) Subject
}

// TenantSubjectProvider is an optional extension for stacks whose identity
// carries an organization claim (so the tenant_scope CRUD entry can resolve
// it). Opaque-token stacks whose introspection returns no org — a Zitadel
// machine token — do not implement it and skip tenant checks with a log line.
type TenantSubjectProvider interface {
	SubjectProvider
	// PrincipalWithTenant returns a subject whose token carries tenant as its
	// org claim.
	PrincipalWithTenant(t *testing.T, tenant string, roles ...string) Subject
}

// CookieSubjectProvider is an optional extension for stacks whose login flow
// issues a session cookie usable against cookie-transport endpoints. Stacks
// without one skip the cookie checks with a log line.
type CookieSubjectProvider interface {
	SubjectProvider
	// Cookie returns the session cookie name and value for /cookie/profile.
	Cookie(t *testing.T) (name, value string)
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
	s.csrf()
	s.tenantScoping()
	s.rateLimits()
	s.dualAuth()
	s.security()
	s.productCrud()
	s.adminSelfProtection()
	s.cookies()
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

// doRaw performs a request with explicit headers and no implicit JSON body,
// for checks (CSRF) that depend on the exact content type sent.
func (s *suite) doRaw(method, path, token string, headers map[string]string, raw []byte) (int, http.Header) {
	s.t.Helper()
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.url(path), body)
	if err != nil {
		s.t.Fatalf("request %s %s: %v", method, path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header
}

// apiKeyStatus performs a GET /products with a raw API key credential.
func (s *suite) apiKeyStatus(key string) int {
	s.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/products"), nil)
	if err != nil {
		s.t.Fatalf("api key request: %v", err)
	}
	req.Header.Set("Authorization", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("api key GET /products: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

// ---- contract sections ----

func (s *suite) unauthenticated() {
	s.t.Helper()
	if code := s.status(http.MethodGet, "/products", "", nil); code != 401 {
		s.t.Errorf("unauthenticated GET /products = %d, want 401", code)
	}
	if code := s.status(http.MethodGet, "/tenant-products", "", nil); code != 401 {
		s.t.Errorf("unauthenticated GET /tenant-products = %d, want 401", code)
	}
	if code := s.status(http.MethodGet, "/cookie/profile", "", nil); code != 401 {
		s.t.Errorf("unauthenticated GET /cookie/profile = %d, want 401", code)
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
	readerKey, _ := s.p.APIKey(s.t, "reader")
	if readerKey == "" {
		s.t.Logf("api keys not configured for this stack, skipping")
		return
	}
	// A valid reader key lists products (dual-auth route).
	if code := s.apiKeyStatus(readerKey); code != 200 {
		s.t.Errorf("reader api key GET /products = %d, want 200", code)
	}
	// An unknown key is rejected.
	if code := s.apiKeyStatus("sk-does-not-exist"); code != 401 {
		s.t.Errorf("invalid api key GET /products = %d, want 401", code)
	}
	// A key with the wrong prefix shape is rejected.
	if code := s.apiKeyStatus("bad-key"); code != 401 {
		s.t.Errorf("wrong-prefix api key GET /products = %d, want 401", code)
	}
	// A reader key cannot create products.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url("/products"), strings.NewReader(`{"name":"contract-key-create"}`))
	if err != nil {
		s.t.Fatalf("reader key POST setup: %v", err)
	}
	req.Header.Set("Authorization", readerKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("reader key POST /products: %v", err)
	}
	func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 403 {
		s.t.Errorf("reader api key POST /products = %d, want 403", resp.StatusCode)
	}
	// An editor key cannot hard-delete (admin only).
	editorKey, _ := s.p.APIKey(s.t, "editor")
	if editorKey == "" {
		return
	}
	req, err = http.NewRequestWithContext(context.Background(), http.MethodDelete, s.url("/admin/products/prod-1/hard"), nil)
	if err != nil {
		s.t.Fatalf("editor key DELETE setup: %v", err)
	}
	req.Header.Set("Authorization", editorKey)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("editor key hard delete: %v", err)
	}
	func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 403 {
		s.t.Errorf("editor api key hard delete = %d, want 403", resp.StatusCode)
	}
}

func (s *suite) csrf() {
	s.t.Helper()
	admin := s.p.Principal(s.t, "admin")
	// A POST without content type and without a CSRF token is rejected.
	if code, _ := s.doRaw(http.MethodPost, "/rate-limited", admin.Token, nil, nil); code != 403 {
		s.t.Errorf("POST without content-type = %d, want 403 (CSRF)", code)
	}
	// A JSON POST without a CSRF header passes (json_check).
	if code, _ := s.doRaw(http.MethodPost, "/rate-limited", admin.Token,
		map[string]string{"Content-Type": "application/json"}, []byte(`{}`)); code == 403 {
		s.t.Errorf("JSON POST without CSRF header = 403, want pass-through (200 or 429)")
	}
	// The per-entry skip stays open without any CSRF material.
	if code, _ := s.doRaw(http.MethodPost, "/no-csrf", admin.Token, nil, nil); code == 403 {
		s.t.Errorf("POST /no-csrf = 403, want per-entry CSRF skip")
	}
	// Authenticated GETs issue the CSRF cookie.
	if _, hdr := s.doRaw(http.MethodGet, "/products", admin.Token, nil, nil); !hasCSRFCookie(hdr) {
		s.t.Errorf("GET /products did not set a csrf_token cookie")
	}
}

func hasCSRFCookie(hdr http.Header) bool {
	for _, c := range hdr.Values("Set-Cookie") {
		if strings.Contains(c, "csrf_token=") {
			return true
		}
	}
	return false
}

func (s *suite) tenantScoping() {
	s.t.Helper()
	// Tenant scoping needs an identity that carries an organization claim.
	// Stacks with only an opaque token (no org) skip the whole section.
	tp, ok := s.p.(TenantSubjectProvider)
	if !ok {
		s.t.Logf("identity carries no org claim, skipping tenant checks")
		return
	}
	const org = "org-alfa"
	admin := tp.PrincipalWithTenant(s.t, org, "admin")
	if !s.tenantListScoped(admin, org) {
		return
	}
	s.tenantCreateInjects(admin, org)
	// Cross-tenant isolation needs two orgs; only stacks that can supply them
	// run these checks.
	op, ok := s.p.(OrgSubjectProvider)
	if !ok {
		s.t.Logf("stack has no multi-org subjects, skipping cross-tenant checks")
		return
	}
	s.tenantCrossIsolation(admin, op)
}

// tenantListScoped verifies the list only contains the subject's org. It
// returns false when the list cannot be checked (empty or failed).
func (s *suite) tenantListScoped(admin Subject, org string) bool {
	s.t.Helper()
	code, body := s.do(http.MethodGet, "/tenant-products", admin.Token, nil)
	if code != 200 {
		s.t.Errorf("admin GET /tenant-products = %d, want 200 (%s)", code, body)
		return false
	}
	tenants := tenantIDs(body)
	if len(tenants) == 0 {
		s.t.Logf("tenant list empty, skipping tenant self-consistency checks")
		return false
	}
	first := tenants[0]
	for _, tn := range tenants[1:] {
		if tn != first {
			s.t.Errorf("tenant list mixes tenants %q and %q, want scoped", first, tn)
			break
		}
	}
	if first != org {
		s.t.Errorf("listed tenant = %q, want the subject org %q", first, org)
	}
	return true
}

// tenantCreateInjects verifies a forged tenant_id in the body is overridden by
// the server-side value from the token.
func (s *suite) tenantCreateInjects(admin Subject, org string) {
	s.t.Helper()
	c, b := s.do(http.MethodPost, "/tenant-products", admin.Token,
		map[string]any{"name": "contract-tenant", "price": 3.0, "tenant_id": "org-evil"})
	if c != 201 {
		s.t.Errorf("admin POST /tenant-products = %d, want 201 (%s)", c, b)
		return
	}
	if got := tenantIDOf(b); got == "org-evil" {
		s.t.Errorf("forged tenant_id accepted (%q), want server injection", got)
	} else if got != org {
		s.t.Errorf("created tenant_id = %q, want subject org %q", got, org)
	}
}

// tenantCrossIsolation verifies one org cannot see or fetch another org's rows.
func (s *suite) tenantCrossIsolation(admin Subject, op OrgSubjectProvider) {
	s.t.Helper()
	orgB := op.PrincipalInOrg(s.t, "org-beta", "viewer")
	c, b := s.do(http.MethodPost, "/tenant-products", admin.Token,
		map[string]any{"name": "contract-cross", "price": 1.0})
	if c != 201 {
		s.t.Errorf("org-alfa create = %d, want 201 (%s)", c, b)
		return
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(b), &created)
	if created.ID == "" {
		s.t.Errorf("cross-tenant create response missing id: %s", b)
		return
	}
	// B must not see A's item in its list.
	if c, b := s.do(http.MethodGet, "/tenant-products", orgB.Token, nil); c != 200 {
		s.t.Errorf("org-beta list = %d, want 200 (%s)", c, b)
	} else if strings.Contains(b, created.ID) {
		s.t.Errorf("org-beta list leaks org-alfa item %s", created.ID)
	}
	// B must not fetch A's item directly.
	if code := s.status(http.MethodGet, "/tenant-products/"+created.ID, orgB.Token, nil); code != 404 {
		s.t.Errorf("org-beta GET org-alfa item = %d, want 404", code)
	}
}

func tenantIDs(listBody string) []string {
	var list struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(listBody), &list); err != nil {
		return nil
	}
	var out []string
	for _, row := range list.Data {
		if tn, _ := row["tenant_id"].(string); tn != "" {
			out = append(out, tn)
		}
	}
	return out
}

func tenantIDOf(docBody string) string {
	var doc map[string]any
	if err := json.Unmarshal([]byte(docBody), &doc); err != nil {
		return ""
	}
	tn, _ := doc["tenant_id"].(string)
	return tn
}

// allowLenient records the outcome of hammering a rate-limited route: only
// 200 (served) and 429 (limited) are acceptable, and at least one request
// must be served. A missing 429 is logged, not failed: with prefork each
// process owns an in-memory bucket, so the limit may not trigger without a
// shared (redis) driver.
func (s *suite) allowLenient(what, token string) {
	s.t.Helper()
	ok := 0
	for range 10 {
		switch code := s.status(http.MethodPost, what, token, nil); code {
		case 200:
			ok++
		case 429:
		default:
			s.t.Errorf("POST %s = %d, want 200 or 429", what, code)
			return
		}
	}
	if ok == 0 {
		s.t.Errorf("POST %s allowed 0 requests, want >= 1", what)
	}
}

func (s *suite) rateLimits() {
	s.t.Helper()
	editor := s.p.Principal(s.t, "editor")
	s.allowLenient("/per-role-limited", editor.Token)
	s.allowLenient("/per-user-limited", editor.Token)
	s.allowLenient("/max-func-limited", editor.Token)
	// Per-key limits ride on an API key credential.
	if key, _ := s.p.APIKey(s.t, "reader"); key != "" {
		ok := 0
		for range 10 {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url("/per-key-limited"), strings.NewReader(`{}`))
			if err != nil {
				s.t.Fatalf("per-key setup: %v", err)
			}
			req.Header.Set("Authorization", key)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				s.t.Fatalf("per-key POST: %v", err)
			}
			func() { _ = resp.Body.Close() }()
			switch resp.StatusCode {
			case 200:
				ok++
			case 429:
			default:
				s.t.Errorf("POST /per-key-limited = %d, want 200 or 429", resp.StatusCode)
				return
			}
		}
		if ok == 0 {
			s.t.Errorf("POST /per-key-limited allowed 0 requests, want >= 1")
		}
	}
}

func (s *suite) dualAuth() {
	s.t.Helper()
	admin := s.p.Principal(s.t, "admin")
	// Session/token on a dual-auth (session+apikey) route.
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
	// A reader key reaches the shared products route (both modes serve it).
	readerKey, _ := s.p.APIKey(s.t, "reader")
	if readerKey == "" {
		return
	}
	if code := s.apiKeyStatus(readerKey); code != 200 {
		s.t.Errorf("reader api key GET /products = %d, want 200", code)
	}
}

func (s *suite) security() {
	s.t.Helper()
	// Security headers are asserted on an authenticated API response: the
	// plain GET /healthz answers from a fast path that skips the middleware
	// chain, so it is not representative of protected traffic.
	admin := s.p.Principal(s.t, "admin")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/products"), nil)
	if err != nil {
		s.t.Fatalf("security headers setup: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("GET /products for headers: %v", err)
	}
	func() { _ = resp.Body.Close() }()
	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := resp.Header.Get(name); got != want {
			s.t.Errorf("api header %s = %q, want %q", name, got, want)
		}
	}
	// CORS preflight must not blow up; the allow-origin value is informational.
	health := s.cfg.HealthURL
	if health == "" {
		health = DefaultHealthURL
	}
	pre, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, health, nil)
	if err != nil {
		s.t.Fatalf("CORS preflight setup: %v", err)
	}
	pre.Header.Set("Origin", "http://example.com")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	presp, err := http.DefaultClient.Do(pre)
	if err != nil {
		s.t.Fatalf("CORS preflight: %v", err)
	}
	func() { _ = presp.Body.Close() }()
	if presp.StatusCode >= 500 {
		s.t.Errorf("CORS preflight = %d, want < 500", presp.StatusCode)
	}
	s.t.Logf("CORS Access-Control-Allow-Origin: %q", presp.Header.Get("Access-Control-Allow-Origin"))
	// Cookie encryption is a property of service-issued cookies (a manual JWT
	// cookie, for example). IdP-issued session cookies are opaque and validated
	// upstream, so this is logged, not asserted.
	if cp, ok := s.p.(CookieSubjectProvider); ok {
		name, value := cp.Cookie(s.t)
		s.t.Logf("session cookie %s length %d (encryption is driver-specific)", name, len(value))
	}
}

func (s *suite) productCrud() {
	s.t.Helper()
	// Subjects are fetched lazily, immediately before the call that uses them:
	// single-subject stacks (one machine token) hold only one role at a time,
	// so a later Principal call would clear an earlier subject's role.
	editor := s.p.Principal(s.t, "editor")
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
	editor = s.p.Principal(s.t, "editor")
	if code := s.status(http.MethodPatch, "/products/"+created.ID, editor.Token, map[string]any{"price": 8.0}); code != 200 {
		s.t.Errorf("PATCH product = %d, want 200", code)
	}
	// Audit trail records the writes.
	admin := s.p.Principal(s.t, "admin")
	if code := s.status(http.MethodGet, "/admin/products/"+created.ID+"/audit", admin.Token, nil); code != 200 {
		s.t.Errorf("admin GET audit = %d, want 200", code)
	}
	// Soft delete hides the row from reads.
	editor = s.p.Principal(s.t, "editor")
	if code := s.status(http.MethodDelete, "/products/"+created.ID, editor.Token, nil); code != 200 {
		s.t.Errorf("DELETE product = %d, want 200", code)
		return
	}
	editor = s.p.Principal(s.t, "editor")
	if code := s.status(http.MethodGet, "/products/"+created.ID, editor.Token, nil); code != 404 {
		s.t.Errorf("GET soft-deleted product = %d, want 404", code)
	}
}

func (s *suite) adminSelfProtection() {
	s.t.Helper()
	admin := s.p.Principal(s.t, "admin")
	if admin.ID == "" {
		s.t.Logf("provider has no subject id, skipping self-protection checks")
		return
	}
	// An admin cannot delete itself.
	if code := s.status(http.MethodDelete, "/admin/users/"+admin.ID, admin.Token, nil); code != 403 {
		s.t.Errorf("admin DELETE self = %d, want 403", code)
	}
	// An admin cannot change its own role.
	if code := s.status(http.MethodPatch, "/admin/users/"+admin.ID+"/role", admin.Token, map[string]any{"role": "viewer"}); code != 403 {
		s.t.Errorf("admin PATCH own role = %d, want 403", code)
	}
	// A viewer cannot list users.
	viewer := s.p.Principal(s.t, "viewer")
	if code := s.status(http.MethodGet, "/admin/users", viewer.Token, nil); code != 403 {
		s.t.Errorf("viewer GET /admin/users = %d, want 403", code)
	}
}

func (s *suite) cookies() {
	s.t.Helper()
	cp, ok := s.p.(CookieSubjectProvider)
	if !ok {
		s.t.Logf("stack has no login cookie, skipping cookie-transport checks")
		return
	}
	name, value := cp.Cookie(s.t)
	if name == "" || value == "" {
		s.t.Errorf("cookie provider returned an empty cookie")
		return
	}
	// The session cookie alone authenticates the cookie-transport endpoint.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/cookie/profile"), nil)
	if err != nil {
		s.t.Fatalf("cookie GET setup: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: name, Value: value, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("cookie GET /cookie/profile: %v", err)
	}
	func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		s.t.Errorf("cookie GET /cookie/profile = %d, want 200", resp.StatusCode)
	}
	// A forged cookie value is rejected.
	bad, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.url("/cookie/profile"), nil)
	if err != nil {
		s.t.Fatalf("bad cookie GET setup: %v", err)
	}
	bad.AddCookie(&http.Cookie{Name: name, Value: "forged-value", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	badResp, err := http.DefaultClient.Do(bad)
	if err != nil {
		s.t.Fatalf("bad cookie GET /cookie/profile: %v", err)
	}
	func() { _ = badResp.Body.Close() }()
	if badResp.StatusCode != 401 {
		s.t.Errorf("forged cookie GET /cookie/profile = %d, want 401", badResp.StatusCode)
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
