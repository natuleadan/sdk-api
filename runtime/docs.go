package runtime

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	scalargo "github.com/bdpiprava/scalar-go"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/hash"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// defaultFaviconSVG is an inline SVG magnifying glass icon used as default favicon.
const defaultFaviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#272727" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>`

// faviconContentType returns the Content-Type for a favicon file based on its extension.
func faviconContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".webp":
		return "image/webp"
	default:
		return "image/svg+xml"
	}
}

// faviconSource holds the favicon bytes and their cache metadata.
// For remote favicons, the bytes are refreshed in the background when the
// TTL expires (stale-while-revalidate), so requests never block on the network.
type faviconSource struct {
	data        []byte
	contentType string
	etag        string

	mu         sync.Mutex
	remoteURL  string
	refreshTTL time.Duration
	expiresAt  time.Time
	cancel     context.CancelFunc
}

// isRemote reports whether the configured favicon URL is an http(s) URL.
func isRemoteFavicon(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// parseDurationSafe parses a duration string; on error returns the fallback.
func parseDurationSafe(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		logx.Infof("openapi: invalid duration %q, using %s", s, fallback)
		return fallback
	}
	return d
}

// doRefresh downloads the remote favicon and updates the cache. On failure the
// previous bytes are kept (stale serve). The caller provides the context for
// timeout control.
func (fs *faviconSource) doRefresh(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fs.remoteURL, nil)
	if err != nil {
		logx.Infof("openapi: favicon refresh request failed (%v), serving stale", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logx.Infof("openapi: favicon refresh failed (%v), serving stale", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		logx.Infof("openapi: favicon refresh got HTTP %d, serving stale", resp.StatusCode)
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB cap
	if err != nil {
		logx.Infof("openapi: favicon refresh read failed (%v), serving stale", err)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || !strings.HasPrefix(ct, "image/") {
		ct = faviconContentType(fs.remoteURL)
	}
	fs.mu.Lock()
	fs.data = b
	fs.contentType = ct
	fs.etag = fmt.Sprintf(`"%x"`, hash.Md5(b))
	fs.expiresAt = time.Now().Add(fs.refreshTTL)
	fs.mu.Unlock()
	logx.Infof("openapi: favicon refreshed from %q (%d bytes)", fs.remoteURL, len(b))
}

// loadFavicon builds the favicon source from the openapi config. Three modes:
//   - ""           → inline SVG magnifying glass (no I/O)
//   - http(s) URL  → remote, downloaded server-side with TTL cache
//   - local path   → file on disk (relative to working dir), read once;
//     falls back to inline if missing or unreadable
func loadFavicon(oai *OpenAPIConf) *faviconSource {
	fs := &faviconSource{data: []byte(defaultFaviconSVG), contentType: "image/svg+xml"}
	fs.etag = fmt.Sprintf(`"%x"`, hash.Md5(fs.data))
	if oai == nil || oai.FaviconURL == "" {
		return fs
	}

	switch {
	case isRemoteFavicon(oai.FaviconURL):
		fs.remoteURL = oai.FaviconURL
		fs.refreshTTL = parseDurationSafe(oai.FaviconRefresh, 24*time.Hour)
		fs.refreshAtStartup()
		if len(fs.data) == len(defaultFaviconSVG) && string(fs.data) == defaultFaviconSVG {
			// Initial fetch failed — still serve inline until a refresh succeeds.
			logx.Infof("openapi: remote favicon %q not reachable at startup, using default until refresh", oai.FaviconURL)
		}
		fs.startRefreshTicker()
	default:
		fs = localFaviconSource(oai.FaviconURL, fs)
	}
	return fs
}

// refreshAtStartup performs the initial synchronous fetch during server init.
func (fs *faviconSource) refreshAtStartup() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fs.doRefresh(ctx)
}

// startRefreshTicker launches a single background goroutine that refreshes
// the favicon periodically. The goroutine uses context.Background because
// it is a server-lifetime task, not tied to any HTTP request.
func (fs *faviconSource) startRefreshTicker() {
	ttl := fs.refreshTTL
	if ttl <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	fs.cancel = cancel
	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fs.doRefresh(ctx)
			}
		}
	}()
}

