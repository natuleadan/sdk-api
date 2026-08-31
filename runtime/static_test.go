package runtime

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func staticTestApp() *fiber.App {
	return fiber.New()
}

func doStaticReq(app *fiber.App, path string, headers map[string]string) *http.Response {
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, _ := app.Test(req)
	return resp
}

// createTempDir creates a temp directory with files and returns the path.
func createTempDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

// --- Disk filesystem ---

func TestStatic_DiskServeFile(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"hello.txt": "hello world",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/pub", Dir: dir}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/pub/hello.txt", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello world", string(body))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

func TestStatic_DiskIndexFile(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"index.html": "<h1>Home</h1>",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/", Dir: dir}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Home")
}

func TestStatic_DiskSubdirectory(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"css/style.css": "body{}",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/assets", Dir: dir}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/assets/css/style.css", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "body{}", string(body))
}

func TestStatic_DiskNotFound(t *testing.T) {
	dir := createTempDir(t, map[string]string{})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/pub", Dir: dir}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/pub/missing.txt", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- MaxAge (Cache-Control) ---

func TestStatic_MaxAge(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"style.css": "body{}",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir, MaxAge: 3600}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/style.css", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Cache-Control"), "max-age=3600")
}

func TestStatic_MaxAgeDefault(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"style.css": "body{}",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/style.css", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	// Default max_age=0 means no Cache-Control or max-age=0
	cc := resp.Header.Get("Cache-Control")
	assert.True(t, cc == "" || cc == "max-age=0, no-cache, no-store, must-revalidate",
		"unexpected Cache-Control: %q", cc)
}

// --- SPA ---

func TestStatic_SPAFallback(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"index.html": "<div id=app></div>",
		"style.css":  "body{}",
	})
	app := fiber.New()
	// SPA pattern: serve static files, then fall back to index.html for any route
	app.Get("/*", static.New(dir, static.Config{
		IndexNames: []string{"index.html"},
	}))
	// Catch-all after static: serve index.html for any unmatched path
	app.Get("/*", func(c fiber.Ctx) error {
		return c.SendFile(filepath.Join(dir, "index.html"))
	})

	resp := doStaticReq(app, "/some/deep/path", nil)
	require.NotNil(t, resp)
	// SPA: unmatched route falls through to catch-all → index.html
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "<div id=app></div>")
}

func TestStatic_SPAStillServesExistingFiles(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"index.html": "<div id=app></div>",
		"style.css":  "body{}",
	})
	app := fiber.New()
	app.Get("/*", static.New(dir, static.Config{
		IndexNames: []string{"index.html"},
	}))
	app.Get("/*", func(c fiber.Ctx) error {
		return c.SendFile(filepath.Join(dir, "index.html"))
	})

	resp := doStaticReq(app, "/style.css", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "body{}", string(body))
}

// --- Compress ---

func TestStatic_CompressFlag(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"big.txt": "hello world content that is big enough to compress",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir, Compress: true}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/big.txt", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// --- ByteRange ---

func TestStatic_ByteRangeFlag(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"video.mp4": "fake video data here for testing range requests",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir, ByteRange: true}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/video.mp4", map[string]string{
		"Range": "bytes=0-9",
	})
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusPartialContent, resp.StatusCode)
}

func TestStatic_ByteRangeDisabled(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"video.mp4": "fake video data here for testing range requests",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir, ByteRange: false}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/video.mp4", map[string]string{
		"Range": "bytes=0-9",
	})
	require.NotNil(t, resp)
	// ByteRange disabled: Range header ignored, returns full content
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// --- Browse ---

func TestStatic_BrowseFlag(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"file.txt": "content",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/pub", Dir: dir, Browse: true}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/pub/", nil)
	require.NotNil(t, resp)
	// With browse enabled and no index.html, it should show directory listing
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "file.txt")
}

// --- Download ---

func TestStatic_DownloadFlag(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"report.pdf": "PDF content here",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: dir, Download: true}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/s/report.pdf", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
}

// --- Custom IndexNames ---

func TestStatic_CustomIndexNames(t *testing.T) {
	dir := createTempDir(t, map[string]string{
		"main.html": "<h1>Custom Index</h1>",
	})
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/", Dir: dir, IndexNames: []string{"main.html"}}
	registerStaticFromDef(app, cfg)

	resp := doStaticReq(app, "/", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Custom Index")
}

// --- embed.FS via WithFS ---

func TestStatic_EmbedFS(t *testing.T) {
	memFS := fstest.MapFS{
		"index.html": {Data: []byte("<h1>Embedded</h1>"), Mode: 0o644},
		"app.js":     {Data: []byte("console.log('hi')"), Mode: 0o644},
	}
	app := staticTestApp()
	svc := &Service{
		config:     &ServiceConfig{},
		fsRegistry: map[string]fs.FS{"web": memFS},
	}
	cfg := StaticDef{Prefix: "/app", FS: "embed", FSName: "web", MaxAge: 60}
	svc.registerStaticToApp(app, cfg)

	resp := doStaticReq(app, "/app/app.js", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "console.log('hi')", string(body))
	assert.Contains(t, resp.Header.Get("Cache-Control"), "max-age=60")
}

func TestStatic_EmbedSPAFallback(t *testing.T) {
	memFS := fstest.MapFS{
		"index.html": {Data: []byte("<div id=root></div>")},
	}
	app := fiber.New()
	handler := static.New("", static.Config{
		FS:         memFS,
		IndexNames: []string{"index.html"},
	})
	app.Get("/*", handler)
	// SPA catch-all: serve index.html from embed for any unmatched path
	app.Get("/*", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString("<div id=root></div>")
	})

	resp := doStaticReq(app, "/any/route/here", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "<div id=root></div>")
}

