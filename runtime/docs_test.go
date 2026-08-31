package runtime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	scalargo "github.com/bdpiprava/scalar-go"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func faviconTestApp(oai *OpenAPIConf) *fiber.App {
	logx.Disable()
	app := fiber.New()
	registerDocs(app, &ServiceConfig{Server: ServerConf{OpenAPI: oai}}, nil)
	return app
}

func TestFavicon_DefaultInline(t *testing.T) {
	app := faviconTestApp(&OpenAPIConf{Enabled: true})

	resp, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("content-type: got %q want image/svg+xml", ct)
	}
	if resp.Header.Get("ETag") == "" {
		t.Error("expected ETag header")
	}
	if resp.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("cache-control: got %q", resp.Header.Get("Cache-Control"))
	}
	body := mustReadTestBody(t, resp)
	if !containsTestString(body, "272727") {
		t.Error("expected default inline SVG (stroke #272727) in body")
	}
}

func TestFavicon_FromDisk(t *testing.T) {
	// favicon_url is resolved relative to the working directory (os.Root ".").
	// Write a temp file in cwd so the scoped read succeeds.
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><circle cx="1" cy="1" r="1" fill="red"/></svg>`
	path := ".test-favicon-custom.svg"
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	app := faviconTestApp(&OpenAPIConf{Enabled: true, FaviconURL: path})

	resp, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	body := mustReadTestBody(t, resp)
	if !containsTestString(body, `fill="red"`) {
		t.Errorf("expected disk file content, got: %s", body)
	}
}

func TestFavicon_DiskMissing_Fallback(t *testing.T) {
	app := faviconTestApp(&OpenAPIConf{
		Enabled:    true,
		FaviconURL: filepath.Join(t.TempDir(), "nope.svg"),
	})

	resp, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	body := mustReadTestBody(t, resp)
	if !containsTestString(body, "272727") {
		t.Error("expected fallback to default inline SVG when file missing")
	}
}

func TestFavicon_ETag_NotModified(t *testing.T) {
	app := faviconTestApp(&OpenAPIConf{Enabled: true})

	// First request gets the ETag
	resp1, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	etag := resp1.Header.Get("ETag")
	resp1.Body.Close()
	if etag == "" {
		t.Fatal("expected ETag on first response")
	}

	// Second request with If-None-Match returns 304 without body
	req := httptestNewRequest(t, "GET", "/favicon.ico", "")
	req.Header.Set("If-None-Match", etag)
	resp2, err := app.Test(req)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("status: got %d want 304", resp2.StatusCode)
	}
	if b := mustReadTestBody(t, resp2); len(b) != 0 {
		t.Errorf("expected empty body on 304, got %d bytes", len(b))
	}
}

// httptestNewRequest builds a GET request for app.Test.
func httptestNewRequest(t *testing.T, method, target, _ string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func mustReadTestBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 512)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}

func containsTestString(b []byte, s string) bool {
	return len(b) >= len(s) && indexOfTest(b, s) >= 0
}

func indexOfTest(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}

// --- Remote favicon (URL with TTL) ---

func TestFavicon_Remote_FirstFetch(t *testing.T) {
	logx.Disable()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10" fill="blue"/></svg>`))
	}))
	defer srv.Close()

	fs := loadFavicon(&OpenAPIConf{Enabled: true, FaviconURL: srv.URL + "/logo.svg"})
	defer fs.stopRefreshTicker()
	if hits.Load() == 0 {
		t.Fatal("expected startup fetch to hit the remote server")
	}
	if !containsTestString(fs.data, "fill=\"blue\"") {
		t.Errorf("expected remote content, got: %s", fs.data)
	}
}

