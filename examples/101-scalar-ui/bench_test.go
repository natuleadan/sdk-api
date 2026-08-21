package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	baseURL = "http://localhost:23101"
	// Allowed origins per CORS group (must match service.yaml)
	appOrigin   = "https://app.example.com"
	hooksOrigin = "https://hooks.example.com"
	evilOrigin  = "https://evil.example.com"
)

var (
	setupOnce sync.Once
	svcCmd    *exec.Cmd
	docker    bool
)

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"

	if !docker {
		out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/scalar-ui-svc", "./cmd/").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}

	fmt.Println("=== scalar-ui cors matrix ===")
	code := m.Run()

	if !docker {
		teardown()
	}
	os.Exit(code)
}

func setup(tb testing.TB) {
	tb.Helper()
	if docker {
		setupOnce.Do(func() { waitHTTP(tb, baseURL+"/healthz", 30*time.Second) })
		return
	}
	setupOnce.Do(func() {
		svcCmd = exec.Command("/tmp/scalar-ui-svc")
		svcCmd.Env = append(os.Environ(), "PORT=23101")
		svcCmd.Stdout = os.Stdout
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start svc: %v", err)
		}
		waitHTTP(tb, baseURL+"/healthz", 30*time.Second)
	})
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Kill()
		svcCmd.Wait()
	}
}

func waitHTTP(tb testing.TB, url string, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	tb.Fatalf("timeout waiting for %s", url)
}

// preflight sends an OPTIONS request and returns the response headers.
func preflight(t *testing.T, path, origin, method string, headers ...string) http.Header {
	t.Helper()
	req, err := http.NewRequest("OPTIONS", baseURL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", method)
	if len(headers) > 0 {
		req.Header.Set("Access-Control-Request-Headers", strings.Join(headers, ", "))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight %s: %v", path, err)
	}
	defer resp.Body.Close()
	return resp.Header
}

func hasHeader(h http.Header, name string) bool {
	return h.Get(name) != ""
}

// preflightAllowed reports whether the preflight response allows the origin.
func preflightAllowed(h http.Header) bool {
	return hasHeader(h, "Access-Control-Allow-Origin")
}

// --- Matrix-driven exhaustive CORS tests ---
//
// Each row declares: path, origin, method, and what the group MUST allow or
// MUST reject. Every combination is verified against the live service.

type corsCase struct {
	name       string
	path       string
	origin     string
	method     string
	reqHeaders []string
	// expectations
	allowOrigin string   // exact Allow-Origin value, "" = must be absent
	allowCreds  bool     // Allow-Credentials must be "true"
	allowMtd    []string // methods that MUST be present in Allow-Methods
	denyMtd     []string // methods that MUST be absent in Allow-Methods
	expose      []string // headers that MUST be present in Expose-Headers
	denyExpose  []string // headers that MUST be absent in Expose-Headers
	maxAge      string   // Access-Control-Max-Age expected value ("" = not checked)
}

func runCORSMatrix(t *testing.T, cases []corsCase) {
	t.Helper()
	setup(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := preflight(t, tc.path, tc.origin, tc.method, tc.reqHeaders...)

			got := h.Get("Access-Control-Allow-Origin")
			if tc.allowOrigin == "" {
				if got != "" {
					t.Errorf("%s: origin %q must be REJECTED, got Allow-Origin %q", tc.path, tc.origin, got)
				}
			} else if got != tc.allowOrigin {
				t.Errorf("%s: want Allow-Origin %q, got %q", tc.path, tc.allowOrigin, got)
			}

			if tc.allowCreds {
				if got := h.Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("%s: want Allow-Credentials true, got %q", tc.path, got)
				}
			} else if hasHeader(h, "Access-Control-Allow-Credentials") {
				t.Errorf("%s: Allow-Credentials must be absent for this group", tc.path)
			}

			methods := h.Get("Access-Control-Allow-Methods")
			for _, m := range tc.allowMtd {
				if !strings.Contains(methods, m) {
					t.Errorf("%s: method %s must be allowed, got %q", tc.path, m, methods)
				}
			}
			for _, m := range tc.denyMtd {
				if strings.Contains(methods, m) {
					t.Errorf("%s: method %s must be DENIED, got %q", tc.path, m, methods)
				}
			}

			expose := h.Get("Access-Control-Expose-Headers")
			for _, e := range tc.expose {
				if !strings.Contains(expose, e) {
					t.Errorf("%s: expose %s must be present, got %q", tc.path, e, expose)
				}
			}
			for _, e := range tc.denyExpose {
				if strings.Contains(expose, e) {
					t.Errorf("%s: expose %s must be ABSENT, got %q", tc.path, e, expose)
				}
			}

			if tc.maxAge != "" {
				if got := h.Get("Access-Control-Max-Age"); got != tc.maxAge {
					t.Errorf("%s: want Max-Age %s, got %q", tc.path, tc.maxAge, got)
				}
			}
		})
	}
}

