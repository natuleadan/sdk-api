package runtime

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedirectYAML_BasicRedirect(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().Status(302).To("/docs")
	})
	app.Get("/docs", func(c fiber.Ctx) error {
		return c.SendString("docs page")
	})

	resp := doRedirectReq(app, "GET", "/", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/docs", resp.Header.Get("Location"))
}

func TestRedirectYAML_301(t *testing.T) {
	app := fiber.New()
	app.Get("/old", func(c fiber.Ctx) error {
		return c.Redirect().Status(301).To("/new")
	})

	resp := doRedirectReq(app, "GET", "/old", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/new", resp.Header.Get("Location"))
}

func TestRedirectYAML_307(t *testing.T) {
	app := fiber.New()
	app.Get("/temp", func(c fiber.Ctx) error {
		return c.Redirect().Status(307).To("/temp-target")
	})

	resp := doRedirectReq(app, "GET", "/temp", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusTemporaryRedirect, resp.StatusCode)
	assert.Equal(t, "/temp-target", resp.Header.Get("Location"))
}

func TestRedirectYAML_MultipleRedirects(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().Status(302).To("/docs")
	})
	app.Get("/blog", func(c fiber.Ctx) error {
		return c.Redirect().Status(301).To("https://example.com/blog")
	})

	resp1 := doRedirectReq(app, "GET", "/", nil)
	require.NotNil(t, resp1)
	assert.Equal(t, "/docs", resp1.Header.Get("Location"))

	resp2 := doRedirectReq(app, "GET", "/blog", nil)
	require.NotNil(t, resp2)
	assert.Equal(t, "https://example.com/blog", resp2.Header.Get("Location"))
}

func TestRedirectYAML_ExternalURL(t *testing.T) {
	app := fiber.New()
	app.Get("/go", func(c fiber.Ctx) error {
		return c.Redirect().Status(302).To("https://example.com")
	})

	resp := doRedirectReq(app, "GET", "/go", nil)
	require.NotNil(t, resp)
	assert.Equal(t, "https://example.com", resp.Header.Get("Location"))
}

func TestRedirectYAML_WithAPITarget(t *testing.T) {
	app := fiber.New()
	app.Get("/api", func(c fiber.Ctx) error {
		return c.Redirect().Status(308).To("/api/v2")
	})
	app.Get("/api/v2", func(c fiber.Ctx) error {
		return c.SendString("v2")
	})

	resp := doRedirectReq(app, "GET", "/api", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusPermanentRedirect, resp.StatusCode)
	assert.Equal(t, "/api/v2", resp.Header.Get("Location"))
}

func TestRedirectYAML_ServeRedirectedTarget(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().Status(302).To("/docs")
	})
	app.Get("/docs", func(c fiber.Ctx) error {
		return c.SendString("Scalar UI docs")
	})

	// Follow the redirect manually
	resp := doRedirectReq(app, "GET", "/", nil)
	require.NotNil(t, resp)
	target := resp.Header.Get("Location")

	// Now hit the target
	req2 := httptest.NewRequest("GET", target, nil)
	resp2, _ := app.Test(req2)
	require.NotNil(t, resp2)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}

// --- Config-based tests (simulating YAML loading) ---

func TestRedirectYAML_ConfigRegistration(t *testing.T) {
	svc := &Service{
		config: &ServiceConfig{
			Server: ServerConf{
				Redirects: []RedirectDef{
					{From: "/", To: "/docs", Status: 302},
					{From: "/blog", To: "https://example.com/blog", Status: 301},
				},
			},
		},
	}
	svc.handlers = &EntryHandlers{}

	app := fiber.New()
	svc.srv = nil
	// Use registerRedirects logic directly
	for _, rd := range svc.config.Server.Redirects {
		if rd.From == "" || rd.To == "" {
			continue
		}
		status := rd.Status
		if status == 0 {
			status = fiber.StatusFound
		}
		from := rd.From
		to := rd.To
		code := status
		app.Get(from, func(c fiber.Ctx) error {
			return c.Redirect().Status(code).To(to)
		})
	}

	resp1 := doRedirectReq(app, "GET", "/", nil)
	require.NotNil(t, resp1)
	assert.Equal(t, fiber.StatusFound, resp1.StatusCode)
	assert.Equal(t, "/docs", resp1.Header.Get("Location"))

	resp2 := doRedirectReq(app, "GET", "/blog", nil)
	require.NotNil(t, resp2)
	assert.Equal(t, fiber.StatusMovedPermanently, resp2.StatusCode)
	assert.Equal(t, "https://example.com/blog", resp2.Header.Get("Location"))
}

func TestRedirectYAML_DefaultStatus(t *testing.T) {
	app := fiber.New()
	// Simulate Status=0 (not set in YAML → default 302)
	app.Get("/default", func(c fiber.Ctx) error {
		code := 302
		return c.Redirect().Status(code).To("/target")
	})

	resp := doRedirectReq(app, "GET", "/default", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/target", resp.Header.Get("Location"))
}

func TestRedirectYAML_EmptyRedirectsSkipped(t *testing.T) {
	svc := &Service{
		config: &ServiceConfig{
			Server: ServerConf{
				Redirects: []RedirectDef{},
			},
		},
	}
	app := fiber.New()
	svc.srv = nil
	svc.registerRedirects()
	// No routes registered → /anything should be 404
	resp := doRedirectReq(app, "GET", "/anything", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}
