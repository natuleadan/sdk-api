package middleware

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sessionStore(t *testing.T) *redis.Redis {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.New(mr.Addr())
}

func sessionApp(store *redis.Redis) *fiber.App {
	app := fiber.New()
	app.Get("/p", Session(SessionConfig{Cookie: "sid", Store: store, TTL: time.Hour}), func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendString(auth.UserID)
	})
	return app
}

func TestSession_Roundtrip(t *testing.T) {
	ctx := context.Background()
	store := sessionStore(t)
	id, err := CreateSession(ctx, store, time.Hour, "user-7", []string{"admin"})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	app := sessionApp(store)
	req, _ := http.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: id})
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, DestroySession(ctx, store, id))
	req2, _ := http.NewRequest("GET", "/p", nil)
	req2.AddCookie(&http.Cookie{Name: "sid", Value: id})
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
}

func TestSession_MissingAndUnknown(t *testing.T) {
	store := sessionStore(t)
	app := sessionApp(store)

	req, _ := http.NewRequest("GET", "/p", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req2, _ := http.NewRequest("GET", "/p", nil)
	req2.AddCookie(&http.Cookie{Name: "sid", Value: "nope"})
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
}

func TestSession_NilStore(t *testing.T) {
	ctx := context.Background()
	if _, err := CreateSession(ctx, nil, time.Hour, "u", nil); err == nil {
		t.Error("expected error with nil store, got nil")
	}
	app := sessionApp(nil)
	req, _ := http.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "x"})
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