func TestFavicon_Remote_Cached(t *testing.T) {
	logx.Disable()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="5"/></svg>`))
	}))
	defer srv.Close()

	fs := loadFavicon(&OpenAPIConf{
		Enabled:        true,
		FaviconURL:     srv.URL + "/logo.svg",
		FaviconRefresh: "1h",
	})
	defer fs.stopRefreshTicker()
	before := hits.Load()

	// Serving within TTL must not re-download.
	app := fiber.New()
	app.Get("/favicon.ico", fs.handleFavicon)
	resp, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if hits.Load() != before {
		t.Errorf("expected no re-fetch within TTL: hits %d → %d", before, hits.Load())
	}
}

func TestFavicon_Remote_TTLExpired_Refreshes(t *testing.T) {
	logx.Disable()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="4"/></svg>`))
	}))
	defer srv.Close()

	fs := loadFavicon(&OpenAPIConf{
		Enabled:        true,
		FaviconURL:     srv.URL + "/logo.svg",
		FaviconRefresh: "50ms",
	})
	defer fs.stopRefreshTicker()
	initial := hits.Load()

	// Force expiry (TTL 50ms already passed by the time we serve).
	time.Sleep(80 * time.Millisecond)
	app := fiber.New()
	app.Get("/favicon.ico", fs.handleFavicon)
	resp, _ := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	resp.Body.Close()

	// Background refresh is async — give it a moment.
	time.Sleep(100 * time.Millisecond)
	if hits.Load() <= initial {
		t.Errorf("expected background refresh after TTL expiry: hits %d → %d", initial, hits.Load())
	}
}

func TestFavicon_Remote_Error_ServesStale(t *testing.T) {
	logx.Disable()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n > 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="7"/></svg>`))
	}))
	defer srv.Close()

	fs := loadFavicon(&OpenAPIConf{
		Enabled:        true,
		FaviconURL:     srv.URL + "/logo.svg",
		FaviconRefresh: "30ms",
	})
	defer fs.stopRefreshTicker()
	good := string(fs.data)

	time.Sleep(60 * time.Millisecond)
	app := fiber.New()
	app.Get("/favicon.ico", fs.handleFavicon)
	resp, _ := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	defer resp.Body.Close()
	body := mustReadTestBody(t, resp)

	if !containsTestString(body, "rect width=\"7\"") {
		t.Errorf("expected stale content served on refresh error, got: %s", body)
	}
	if string(body) != good {
		t.Errorf("stale content differs from original fetch")
	}
}

func TestFavicon_URL_Detection(t *testing.T) {
	logx.Disable()
	if !isRemoteFavicon("https://cdn.example.com/logo.svg") {
		t.Error("https URL should be remote")
	}
	if !isRemoteFavicon("http://example.com/favicon.ico") {
		t.Error("http URL should be remote")
	}
	if isRemoteFavicon("static/logo.svg") {
		t.Error("local path should not be remote")
	}
	if isRemoteFavicon("") {
		t.Error("empty should not be remote")
	}
}

func TestFavicon_RemoteContentType(t *testing.T) {
	logx.Disable()
	if ct := remoteContentType("https://cdn.example.com/logo.svg"); ct != "image/svg+xml" {
		t.Errorf("svg: got %q", ct)
	}
	if ct := remoteContentType("https://cdn.example.com/favicon.ico"); ct != "image/x-icon" {
		t.Errorf("ico: got %q", ct)
	}
	if ct := remoteContentType("https://cdn.example.com/favicon.png"); ct != "image/png" {
		t.Errorf("png: got %q", ct)
	}
	if ct := remoteContentType("not a url"); ct != "image/svg+xml" {
		t.Errorf("invalid: got %q", ct)
	}
}

func TestFavicon_Local_WithRealLogo(t *testing.T) {
	logx.Disable()
	// Simulate the real natuleadan logo: a larger SVG read from disk.
	logo := `<svg viewBox="0 0 1576 1576" xmlns="http://www.w3.org/2000/svg"><path d="M0 0h1576v1576H0z" fill="#272727"/><text x="50%" y="50%" text-anchor="middle" font-size="200">N</text></svg>`
	path := ".test-favicon-logo.svg"
	if err := os.WriteFile(path, []byte(logo), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	app := faviconTestApp(&OpenAPIConf{Enabled: true, FaviconURL: path})
	resp, err := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body := mustReadTestBody(t, resp)
	if !containsTestString(body, "1576") {
		t.Errorf("expected the real logo content, got: %s", body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("content-type: got %q", ct)
	}
}

func TestFavicon_Remote_Serves304(t *testing.T) {
	logx.Disable()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg"><rect width="%d"/></svg>`, hits.Load()))
	}))
	defer srv.Close()

	fs := loadFavicon(&OpenAPIConf{
		Enabled:        true,
		FaviconURL:     srv.URL + "/logo.svg",
		FaviconRefresh: "1h",
	})
	defer fs.stopRefreshTicker()
	app := fiber.New()
	app.Get("/favicon.ico", fs.handleFavicon)

	resp1, _ := app.Test(httptestNewRequest(t, "GET", "/favicon.ico", ""))
	etag := resp1.Header.Get("ETag")
	resp1.Body.Close()
	if etag == "" {
		t.Fatal("expected ETag on remote favicon")
	}

	req := httptestNewRequest(t, "GET", "/favicon.ico", "")
	req.Header.Set("If-None-Match", etag)
	resp2, _ := app.Test(req)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("status: got %d want 304", resp2.StatusCode)
	}
}

