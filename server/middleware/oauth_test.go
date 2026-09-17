package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func introspectStub(t *testing.T, hits *int64, active bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if active {
			w.Write([]byte(`{"active":true,"sub":"user-1","scope":"read write"}`))
			return
		}
		w.Write([]byte(`{"active":false}`))
	}))
}

func oauthApp(cfg OAuthConfig) *fiber.App {
	app := fiber.New()
	app.Get("/p", Introspect(cfg), func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendString(auth.UserID + "|" + strings.Join(auth.Roles, ","))
	})
	return app
}

func bearerReq(t *testing.T, app *fiber.App, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", "/p", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp
}

func TestOAuth_Active(t *testing.T) {
	var hits int64
	srv := introspectStub(t, &hits, true)
	defer srv.Close()

	app := oauthApp(OAuthConfig{IntrospectionURL: srv.URL, ClientID: "c", ClientSecret: "s", CacheTTL: time.Minute})
	resp := bearerReq(t, app, "tok-1")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "user-1|read,write", string(body))
}

func TestOAuth_InactiveAndMissing(t *testing.T) {
	var hits int64
	srv := introspectStub(t, &hits, false)
	defer srv.Close()

	app := oauthApp(OAuthConfig{IntrospectionURL: srv.URL})
	resp := bearerReq(t, app, "tok-bad")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = bearerReq(t, app, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestOAuth_Cache(t *testing.T) {
	var hits int64
	srv := introspectStub(t, &hits, true)
	defer srv.Close()

	app := oauthApp(OAuthConfig{IntrospectionURL: srv.URL, CacheTTL: time.Minute})
	bearerReq(t, app, "tok-cache")
	bearerReq(t, app, "tok-cache")
	assert.Equal(t, int64(1), atomic.LoadInt64(&hits), "second call must hit cache")
}

func TestOAuth_NoURL(t *testing.T) {
	app := oauthApp(OAuthConfig{})
	resp := bearerReq(t, app, "tok")
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