func TestCORS_Matrix_AllEndpoints(t *testing.T) {
	cases := []corsCase{
		// --- docs (wildcard, GET only, no credentials) ---
		{name: "docs_any_origin_GET", path: "/docs", origin: evilOrigin, method: "GET", allowOrigin: "*", allowMtd: []string{"GET"}},
		{name: "docs_POST_denied", path: "/docs", origin: evilOrigin, method: "POST", allowOrigin: "*", denyMtd: []string{"POST", "DELETE", "PATCH"}},
		{name: "docs_no_credentials", path: "/docs", origin: evilOrigin, method: "GET", allowOrigin: "*"},

		// --- app group: ping (rest) ---
		{name: "ping_app_origin_GET", path: "/api/ping", origin: appOrigin, method: "GET", allowOrigin: appOrigin, allowCreds: true},
		{name: "ping_evil_denied", path: "/api/ping", origin: evilOrigin, method: "GET"},
		{name: "ping_hooks_denied", path: "/api/ping", origin: hooksOrigin, method: "GET"},
		{name: "ping_app_POST_allowed", path: "/api/ping", origin: appOrigin, method: "POST", allowOrigin: appOrigin, allowCreds: true, allowMtd: []string{"GET", "POST", "PATCH", "DELETE"}},
		{name: "ping_app_expose", path: "/api/ping", origin: appOrigin, method: "GET", allowOrigin: appOrigin, allowCreds: true, expose: []string{"X-Request-ID"}},

		// --- app group: upload (file) ---
		{name: "upload_app_origin_POST", path: "/api/upload", origin: appOrigin, method: "POST", allowOrigin: appOrigin, allowCreds: true},
		{name: "upload_evil_denied", path: "/api/upload", origin: evilOrigin, method: "POST"},

		// --- app group: jobs (async) ---
		{name: "jobs_app_origin_POST", path: "/api/jobs", origin: appOrigin, method: "POST", allowOrigin: appOrigin, allowCreds: true},
		{name: "jobs_evil_denied", path: "/api/jobs", origin: evilOrigin, method: "POST"},

		// --- app group: products (crud) ---
		{name: "products_app_origin_DELETE", path: "/api/products", origin: appOrigin, method: "DELETE", allowOrigin: appOrigin, allowCreds: true, allowMtd: []string{"GET", "POST", "PATCH", "DELETE"}},
		{name: "products_evil_denied", path: "/api/products", origin: evilOrigin, method: "GET"},

		// --- app group: graphql ---
		{name: "graphql_app_origin_POST", path: "/api/graphql", origin: appOrigin, method: "POST", allowOrigin: appOrigin, allowCreds: true},
		{name: "graphql_evil_denied", path: "/api/graphql", origin: evilOrigin, method: "POST"},

		// --- ws group: websocket + sse (GET only, no credentials, private network) ---
		{name: "ws_chat_app_GET", path: "/api/ws/chat", origin: appOrigin, method: "GET", allowOrigin: appOrigin, allowMtd: []string{"GET"}, denyMtd: []string{"POST", "DELETE"}},
		{name: "ws_chat_evil_denied", path: "/api/ws/chat", origin: evilOrigin, method: "GET"},
		{name: "sse_events_app_GET", path: "/api/sse/events", origin: appOrigin, method: "GET", allowOrigin: appOrigin},
		{name: "sse_events_evil_denied", path: "/api/sse/events", origin: evilOrigin, method: "GET"},
		{name: "ws_no_credentials", path: "/api/ws/chat", origin: appOrigin, method: "GET", allowOrigin: appOrigin},
		{name: "sse_no_credentials", path: "/api/sse/events", origin: appOrigin, method: "GET", allowOrigin: appOrigin},

		// --- webhooks group (POST only, hooks origin, no credentials, no expose) ---
		{name: "echo_hooks_POST", path: "/api/echo", origin: hooksOrigin, method: "POST", allowOrigin: hooksOrigin, allowMtd: []string{"POST"}, denyMtd: []string{"GET", "PATCH", "DELETE"}},
		{name: "echo_hooks_GET_denied", path: "/api/echo", origin: hooksOrigin, method: "GET", allowOrigin: hooksOrigin, denyMtd: []string{"GET"}},
		{name: "echo_app_denied", path: "/api/echo", origin: appOrigin, method: "POST"},
		{name: "echo_evil_denied", path: "/api/echo", origin: evilOrigin, method: "POST"},
		{name: "echo_no_credentials", path: "/api/echo", origin: hooksOrigin, method: "POST", allowOrigin: hooksOrigin},
		{name: "echo_no_expose", path: "/api/echo", origin: hooksOrigin, method: "POST", allowOrigin: hooksOrigin, denyExpose: []string{"X-Request-ID"}},

		// --- sub group (subdomain matching: *.example.com, GET only) ---
		{name: "sub_foo_origin_GET", path: "/api/sub", origin: "https://foo.example.com", method: "GET", allowOrigin: "https://foo.example.com", allowMtd: []string{"GET"}, denyMtd: []string{"POST"}},
		{name: "sub_root_denied", path: "/api/sub", origin: "https://example.com", method: "GET"},
		{name: "sub_other_denied", path: "/api/sub", origin: "https://evil.com", method: "GET"},
		{name: "sub_nested_origin_GET", path: "/api/sub", origin: "https://deep.sub.example.com", method: "GET", allowOrigin: "https://deep.sub.example.com"},
		{name: "sub_no_credentials", path: "/api/sub", origin: "https://foo.example.com", method: "GET", allowOrigin: "https://foo.example.com"},

		// --- app group dynamic func (SetCORSOriginsFunc: *.trusted.example.com) ---
		{name: "ping_func_trusted", path: "/api/ping", origin: "https://x.trusted.example.com", method: "GET", allowOrigin: "https://x.trusted.example.com", allowCreds: true},
		{name: "ping_func_untrusted_denied", path: "/api/ping", origin: "https://x.untrusted.example.com", method: "GET"},

		// --- max-age: each group exposes its configured Access-Control-Max-Age ---
		{name: "docs_maxage_3600", path: "/docs", origin: evilOrigin, method: "GET", allowOrigin: "*", maxAge: "3600"},
		{name: "webhooks_maxage_600", path: "/api/echo", origin: hooksOrigin, method: "POST", allowOrigin: hooksOrigin, maxAge: "600"},
		{name: "app_maxage_default", path: "/api/ping", origin: appOrigin, method: "GET", allowOrigin: appOrigin, allowCreds: true, maxAge: "300"},

		// --- internal (no CORS at all, same-origin only) ---
		{name: "internal_any_denied", path: "/api/internal/status", origin: evilOrigin, method: "GET"},
		{name: "internal_app_denied", path: "/api/internal/status", origin: appOrigin, method: "GET"},
	}

	runCORSMatrix(t, cases)
}