// stopRefreshTicker cancels the background refresh goroutine if running.
func (fs *faviconSource) stopRefreshTicker() {
	if fs.cancel != nil {
		fs.cancel()
		fs.cancel = nil
	}
}

// localFaviconSource reads a favicon file from the working directory (scoped
// with os.Root to prevent path traversal). Falls back to the provided source.
func localFaviconSource(path string, fallback *faviconSource) *faviconSource {
	root, err := os.OpenRoot(".")
	if err == nil {
		defer func() { _ = root.Close() }()
		b, readErr := root.ReadFile(path)
		if readErr != nil {
			logx.Infof("openapi: favicon %q not found, using default", path)
			return fallback
		}
		fs := &faviconSource{
			data:        b,
			contentType: faviconContentType(path),
			etag:        fmt.Sprintf(`"%x"`, hash.Md5(b)),
		}
		logx.Infof("openapi: serving favicon from %q", path)
		return fs
	}
	logx.Infof("openapi: cannot open working dir for favicon, using default")
	return fallback
}

// handleFavicon serves the favicon with browser caching (ETag + 304).
// Refresh is handled by the background ticker started in loadFavicon,
// so this handler is purely synchronous.
func (fs *faviconSource) handleFavicon(c fiber.Ctx) error {
	fs.mu.Lock()
	data, ct, etag := fs.data, fs.contentType, fs.etag
	fs.mu.Unlock()

	c.Set("Content-Type", ct)
	c.Set("ETag", etag)
	c.Set("Cache-Control", "public, max-age=86400")
	if c.Get("If-None-Match") == etag {
		c.Status(fiber.StatusNotModified)
		return nil
	}
	return c.Send(data)
}

// remoteContentType extracts content type from a URL path's extension.
func remoteContentType(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "image/svg+xml"
	}
	return faviconContentType(u.Path)
}

// registerDocs registers /openapi.json and /docs (Scalar UI) endpoints
// if the server.openapi.enabled config is true.
// SpecMutator mutates the generated OpenAPI spec before it is marshaled and
// rendered. Use it for spec content that YAML cannot express.
type SpecMutator func(spec *openapi3.T) error

// WithOpenAPIMutator registers a hook applied to the generated OpenAPI spec
// before it is served and rendered. Use it for the parts of the docs that the
// openapi YAML block cannot express (dynamic operations, computed schemas...).
func (s *Service) WithOpenAPIMutator(fn SpecMutator) *Service {
	s.openAPIMutators = append(s.openAPIMutators, fn)
	return s
}

// WithScalarOptions appends raw scalar-go render options on top of everything
// configured via the openapi YAML block. Escape hatch for exotic needs.
func (s *Service) WithScalarOptions(opts ...scalargo.Option) *Service {
	s.scalarOptions = append(s.scalarOptions, opts...)
	return s
}

// openAPIHooks snapshots the Service hooks for registerDocs.
func (s *Service) openAPIHooks() *DocsHooks {
	if len(s.openAPIMutators) == 0 && len(s.scalarOptions) == 0 {
		return nil
	}
	return &DocsHooks{Mutators: s.openAPIMutators, ScalarOptions: s.scalarOptions}
}

// DocsHooks carries the Service-level OpenAPI hooks into registerDocs.
type DocsHooks struct {
	// Mutators run in registration order on the generated spec.
	Mutators []SpecMutator
	// ScalarOptions are appended to the options built from the YAML config.
	ScalarOptions []scalargo.Option
}

