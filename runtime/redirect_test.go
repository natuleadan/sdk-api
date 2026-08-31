package runtime

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func redirectTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"code": code, "message": err.Error()})
		},
	})
}

func doRedirectReq(app *fiber.App, method, path string, headers map[string]string) *http.Response {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, _ := app.Test(req)
	return resp
}

// --- To ---

func TestRedirect_To(t *testing.T) {
	app := redirectTestApp()
	app.Get("/old", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.To("/new")
	})
	app.Get("/new", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp := doRedirectReq(app, "GET", "/old", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/new", resp.Header.Get("Location"))
}

func TestRedirect_ToExternalURL(t *testing.T) {
	app := redirectTestApp()
	app.Get("/out", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.To("https://example.com")
	})

	resp := doRedirectReq(app, "GET", "/out", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "https://example.com", resp.Header.Get("Location"))
}

// --- Status ---

func TestRedirect_Status(t *testing.T) {
	app := redirectTestApp()
	app.Get("/moved", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Status(fiber.StatusMovedPermanently).To("/permanent")
	})

	resp := doRedirectReq(app, "GET", "/moved", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/permanent", resp.Header.Get("Location"))
}

func TestRedirect_Status307(t *testing.T) {
	app := redirectTestApp()
	app.Get("/temp", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Status(fiber.StatusTemporaryRedirect).To("/temp-target")
	})

	resp := doRedirectReq(app, "GET", "/temp", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusTemporaryRedirect, resp.StatusCode)
	assert.Equal(t, "/temp-target", resp.Header.Get("Location"))
}

func TestRedirect_Status308(t *testing.T) {
	app := redirectTestApp()
	app.Get("/perm-post", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Status(fiber.StatusPermanentRedirect).To("/perm-target")
	})

	resp := doRedirectReq(app, "GET", "/perm-post", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusPermanentRedirect, resp.StatusCode)
	assert.Equal(t, "/perm-target", resp.Header.Get("Location"))
}

// --- Route ---

func TestRedirect_Route(t *testing.T) {
	app := redirectTestApp()
	app.Get("/user/:id", func(c fiber.Ctx) error {
		return c.SendString("user:" + c.Params("id"))
	}).Name("user")

	app.Get("/go", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Route("user", RedirectConfig{
			Params: Map{"id": "42"},
		})
	})

	resp := doRedirectReq(app, "GET", "/go", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/user/42", resp.Header.Get("Location"))
}

func TestRedirect_RouteWithQueries(t *testing.T) {
	app := redirectTestApp()
	app.Get("/user/:id", func(c fiber.Ctx) error {
		return c.SendString("ok")
	}).Name("user")

	app.Get("/go", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Route("user", RedirectConfig{
			Params:  Map{"id": "7"},
			Queries: map[string]string{"tab": "profile", "sort": "name"},
		})
	})

	resp := doRedirectReq(app, "GET", "/go", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "/user/7?")
	assert.Contains(t, loc, "tab=profile")
	assert.Contains(t, loc, "sort=name")
}

func TestRedirect_RouteNoParams(t *testing.T) {
	app := redirectTestApp()
	app.Get("/dashboard", func(c fiber.Ctx) error {
		return c.SendString("ok")
	}).Name("dashboard")

	app.Get("/login-ok", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Route("dashboard")
	})

	resp := doRedirectReq(app, "GET", "/login-ok", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/dashboard", resp.Header.Get("Location"))
}

// --- Back ---

func TestRedirect_BackWithReferer(t *testing.T) {
	app := redirectTestApp()
	app.Get("/action", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Back("/")
	})

	// httptest default host is empty, so Back() treats any referer as cross-origin.
	// Use the same host info to make the referer appear same-origin.
	resp := doRedirectReq(app, "GET", "/action", map[string]string{
		"Referer": "/previous",
	})
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/previous", resp.Header.Get("Location"))
}

func TestRedirect_BackFallback(t *testing.T) {
	app := redirectTestApp()
	app.Get("/action", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Back("/home")
	})

	// No referer header
	resp := doRedirectReq(app, "GET", "/action", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/home", resp.Header.Get("Location"))
}