// --- WebSocket private-network preflight (ws group) ---

func TestCORS_WS_PrivateNetwork(t *testing.T) {
	setup(t)
	req, err := http.NewRequest("OPTIONS", baseURL+"/api/ws/chat", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", appOrigin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight ws: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("ws: want Allow-Private-Network true, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != appOrigin {
		t.Errorf("ws: want echo %q, got %q", appOrigin, got)
	}
}

func TestCORS_SSE_PrivateNetwork(t *testing.T) {
	setup(t)
	req, err := http.NewRequest("OPTIONS", baseURL+"/api/sse/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", appOrigin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight sse: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("sse: want Allow-Private-Network true, got %q", got)
	}
}

// --- Headers allowlist (app group allows Content-Type, Authorization) ---

func TestCORS_App_AllowedHeaders(t *testing.T) {
	setup(t)
	h := preflight(t, "/api/ping", appOrigin, "POST", "Content-Type", "Authorization")
	if got := h.Get("Access-Control-Allow-Origin"); got != appOrigin {
		t.Errorf("app: want echo %q, got %q", appOrigin, got)
	}
	allowHeaders := h.Get("Access-Control-Allow-Headers")
	for _, hdr := range []string{"Content-Type", "Authorization"} {
		if !strings.Contains(allowHeaders, hdr) {
			t.Errorf("app: allow-headers missing %s (got %q)", hdr, allowHeaders)
		}
	}
}

func TestCORS_App_DisallowedHeader(t *testing.T) {
	setup(t)
	// X-Evil-Header is not in the app group allowlist -> preflight must not
	// list it as allowed.
	h := preflight(t, "/api/ping", appOrigin, "POST", "X-Evil-Header")
	allowHeaders := h.Get("Access-Control-Allow-Headers")
	if strings.Contains(allowHeaders, "X-Evil-Header") {
		t.Errorf("app: X-Evil-Header must NOT be in allow-headers, got %q", allowHeaders)
	}
}

func TestCORS_Webhooks_AllowedHeaders(t *testing.T) {
	setup(t)
	h := preflight(t, "/api/echo", hooksOrigin, "POST", "Content-Type")
	allowHeaders := h.Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "Content-Type") {
		t.Errorf("webhooks: allow-headers missing Content-Type (got %q)", allowHeaders)
	}
	if strings.Contains(allowHeaders, "Authorization") {
		t.Errorf("webhooks: Authorization must NOT be allowed (got %q)", allowHeaders)
	}
}

