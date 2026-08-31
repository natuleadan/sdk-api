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

func TestRedirectYAML_ServeRedirectedTarget(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().Status(302).To("/docs")
	})
	app.Get("/docs", func(c fiber.Ctx) error {
		return c.SendString("Scalar UI docs")
	})

	resp := doRedirectReq(app, "GET", "/", nil)
	require.NotNil(t, resp)
	target := resp.Header.Get("Location")

	req2 := httptest.NewRequest("GET", target, nil)
	resp2, _ := app.Test(req2)
	require.NotNil(t, resp2)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}

func TestRedirectYAML_DefaultStatus(t *testing.T) {
	app := fiber.New()
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
	resp := doRedirectReq(app, "GET", "/anything", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- Wildcard forwarding ---

func TestRedirectYAML_WildcardForward(t *testing.T) {
	app := fiber.New()
	from := "/old/*"
	to := "/new/*"
	handler := buildRedirectHandler(from, to, 301, true, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, from, handler)
	app.Get("/new/:*", func(c fiber.Ctx) error {
		return c.SendString("ok:" + c.Params("*"))
	})

	resp := doRedirectReq(app, "GET", "/old/page1", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/new/page1", resp.Header.Get("Location"))
}

func TestRedirectYAML_WildcardDeepPath(t *testing.T) {
	app := fiber.New()
	handler := buildRedirectHandler("/old/*", "/new/*", 302, true, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, "/old/*", handler)
	app.Get("/new/:*", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp := doRedirectReq(app, "GET", "/old/a/b/c", nil)
	require.NotNil(t, resp)
	assert.Equal(t, "/new/a/b/c", resp.Header.Get("Location"))
}

// --- Param forwarding ---

func TestRedirectYAML_ParamForward(t *testing.T) {
	app := fiber.New()
	from := "/user/:id"
	to := "/profile/:id"
	handler := buildRedirectHandler(from, to, 301, true, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, from, handler)
	app.Get("/profile/:id", func(c fiber.Ctx) error {
		return c.SendString("profile:" + c.Params("id"))
	})

	resp := doRedirectReq(app, "GET", "/user/42", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/profile/42", resp.Header.Get("Location"))
}

func TestRedirectYAML_MultipleParams(t *testing.T) {
	app := fiber.New()
	from := "/org/:org/user/:uid"
	to := "/team/:org/member/:uid"
	handler := buildRedirectHandler(from, to, 301, true, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, from, handler)
	app.Get("/team/:org/member/:uid", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp := doRedirectReq(app, "GET", "/org/acme/user/u1", nil)
	require.NotNil(t, resp)
	assert.Equal(t, "/team/acme/member/u1", resp.Header.Get("Location"))
}

// --- Query string preservation ---

func TestRedirectYAML_PreserveQuery(t *testing.T) {
	app := fiber.New()
	handler := buildRedirectHandler("/old", "/new", 302, true, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, "/old", handler)

	resp := doRedirectReq(app, "GET", "/old?foo=bar&baz=1", nil)
	require.NotNil(t, resp)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "/new?")
	assert.Contains(t, loc, "foo=bar")
	assert.Contains(t, loc, "baz=1")
}

func TestRedirectYAML_NoPreserveQuery(t *testing.T) {
	app := fiber.New()
	preserve := false
	handler := buildRedirectHandler("/old", "/new", 302, preserve, map[string]bool{"GET": true})
	app.Add([]string{"GET"}, "/old", handler)

	resp := doRedirectReq(app, "GET", "/old?foo=bar", nil)
	require.NotNil(t, resp)
	assert.Equal(t, "/new", resp.Header.Get("Location"))
}

// --- Method filter ---

func TestRedirectYAML_MethodFilter(t *testing.T) {
	app := fiber.New()
	methods := map[string]bool{"GET": true, "HEAD": true}
	handler := buildRedirectHandler("/old", "/new", 302, true, methods)
	app.Add([]string{"GET", "HEAD"}, "/old", handler)
	app.Post("/old", func(c fiber.Ctx) error {
		return c.SendString("post-ok")
	})

	// GET → redirect
	respGet := doRedirectReq(app, "GET", "/old", nil)
	require.NotNil(t, respGet)
	assert.Equal(t, "/new", respGet.Header.Get("Location"))

	// POST → passes through (no redirect, handler returns "post-ok")
	respPost := doRedirectReq(app, "POST", "/old", nil)
	require.NotNil(t, respPost)
	assert.Equal(t, fiber.StatusOK, respPost.StatusCode)
}

func TestRedirectYAML_MethodFilterGetAndPost(t *testing.T) {
	app := fiber.New()
	methods := map[string]bool{"GET": true, "POST": true}
	handler := buildRedirectHandler("/old", "/new", 302, true, methods)
	app.Add([]string{"GET", "POST"}, "/old", handler)

	respGet := doRedirectReq(app, "GET", "/old", nil)
	require.NotNil(t, respGet)
	assert.Equal(t, "/new", respGet.Header.Get("Location"))

	respPost := doRedirectReq(app, "POST", "/old", nil)
	require.NotNil(t, respPost)
	assert.Equal(t, "/new", respPost.Header.Get("Location"))
}

// --- extractParams ---

func TestExtractParams(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
	}{
		{"/user/:id", []string{"id"}},
		{"/org/:org/user/:uid", []string{"org", "uid"}},
		{"/static", nil},
		{"/:a/:b/:c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := extractParams(tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}