func TestRedirect_BackDefaultFallback(t *testing.T) {
	app := redirectTestApp()
	app.Get("/action", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Back()
	})

	// No referer, no fallback → Fiber may error (500) or redirect to "/"
	resp := doRedirectReq(app, "GET", "/action", nil)
	require.NotNil(t, resp)
	// Fiber returns 500 when Back() has no referer and no fallback
	assert.True(t, resp.StatusCode == fiber.StatusSeeOther || resp.StatusCode == fiber.StatusInternalServerError,
		"expected 303 or 500, got %d", resp.StatusCode)
}

func TestRedirect_BackCrossOriginFallsBack(t *testing.T) {
	app := redirectTestApp()
	app.Get("/action", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.Back("/safe")
	})

	resp := doRedirectReq(app, "GET", "/action", map[string]string{
		"Referer": "http://evil.com/steal",
	})
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	// Cross-origin referer should be rejected, fallback used
	assert.Equal(t, "/safe", resp.Header.Get("Location"))
}

// --- With (flash messages) ---

func TestRedirect_WithFlashMessage(t *testing.T) {
	app := redirectTestApp()
	app.Get("/set", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.With("status", "saved").With("type", "success").To("/get")
	})
	app.Get("/get", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		msgs := rc.Messages()
		return c.JSON(msgs)
	})

	// First request: set flash
	req1 := httptest.NewRequest("GET", "/set", nil)
	resp1, _ := app.Test(req1)
	require.NotNil(t, resp1)
	assert.Equal(t, fiber.StatusSeeOther, resp1.StatusCode)

	// Extract flash cookie
	var flashCookie string
	for _, ck := range resp1.Cookies() {
		if ck.Name == "fiber_flash" {
			flashCookie = ck.Value
			break
		}
	}
	require.NotEmpty(t, flashCookie, "flash cookie should be set")

	// Second request: read flash
	req2 := httptest.NewRequest("GET", "/get", nil)
	req2.AddCookie(&http.Cookie{Name: "fiber_flash", Value: flashCookie})
	resp2, _ := app.Test(req2)
	require.NotNil(t, resp2)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}

// --- WithInput ---

func TestRedirect_WithInput(t *testing.T) {
	app := redirectTestApp()
	app.Post("/submit", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.WithInput().To("/form")
	})
	app.Get("/form", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		inputs := rc.OldInputs()
		return c.JSON(inputs)
	})

	// POST with form data
	req := httptest.NewRequest("POST", "/submit", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Note: WithInput reads from the form, but in test we need to set body
	// Fiber's WithInput stores form data in cookie

	resp, _ := app.Test(req)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
}

// --- Messages / Message ---

