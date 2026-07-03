package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// TokenRefreshConfig configures the token refresh endpoint behavior.
type TokenRefreshConfig struct {
	// RefreshTokenTTL is how long the refresh token is valid (manual mode).
	RefreshTokenTTL time.Duration
	// JWTSecret used to sign new tokens (manual mode).
	JWTSecret string
	// ZitadelTokenURL is the Zitadel token endpoint URL (openfga-zitadel mode).
	ZitadelTokenURL string
	// ZitadelClientID is the Zitadel OAuth2 client ID.
	ZitadelClientID string
	// KratosRefreshURL is the Kratos session refresh URL (ory mode).
	KratosRefreshURL string
}

// TokenRefreshHandler returns a handler that delegates token refresh to the
// configured identity provider, or re-signs the JWT in manual mode.
func TokenRefreshHandler(cfg TokenRefreshConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code":    400,
				"message": "invalid request body",
			})
		}
		if req.RefreshToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code":    400,
				"message": "refresh_token is required",
			})
		}

		switch {
		case cfg.ZitadelTokenURL != "":
			return zitadelTokenRefresh(c, cfg, req.RefreshToken)
		case cfg.KratosRefreshURL != "":
			return kratosTokenRefresh(c, cfg, req.RefreshToken)
		default:
			return manualTokenRefresh(c, cfg, req.RefreshToken)
		}
	}
}

func zitadelTokenRefresh(c *fiber.Ctx, cfg TokenRefreshConfig, refreshToken string) error {
	body := fmt.Sprintf(
		"grant_type=refresh_token&refresh_token=%s&client_id=%s",
		refreshToken, cfg.ZitadelClientID,
	)
	req, err := http.NewRequestWithContext(c.UserContext(), http.MethodPost,
		cfg.ZitadelTokenURL, strings.NewReader(body))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "internal error"})
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "token refresh failed"})
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "read failed"})
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "parse failed"})
	}
	return c.Status(resp.StatusCode).JSON(result)
}

func kratosTokenRefresh(c *fiber.Ctx, cfg TokenRefreshConfig, refreshToken string) error {
	body, err := json.Marshal(map[string]string{"session_token": refreshToken})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "internal error"})
	}
	req, err := http.NewRequestWithContext(c.UserContext(), http.MethodPost,
		cfg.KratosRefreshURL, bytes.NewReader(body))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "internal error"})
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "token refresh failed"})
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "read failed"})
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return c.Status(502).JSON(fiber.Map{"code": 502, "message": "parse failed"})
	}
	return c.Status(resp.StatusCode).JSON(result)
}

func manualTokenRefresh(c *fiber.Ctx, cfg TokenRefreshConfig, _ string) error {
	auth := GetAuth(c)
	if auth == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    401,
			"message": "authentication required",
		})
	}

	ttl := cfg.RefreshTokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	claims := jwt.MapClaims{
		"sub":         auth.UserID,
		"org_id":      auth.OrgID,
		"roles":       auth.Roles,
		"permissions": auth.Permissions,
		"iat":         time.Now().Unix(),
		"exp":         time.Now().Add(ttl).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "signing failed"})
	}

	return c.JSON(fiber.Map{
		"access_token":  signed,
		"token_type":    "Bearer",
		"expires_in":    int(ttl.Seconds()),
	})
}