// --- functional smoke ---

func TestHealthz(t *testing.T) {
	setup(t)
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("healthz: want 200, got %d", resp.StatusCode)
	}
}

func TestOpenAPI_HasEndpoints(t *testing.T) {
	setup(t)
	resp, err := http.Get(baseURL + "/openapi.json")
	if err != nil {
		t.Fatalf("openapi: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("openapi: want 200, got %d", resp.StatusCode)
	}
	body := make([]byte, 1<<20)
	n, _ := resp.Body.Read(body)
	s := string(body[:n])
	for _, p := range []string{"/api/ping", "/api/echo", "/api/products", "/api/internal/status", "/api/upload", "/api/sse/events", "/api/ws/chat"} {
		if !strings.Contains(s, p) {
			t.Errorf("openapi: missing path %s", p)
		}
	}
}

func TestFavicon_Inline(t *testing.T) {
	setup(t)
	resp, err := http.Get(baseURL + "/favicon.ico")
	if err != nil {
		t.Fatalf("favicon: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("favicon: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("favicon: want image/svg+xml, got %q", ct)
	}
	if resp.Header.Get("ETag") == "" {
		t.Error("favicon: want ETag")
	}
}

// --- per-route CSP (csp_groups): docs ampliado, APIs estricto ---

func TestCSP_Docs_Amplified(t *testing.T) {
	setup(t)
	resp, err := http.Get(baseURL + "/docs")
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	defer resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "fonts.scalar.com") {
		t.Errorf("docs: want amplified CSP with fonts.scalar.com, got %q", csp)
	}
	if !strings.Contains(csp, "cdn.jsdelivr.net") {
		t.Errorf("docs: want jsdelivr in CSP, got %q", csp)
	}
}

func TestCSP_APIs_Strict(t *testing.T) {
	setup(t)
	for _, p := range []string{"/api/ping", "/api/internal/status", "/api/products"} {
		resp, err := http.Get(baseURL + p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		resp.Body.Close()
		if strings.Contains(csp, "fonts.scalar.com") {
			t.Errorf("%s: strict CSP must NOT include fonts.scalar.com, got %q", p, csp)
		}
		if strings.Contains(csp, "cdn.jsdelivr.net") {
			t.Errorf("%s: strict CSP must NOT include cdn.jsdelivr.net, got %q", p, csp)
		}
	}
}