// registerDocs mounts /openapi.json, /docs (Scalar UI) and /favicon.ico when
// server.openapi is enabled. Optional hooks customize the spec and renderer.
func registerDocs(app *fiber.App, cfg *ServiceConfig, models map[string]*db.TableInfo, hooks ...*DocsHooks) {
	oai := cfg.Server.OpenAPI
	if oai == nil || !oai.Enabled {
		return
	}

	// Register /favicon.ico endpoint with in-memory cache + ETag.
	// The favicon source is resolved once at startup (inline / local file /
	// remote with TTL refresh); every request is a cheap memory copy, and
	// 304 responses skip the body entirely (Cache-Control max-age=86400).
	faviconSrc := loadFavicon(oai)
	app.Get("/favicon.ico", faviconSrc.handleFavicon)

	spec, err := BuildOpenAPI(cfg, models)
	if err != nil {
		logx.Errorf("openapi build: %v", err)
		return
	}

	var dh *DocsHooks
	if len(hooks) > 0 {
		dh = hooks[0]
	}
	if dh != nil {
		for _, mutate := range dh.Mutators {
			if err := mutate(spec); err != nil {
				logx.Errorf("openapi mutator: %v", err)
			}
		}
	}
	if oai.Title != "" && spec.Info != nil {
		spec.Info.Title = oai.Title
	}
	if oai.Description != "" && spec.Info != nil {
		spec.Info.Description = oai.Description
	}

	jsonData, err := spec.MarshalJSON()
	if err != nil {
		logx.Errorf("openapi marshal: %v", err)
		return
	}

	var (
		compressedSpec []byte
		compressOnce   sync.Once
	)
	compressOnce.Do(func() {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(jsonData); err != nil {
			logx.Errorf("gzip write: %v", err)
		}
		if err := gw.Close(); err != nil {
			logx.Errorf("gzip close: %v", err)
		}
		compressedSpec = buf.Bytes()
	})

	specPath := oai.SpecPath
	if specPath == "" {
		specPath = "/openapi.json"
	}
	ttl := specCacheTTL(oai.SpecCacheTTL)
	maxAge := fmt.Sprintf("public, max-age=%d", int(ttl.Seconds()))
	app.Get(specPath, func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		etag := fmt.Sprintf(`"%x"`, hash.Md5(jsonData))
		c.Set("ETag", etag)
		c.Set("Cache-Control", maxAge)
		if c.Get("If-None-Match") == etag {
			c.Status(fiber.StatusNotModified)
			return nil
		}
		if strings.Contains(c.Get("Accept-Encoding"), "gzip") && len(compressedSpec) > 0 {
			c.Set("Content-Encoding", "gzip")
			return c.Send(compressedSpec)
		}
		return c.Send(jsonData)
	})

	opts := buildScalarOptions(oai, jsonData)
	if dh != nil && len(dh.ScalarOptions) > 0 {
		opts = append(opts, dh.ScalarOptions...)
	}
	if len(oai.Sources) > 0 {
		sources := make([]scalargo.DocumentSource, 0, len(oai.Sources))
		for _, src := range oai.Sources {
			sources = append(sources, scalargo.DocumentSource{
				Title:   src.Title,
				Slug:    src.Slug,
				URL:     src.URL,
				Default: src.Default,
			})
		}
		opts = append(opts, scalargo.WithMultipleSources(sources...))
	}

	scalarHTML, err := scalargo.NewV2(opts...)
	if err != nil {
		logx.Errorf("scalar render: %v", err)
		return
	}

	docsPath := oai.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}
	app.Get(docsPath, func(c fiber.Ctx) error {
		if len(oai.CSPConnect) > 0 {
			c.Set("Content-Security-Policy", buildDocsCSP(oai.CSPConnect))
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(scalarHTML)
	})

	logx.Infof("docs: %s and %s", specPath, docsPath)
}

// buildDocsCSP builds a Content-Security-Policy for the docs page that lets
// Scalar "Try It" reach the configured API hosts.
func buildDocsCSP(extra []string) string {
	connect := append([]string{"'self'", "https://cdn.jsdelivr.net", "https://api.scalar.com"}, extra...)
	return "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
		"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
		"img-src 'self' data: https:; " +
		"connect-src " + strings.Join(connect, " ") + "; " +
		"font-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com https://cdn.jsdelivr.net https://fonts.scalar.com; " +
		"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
}