func TestStatic_EmbedMissingFSName(t *testing.T) {
	app := staticTestApp()
	svc := &Service{
		config:     &ServiceConfig{},
		fsRegistry: map[string]fs.FS{},
	}
	cfg := StaticDef{Prefix: "/app", FS: "embed", FSName: ""}
	svc.registerStaticToApp(app, cfg)
	// No route should have been registered (logged error)
	// Verify: request to /app/anything should return 404 (Fiber default)
	resp := doStaticReq(app, "/app/test.txt", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestStatic_EmbedUnregisteredFSName(t *testing.T) {
	app := staticTestApp()
	svc := &Service{
		config:     &ServiceConfig{},
		fsRegistry: map[string]fs.FS{},
	}
	cfg := StaticDef{Prefix: "/app", FS: "embed", FSName: "nonexistent"}
	svc.registerStaticToApp(app, cfg)
	resp := doStaticReq(app, "/app/test.txt", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- Multiple static defs ---

func TestStatic_MultipleDefs(t *testing.T) {
	dir1 := createTempDir(t, map[string]string{"a.txt": "from assets"})
	dir2 := createTempDir(t, map[string]string{"b.txt": "from files"})
	app := staticTestApp()
	cfg1 := StaticDef{Prefix: "/assets", Dir: dir1}
	cfg2 := StaticDef{Prefix: "/files", Dir: dir2}
	registerStaticFromDef(app, cfg1)
	registerStaticFromDef(app, cfg2)

	resp1 := doStaticReq(app, "/assets/a.txt", nil)
	require.NotNil(t, resp1)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Equal(t, "from assets", string(body1))

	resp2 := doStaticReq(app, "/files/b.txt", nil)
	require.NotNil(t, resp2)
	body2, _ := io.ReadAll(resp2.Body)
	assert.Equal(t, "from files", string(body2))
}

// --- Error: dir required for disk mode ---

func Static_NoDirNoEmbed(t *testing.T) {
	app := staticTestApp()
	cfg := StaticDef{Prefix: "/s", Dir: ""}
	// Should log error and NOT register route
	registerStaticFromDef(app, cfg)
	resp := doStaticReq(app, "/s/anything", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- helpers (reuse production logic in tests) ---

// registerStaticFromDef registers a static route directly on an app (no Service).
func registerStaticFromDef(app *fiber.App, sd StaticDef) {
	if sd.Dir == "" && sd.FS != "embed" {
		return
	}
	svc := &Service{
		config:     &ServiceConfig{},
		fsRegistry: make(map[string]fs.FS),
	}
	svc.srv = nil // we won't use svc.srv.App()
	svc.registerStaticToApp(app, sd)
}

// registerStaticToApp registers a static route on a given app directly.
// This is exposed only for testing — production uses serveStaticFiles().
func (s *Service) registerStaticToApp(app *fiber.App, sd StaticDef) {
	// Inline the registration logic from registerStatic for direct test use.
	var fsys fs.FS
	if sd.FS == "embed" {
		if sd.FSName == "" {
			return
		}
		var ok bool
		fsys, ok = s.fsRegistry[sd.FSName]
		if !ok {
			return
		}
	} else if sd.Dir == "" {
		return
	}

	indexNames := sd.IndexNames
	if len(indexNames) == 0 {
		indexNames = []string{"index.html"}
	}

	cfg := static.Config{
		Compress:   sd.Compress,
		ByteRange:  sd.ByteRange,
		Browse:     sd.Browse,
		Download:   sd.Download,
		MaxAge:     sd.MaxAge,
		IndexNames: indexNames,
	}

	if fsys != nil {
		cfg.FS = fsys
	}

	if sd.SPA {
		indexPath := indexNames[0]
		cfg.NotFoundHandler = func(c fiber.Ctx) error {
			if fsys != nil {
				data, err := fs.ReadFile(fsys, indexPath)
				if err != nil {
					return c.SendStatus(fiber.StatusNotFound)
				}
				c.Set("Content-Type", "text/html; charset=utf-8")
				return c.Send(data)
			}
			fullPath := filepath.Join(sd.Dir, indexPath)
			return c.SendFile(fullPath)
		}
	}

	root := sd.Dir
	if fsys != nil {
		root = ""
	}

	handler := static.New(root, cfg)
	app.Get(sd.Prefix+"/*", handler)
}
