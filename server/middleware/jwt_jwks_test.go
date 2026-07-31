package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startJWKSServer(t *testing.T, priv *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	pub := &priv.PublicKey
	jwksDoc := map[string]any{
		"keys": []map[string]any{
			{
				"kid": kid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksDoc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)
	return signed
}

func TestJWTWithJWKS_ValidToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := startJWKSServer(t, priv, "key-1")

	app := fiber.New()
	app.Get("/protected", JWT(JWTConfig{JWKSURL: srv.URL, Algorithm: "RS256"}), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"sub": c.Locals("claims").(jwt.MapClaims)["sub"]})
	})

	token := signRS256(t, priv, "key-1", map[string]any{"sub": "user-rs256"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestJWTWithJWKS_WrongKid(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := startJWKSServer(t, priv, "key-1")

	app := fiber.New()
	app.Get("/protected", JWT(JWTConfig{JWKSURL: srv.URL, Algorithm: "RS256"}), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	token := signRS256(t, priv, "unknown-kid", map[string]any{"sub": "x"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestJWTWithJWKS_WrongSignature(t *testing.T) {
	priv1, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	priv2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	_ = priv2
	srv := startJWKSServer(t, priv1, "key-1")

	app := fiber.New()
	app.Get("/protected", JWT(JWTConfig{JWKSURL: srv.URL, Algorithm: "RS256"}), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// Token signed with a different key but same kid
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": "x"})
	tok.Header["kid"] = "key-1"
	otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	token, err := tok.SignedString(otherPriv)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestJWTWithJWKS_MissingToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := startJWKSServer(t, priv, "key-1")

	app := fiber.New()
	app.Get("/protected", JWT(JWTConfig{JWKSURL: srv.URL, Algorithm: "RS256"}), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
