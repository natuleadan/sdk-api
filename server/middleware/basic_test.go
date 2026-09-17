package middleware

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func basicApp(t *testing.T, validator BasicValidator) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Get("/p", Basic(BasicConfig{Validator: validator}), func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendString(auth.UserID)
	})
	return app
}

func TestBasic_Valid(t *testing.T) {
	app := basicApp(t, func(_ context.Context, user, pass string) (*AuthContext, error) {
		if user == "u" && pass == "p" {
			return &AuthContext{UserID: "u", Roles: []string{"r"}}, nil
		}
		return nil, nil
	})
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestBasic_WrongPassword(t *testing.T) {
	app := basicApp(t, func(context.Context, string, string) (*AuthContext, error) {
		return nil, nil
	})
	req, _ := http.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:wrong")))
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestBasic_MissingAndMalformed(t *testing.T) {
	app := basicApp(t, func(context.Context, string, string) (*AuthContext, error) {
		return &AuthContext{UserID: "u"}, nil
	})
	for _, header := range []string{"", "Bearer x", "Basic !!!", "Basic " + base64.StdEncoding.EncodeToString([]byte("nousercolon"))} {
		req, _ := http.NewRequest("GET", "/p", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "header %q", header)
		assert.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
	}
}