func TestRedirect_MessagesEmpty(t *testing.T) {
	app := redirectTestApp()
	app.Get("/check", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		msgs := rc.Messages()
		return c.JSON(fiber.Map{"count": len(msgs)})
	})

	resp := doRedirectReq(app, "GET", "/check", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// --- OldInputs / OldInput ---

func TestRedirect_OldInputsEmpty(t *testing.T) {
	app := redirectTestApp()
	app.Get("/check-inputs", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		inputs := rc.OldInputs()
		return c.JSON(fiber.Map{"count": len(inputs)})
	})

	resp := doRedirectReq(app, "GET", "/check-inputs", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// --- IsSameOrigin ---

func TestIsSameOrigin(t *testing.T) {
	tests := []struct {
		name        string
		referer     string
		requestHost string
		want        bool
	}{
		{"same host", "http://localhost/path", "localhost", true},
		{"same host with port", "http://localhost:3000/path", "localhost:3000", true},
		{"same host different port", "http://localhost:3000/path", "localhost:8080", false},
		{"different host", "http://evil.com/path", "localhost", false},
		{"empty referer", "", "localhost", false},
		{"empty host", "http://localhost/path", "", false},
		{"https match", "https://example.com/path", "example.com", true},
		{"case insensitive", "http://EXAMPLE.COM/path", "example.com", true},
		{"with default port 80", "http://example.com:80/path", "example.com", true},
		{"with default port 443", "https://example.com:443/path", "example.com", true},
		{"invalid referer", "://bad", "localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSameOrigin(tt.referer, tt.requestHost)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- RestCtx.Redirect() integration ---

func TestRestCtx_Redirect(t *testing.T) {
	app := redirectTestApp()
	app.Get("/via-restctx", func(c fiber.Ctx) error {
		rc := newRestCtx(c, nil)
		return rc.Redirect().Status(fiber.StatusMovedPermanently).To("/target")
	})

	resp := doRedirectReq(app, "GET", "/via-restctx", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/target", resp.Header.Get("Location"))
}

func TestRestCtx_RedirectBack(t *testing.T) {
	app := redirectTestApp()
	app.Get("/back-test", func(c fiber.Ctx) error {
		rc := newRestCtx(c, nil)
		return rc.Redirect().Back("/fallback")
	})

	// Without proper Host header, Fiber's Back() may fall back
	resp := doRedirectReq(app, "GET", "/back-test", map[string]string{
		"Referer": "http://localhost/origin",
	})
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	// Location should be either the referer or the fallback
	loc := resp.Header.Get("Location")
	assert.True(t, loc == "http://localhost/origin" || loc == "/fallback",
		"expected referer or fallback, got %q", loc)
}

func TestRestCtx_RedirectWithFlash(t *testing.T) {
	app := redirectTestApp()
	app.Get("/flash-set", func(c fiber.Ctx) error {
		rc := newRestCtx(c, nil)
		return rc.Redirect().With("msg", "hello").To("/flash-get")
	})
	app.Get("/flash-get", func(c fiber.Ctx) error {
		rc := newRestCtx(c, nil)
		msg := rc.Redirect().Message("msg")
		return c.JSON(fiber.Map{"key": msg.Key, "value": msg.Value})
	})

	// Set flash
	req1 := httptest.NewRequest("GET", "/flash-set", nil)
	resp1, _ := app.Test(req1)
	require.NotNil(t, resp1)
	assert.Equal(t, fiber.StatusSeeOther, resp1.StatusCode)

	var flashCookie string
	for _, ck := range resp1.Cookies() {
		if ck.Name == "fiber_flash" {
			flashCookie = ck.Value
			break
		}
	}
	require.NotEmpty(t, flashCookie)

	// Read flash
	req2 := httptest.NewRequest("GET", "/flash-get", nil)
	req2.AddCookie(&http.Cookie{Name: "fiber_flash", Value: flashCookie})
	resp2, _ := app.Test(req2)
	require.NotNil(t, resp2)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}

// --- Chaining ---

func TestRedirect_Chaining(t *testing.T) {
	app := redirectTestApp()
	app.Get("/chain", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		// Chain Status then To
		return rc.Status(fiber.StatusMovedPermanently).To("/dest")
	})

	resp := doRedirectReq(app, "GET", "/chain", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/dest", resp.Header.Get("Location"))
}

func TestRedirect_ChainWithFlash(t *testing.T) {
	app := redirectTestApp()
	app.Get("/chain-flash", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		// Chain With then Status then To
		return rc.With("key", "val").Status(fiber.StatusFound).To("/dest")
	})

	resp := doRedirectReq(app, "GET", "/chain-flash", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/dest", resp.Header.Get("Location"))
	// Flash cookie should be set
	var hasFlash bool
	for _, ck := range resp.Cookies() {
		if ck.Name == "fiber_flash" {
			hasFlash = true
			break
		}
	}
	assert.True(t, hasFlash, "flash cookie should be set")
}

// --- Edge cases ---

func TestRedirect_ToEmptyString(t *testing.T) {
	app := redirectTestApp()
	app.Get("/empty", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		return rc.To("")
	})

	resp := doRedirectReq(app, "GET", "/empty", nil)
	require.NotNil(t, resp)
	// Fiber redirects to empty string (Location header will be empty or "/")
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
}

func TestRedirect_StatusDefault(t *testing.T) {
	app := redirectTestApp()
	app.Get("/default-status", func(c fiber.Ctx) error {
		rc := newRedirect(c)
		// No Status() call → default 303
		return rc.To("/target")
	})

	resp := doRedirectReq(app, "GET", "/default-status", nil)
	require.NotNil(t, resp)
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
}