// buildScalarOptions maps the OpenAPIConf onto scalar-go render options.
// It is the single place where YAML-driven theming and display settings are
// translated; registerDocs calls it before rendering the /docs page.
func buildScalarOptions(oai *OpenAPIConf, spec []byte) []scalargo.Option {
	opts := []scalargo.Option{
		scalargo.WithSpecBytes(spec),
		scalargo.WithDefaultFonts(),
	}
	if oai.Theme != "" {
		opts = append(opts, scalargo.WithTheme(scalargo.Theme(oai.Theme)))
	}
	if oai.DarkMode {
		opts = append(opts, scalargo.WithDarkMode())
	}
	if oai.ForceDarkMode {
		opts = append(opts, scalargo.WithForceDarkMode())
	}
	if oai.HideDarkModeToggle {
		opts = append(opts, scalargo.WithHideDarkModeToggle())
	}
	if oai.Layout != "" {
		opts = append(opts, scalargo.WithLayout(scalargo.Layout(oai.Layout)))
	}
	if oai.CustomCSS != "" {
		opts = append(opts, scalargo.WithOverrideCSS(oai.CustomCSS))
	}
	if oai.CustomHeadJS != "" {
		opts = append(opts, scalargo.WithCustomHeadJS(oai.CustomHeadJS))
	}
	if oai.CustomBodyJS != "" {
		opts = append(opts, scalargo.WithCustomBodyJS(oai.CustomBodyJS))
	}
	if oai.Title != "" {
		opts = append(opts, scalargo.WithMetaDataOpts(scalargo.WithTitle(oai.Title)))
	}
	if oai.Description != "" {
		opts = append(opts, scalargo.WithMetaDataOpts(scalargo.WithKeyValue("description", oai.Description)))
	}
	if oai.HideDownload {
		opts = append(opts, scalargo.WithHideDownloadButton())
	}
	if oai.HideModels {
		opts = append(opts, scalargo.WithHideModels())
	}
	if oai.HideSearch {
		opts = append(opts, scalargo.WithHideSearch(true))
	}
	if oai.Sidebar != nil {
		opts = append(opts, scalargo.WithSidebarVisibility(*oai.Sidebar))
	}
	if oai.ShowToolbar != "" {
		opts = append(opts, scalargo.WithShowToolbar(scalargo.ShowToolbarOption(oai.ShowToolbar)))
	}
	if oai.SearchHotKey != "" {
		opts = append(opts, scalargo.WithSearchHotKey(oai.SearchHotKey))
	}
	if oai.Editable {
		opts = append(opts, scalargo.WithEditable())
	}
	if oai.TagsSorter != "" {
		opts = append(opts, scalargo.WithTagsSorter(scalargo.SorterOption(oai.TagsSorter)))
	}
	if oai.OperationsSorter != "" {
		opts = append(opts, scalargo.WithOperationsSorter(scalargo.SorterOption(oai.OperationsSorter)))
	}
	if oai.OperationTitleSource != "" {
		opts = append(opts, scalargo.WithOperationTitleSource(scalargo.OperationTitleSource(oai.OperationTitleSource)))
	}
	if oai.OrderSchemaPropertiesBy != "" {
		opts = append(opts, scalargo.WithOrderSchemaPropertiesBy(scalargo.SchemaPropertiesOrder(oai.OrderSchemaPropertiesBy)))
	}
	if oai.PersistAuth {
		opts = append(opts, scalargo.WithPersistAuth(true))
	}
	if oai.DefaultHTTPClient != nil {
		opts = append(opts, scalargo.WithDefaultHTTPClient(oai.DefaultHTTPClient.Target, oai.DefaultHTTPClient.Client))
	}
	if len(oai.HiddenClients) > 0 {
		opts = append(opts, scalargo.WithHiddenClients(oai.HiddenClients...))
	}
	if oai.CDN != "" {
		opts = append(opts, scalargo.WithCDN(oai.CDN))
	}
	if oai.Proxy != "" {
		opts = append(opts, scalargo.WithProxy(oai.Proxy))
	}
	if oai.BaseServerURL != "" {
		opts = append(opts, scalargo.WithBaseServerURL(oai.BaseServerURL))
	}
	if len(oai.ServersOverride) > 0 {
		overrides := make([]scalargo.ServerOverride, 0, len(oai.ServersOverride))
		for _, so := range oai.ServersOverride {
			overrides = append(overrides, scalargo.ServerOverride{URL: so.URL, Description: so.Description})
		}
		opts = append(opts, scalargo.WithServers(overrides...))
	}
	return opts
}

// specCacheTTL parses the configured Cache-Control max-age for /openapi.json.
// Falls back to one hour on empty or invalid input.
func specCacheTTL(raw string) time.Duration {
	if raw == "" {
		return time.Hour
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		logx.Errorf("openapi spec_cache_ttl %q invalid, falling back to 1h", raw)
		return time.Hour
	}
	return parsed
}