// ---- OpenAPI spec cache TTL + Scalar theme (fix: fields were parsed but ignored) ----

func docsTestConfig() *ServiceConfig {
	return &ServiceConfig{
		Name: "docs-svc",
		Server: ServerConf{
			OpenAPI: &OpenAPIConf{
				Enabled:      true,
				Theme:    "moon",
				DarkMode: true,
			},
		},
	}
}

func TestSpecCacheTTL(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "", want: time.Hour},
		{raw: "1h", want: time.Hour},
		{raw: "30m", want: 30 * time.Minute},
		{raw: "2h30m", want: 150 * time.Minute},
		{raw: "bogus", want: time.Hour},
		{raw: "-5m", want: time.Hour},
	}
	for _, tc := range cases {
		if got := specCacheTTL(tc.raw); got != tc.want {
			t.Errorf("specCacheTTL(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestRegisterDocs_SpecCacheHeader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	registerDocs(app, docsTestConfig(), nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/openapi.json", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Cache-Control")
	if got != "public, max-age=3600" {
		t.Errorf("cache-control: got %q want 3600 (1h)", got)
	}
}

func TestRegisterDocs_SpecCacheHeader_CustomTTL(t *testing.T) {
	logx.Disable()
	cfg := docsTestConfig()
	cfg.Server.OpenAPI.SpecCacheTTL = "10m"
	app := fiber.New()
	registerDocs(app, cfg, nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/openapi.json", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("Cache-Control")
	if got != "public, max-age=600" {
		t.Errorf("cache-control: got %q want 600 (10m)", got)
	}
}

func TestRegisterDocs_ThemeRendered(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	registerDocs(app, docsTestConfig(), nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/docs", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body := mustReadTestBody(t, resp)
	if !containsTestString(body, `"theme":"moon"`) {
		t.Error("rendered /docs HTML does not embed theme moon")
	}
	if !containsTestString(body, `"darkMode":true`) {
		t.Error("rendered /docs HTML does not embed darkMode true")
	}
}

// ---- YAML-driven theming, display options and hooks ----

func TestRegisterDocs_DisplayOptionsRendered(t *testing.T) {
	logx.Disable()
	cfg := docsTestConfig()
	cfg.Server.OpenAPI.Layout = "modern"
	cfg.Server.OpenAPI.HideDownload = true
	cfg.Server.OpenAPI.PersistAuth = true
	app := fiber.New()
	registerDocs(app, cfg, nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/docs", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body := mustReadTestBody(t, resp)

	for _, want := range []string{`"layout":"modern"`, `"hideDownloadButton":true`, `"persistAuth":true`} {
		if !containsTestString(body, want) {
			t.Errorf("docs HTML missing %s", want)
		}
	}
}

func TestRegisterDocs_CustomCSSAndJS(t *testing.T) {
	logx.Disable()
	cfg := docsTestConfig()
	cfg.Server.OpenAPI.CustomCSS = ":root { --scalar-color-accent: #b32323; }"
	cfg.Server.OpenAPI.CustomHeadJS = "console.log('head-ok');"
	cfg.Server.OpenAPI.CustomBodyJS = "console.log('body-ok');"
	app := fiber.New()
	registerDocs(app, cfg, nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/docs", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body := mustReadTestBody(t, resp)

	if !containsTestString(body, "--scalar-color-accent: #b32323") {
		t.Error("custom_css not injected in docs HTML")
	}
	if !containsTestString(body, "head-ok") {
		t.Error("custom_head_js not injected in docs HTML")
	}
	if !containsTestString(body, "body-ok") {
		t.Error("custom_body_js not injected in docs HTML")
	}
}

func TestRegisterDocs_TitleAndDescriptionInSpec(t *testing.T) {
	logx.Disable()
	cfg := docsTestConfig()
	cfg.Server.OpenAPI.Title = "Custom Docs Title"
	cfg.Server.OpenAPI.Description = "End-to-end description"
	app := fiber.New()
	registerDocs(app, cfg, nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/openapi.json", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body := mustReadTestBody(t, resp)

	if !containsTestString(body, "Custom Docs Title") {
		t.Error("spec info.title not overridden by openapi.title")
	}
	if !containsTestString(body, "End-to-end") {
		t.Error("spec info.description missing")
	}
}

func TestRegisterDocs_MutatorHook(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	hooks := &DocsHooks{
		Mutators: []SpecMutator{func(spec *openapi3.T) error {
			spec.Info.Extensions["x-hook-marker"] = "mutator-applied"
			return nil
		}},
	}
	registerDocs(app, docsTestConfig(), nil, hooks)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/openapi.json", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if !containsTestString(mustReadTestBody(t, resp), "mutator-applied") {
		t.Error("SpecMutator hook did not apply to the served spec")
	}
}

func TestRegisterDocs_ScalarOptionsEscapeHatch(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	hooks := &DocsHooks{
		ScalarOptions: []scalargo.Option{scalargo.WithSearchHotKey("k")},
	}
	registerDocs(app, docsTestConfig(), nil, hooks)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/docs", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if !containsTestString(mustReadTestBody(t, resp), `"searchHotKey":"k"`) {
		t.Error("WithScalarOptions escape hatch option not rendered")
	}
}

func TestRegisterDocs_CSPConnectHeader(t *testing.T) {
	logx.Disable()
	cfg := docsTestConfig()
	cfg.Server.OpenAPI.CSPConnect = []string{"https://api-child.example.com"}
	app := fiber.New()
	registerDocs(app, cfg, nil)

	resp, err := app.Test(httptestNewRequest(t, "GET", "/docs", ""))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "https://api-child.example.com") {
		t.Errorf("csp_connect host missing from docs CSP: %q", csp)
	}
	if !strings.Contains(csp, "connect-src") {
		t.Errorf("connect-src missing from docs CSP: %q", csp)
	}
}

func TestService_WithOpenAPIMutatorAccumulates(t *testing.T) {
	svc := &Service{}
	svc.WithOpenAPIMutator(func(spec *openapi3.T) error { return nil })
	svc.WithScalarOptions(scalargo.WithTheme("moon"))
	if len(svc.openAPIMutators) != 1 {
		t.Errorf("openAPIMutators = %d, want 1", len(svc.openAPIMutators))
	}
	if len(svc.scalarOptions) != 1 {
		t.Errorf("scalarOptions = %d, want 1", len(svc.scalarOptions))
	}
}
