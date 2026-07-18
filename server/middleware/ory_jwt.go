package middleware

import (
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/ory"
)

// JWTWithOry validates JWT tokens using Ory Kratos's JWKS (RS256).
// Used in "ory" auth mode.
func JWTWithOry(cfg JWTConfig, oClient *ory.Client) fiber.Handler {
	if oClient == nil {
		panic("ory client is required for JWTWithOry")
	}
	if cfg.ContextKey == "" {
		cfg.ContextKey = "claims"
	}

	return func(c fiber.Ctx) error {
		token, rawToken := extractToken(c, cfg.TokenLookup)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing or malformed token",
			})
		}

		claims, err := oClient.ValidateJWT(c.Context(), token, "")
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": fmt.Sprintf("invalid token: %v", err),
			})
		}

		c.Locals(cfg.ContextKey, claims)
		injectAuth(c, buildAuthContext(claims, rawToken))
		if cfg.TokenBlacklist != nil && cfg.TokenBlacklist(rawToken) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "token revoked",
			})
		}
		return c.Next()
	}
}
