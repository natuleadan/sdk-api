package middleware

import (
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/zitadel"
)

// JWTWithZitadel validates JWT tokens using Zitadel's JWKS (RS256).
// Used in "openfga-zitadel" auth mode.
func JWTWithZitadel(cfg JWTConfig, zClient *zitadel.Client) fiber.Handler {
	if zClient == nil {
		panic("zitadel client is required for JWTWithZitadel")
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

		claims, err := zClient.ValidateToken(c.Context(), token)
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

// fiber:context-methods migrated
