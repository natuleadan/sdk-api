package runtime

import (
	"bytes"
	"compress/gzip"
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

	mu        sync.Mutex
	remoteURL string
	refreshTTL time.Duration
	expiresAt time.Time
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

// refresh downloads the remote favicon and updates the cache. On failure the
// previous bytes are kept (stale serve).
func (fs *faviconSource) refresh() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fs.remoteURL)
	if err != nil {
		logx.Infof("openapi: favicon refresh failed (%v), serving stale", err)
		return
	}
	defer resp.Body.Close()
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
		fs.refresh() // first fetch at startup
		if len(fs.data) == len(defaultFaviconSVG) && string(fs.data) == defaultFaviconSVG {
			// Initial fetch failed — still serve inline until a refresh succeeds.
			logx.Infof("openapi: remote favicon %q not reachable at startup, using default until refresh", oai.FaviconURL)
		}
	default:
		fs = localFaviconSource(oai.FaviconURL, fs)
	}
	return fs
}

// localFaviconSource reads a favicon file from the working directory (scoped
// with os.Root to prevent path traversal). Falls back to the provided source.
func localFaviconSource(path string, fallback *faviconSource) *faviconSource {
	root, err := os.OpenRoot(".")
	if err == nil {
		defer root.Close()
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
func (fs *faviconSource) handleFavicon(c fiber.Ctx) error {
	fs.mu.Lock()
	data, ct, etag := fs.data, fs.contentType, fs.etag
	expired := fs.isRemote() && time.Now().After(fs.expiresAt)
	fs.mu.Unlock()

	// Stale-while-revalidate: if TTL expired, refresh in background (non-blocking).
	if expired {
		go fs.refresh()
	}

	c.Set("Content-Type", ct)
	c.Set("ETag", etag)
	c.Set("Cache-Control", "public, max-age=86400")
	if c.Get("If-None-Match") == etag {
		c.Status(fiber.StatusNotModified)
		return nil
	}
	return c.Send(data)
}

func (fs *faviconSource) isRemote() bool {
	return fs.remoteURL != ""
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
func registerDocs(app *fiber.App, cfg *ServiceConfig, models map[string]*db.TableInfo) {
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
	app.Get(specPath, func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		if strings.Contains(c.Get("Accept-Encoding"), "gzip") && len(compressedSpec) > 0 {
			c.Set("Content-Encoding", "gzip")
			return c.Send(compressedSpec)
		}
		etag := fmt.Sprintf(`"%x"`, hash.Md5(jsonData))
		c.Set("ETag", etag)
		c.Set("Cache-Control", "public, max-age=3600")
		if c.Get("If-None-Match") == etag {
			c.Status(fiber.StatusNotModified)
			return nil
		}
		return c.Send(jsonData)
	})

	opts := []scalargo.Option{
		scalargo.WithSpecBytes(jsonData),
		scalargo.WithDefaultFonts(),
	}
	if oai.DarkMode {
		opts = append(opts, scalargo.WithDarkMode())
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
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(scalarHTML)
	})

	logx.Infof("docs: %s and %s", specPath, docsPath)
}
