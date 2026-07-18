package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/stretchr/testify/suite"
)

type MiddlewareSuite struct {
	suite.Suite
	app *fiber.App
}

func (s *MiddlewareSuite) SetupTest() {
	logx.Disable()
	s.app = fiber.New()
}

func (s *MiddlewareSuite) TearDownTest() {
	s.app = nil
}

func (s *MiddlewareSuite) TestTraceSkipPath() {
	s.app.Use(Trace(TraceConfig{
		SkipPaths: []string{"/health"},
	}))
	s.app.Get("/health", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/health", nil)
	resp, _ := s.app.Test(req)
	s.Equal(http.StatusOK, resp.StatusCode)
}

func (s *MiddlewareSuite) TestTraceResponseHeader() {
	s.app.Use(Trace(TraceConfig{
		TraceResponseHeader: "X-Trace-Id",
	}))
	s.app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := s.app.Test(req)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.NotEmpty(resp.Header.Get("X-Trace-Id"))
}

func (s *MiddlewareSuite) TestIsClientError() {
	s.True(isClientError(400))
	s.True(isClientError(401))
	s.True(isClientError(404))
	s.False(isClientError(500))
	s.False(isClientError(0))
}

func (s *MiddlewareSuite) TestIsJSONContentType() {
	s.app.Post("/json-check", func(c fiber.Ctx) error {
		if isJSONContentType(c) {
			return c.SendString("json")
		}
		return c.SendString("not-json")
	})

	req := testRequest(context.Background(), "POST", "/json-check", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	s.Equal("json", string(body))

	req2 := testRequest(context.Background(), "POST", "/json-check", nil)
	req2.Header.Set("Content-Type", "text/plain")
	resp2, _ := s.app.Test(req2)
	body2, _ := io.ReadAll(resp2.Body)
	s.Equal("not-json", string(body2))
}

func (s *MiddlewareSuite) TestDefaultJWTConfig() {
	cfg := DefaultJWTConfig()
	s.Equal("HS256", cfg.Algorithm)
	s.Empty(cfg.Secret)
	s.Empty(cfg.ContextKey)
	s.Equal("header:Authorization", cfg.TokenLookup)
}

func (s *MiddlewareSuite) TestGenerateNonce() {
	n1 := GenerateNonce()
	n2 := GenerateNonce()
	s.NotEmpty(n1)
	s.NotEmpty(n2)
	s.NotEqual(n1, n2)
	s.GreaterOrEqual(len(n1), 40)
}

func (s *MiddlewareSuite) TestParseRSAPublicKey() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	s.Require().NoError(err)

	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	parsed := parseRSAPublicKey(pemData)
	s.NotNil(parsed)
	s.Equal(key.N, parsed.N)

	// Invalid PEM returns nil
	s.Nil(parseRSAPublicKey([]byte("invalid")))
}

func (s *MiddlewareSuite) TestParseECDSAPublicKey() {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.Require().NoError(err)

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	s.Require().NoError(err)

	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	parsed := parseECDSAPublicKey(pemData)
	s.NotNil(parsed)

	// Invalid PEM returns nil
	s.Nil(parseECDSAPublicKey([]byte("invalid")))
}

func (s *MiddlewareSuite) TestSignToken_HS256() {
	token, err := SignToken("test-secret", "HS256", jwt.MapClaims{"sub": "user1"})
	s.Require().NoError(err)
	s.NotEmpty(token)

	parser := newParser(JWTConfig{Secret: "test-secret", Algorithm: "HS256"})
	parsed, err := parser.parse(token)
	s.Require().NoError(err)
	sub, _ := parsed.GetSubject()
	s.Equal("user1", sub)
}

func (s *MiddlewareSuite) TestSignToken_RS256() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)

	privBytes := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})

	token, err := SignToken(string(privPEM), "RS256", jwt.MapClaims{"sub": "user2"})
	s.Require().NoError(err)
	s.NotEmpty(token)
}

func (s *MiddlewareSuite) TestGunzip() {
	s.app.Use(Gunzip())
	s.app.Post("/gunzip", func(c fiber.Ctx) error {
		body := string(c.Body())
		return c.SendString(body)
	})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(`{"compressed":true}`))
	gz.Close()

	req := testRequest(context.Background(), "POST", "/gunzip", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := s.app.Test(req)
	s.Equal(200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	s.Contains(string(body), "compressed")
}

func TestMiddlewareSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareSuite))
}
