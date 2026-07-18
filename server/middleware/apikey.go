// Package middleware provides Fiber HTTP middlewares for auth, security, validation, rate limiting, and more.
package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/openfga"
)

// APIKeyConfig configures API key authentication for an entry.
type APIKeyConfig struct {
	// Prefix identifies API keys (e.g., "sk-"). Empty means no prefix check.
	Prefix string
	// Client is the OpenFGA checker for authorization checks.
	Client openfga.Checker
	// Relation is the required relation (e.g., "can_access", "can_write").
	Relation string
	// Object is the resource object (e.g., "webhook:stripe").
	Object string
	// Header is the header to look for the API key (default: "Authorization").
	Header string
	// AuthResolver resolves an API key into an AuthContext for role-based auth.
	// When nil and no FGA client, only presence + prefix are validated.
	AuthResolver func(ctx context.Context, key string) (*AuthContext, error)
}

// APIKey creates a middleware that validates API keys against OpenFGA.
// The API key is treated as a subject in OpenFGA (apikey:<key_id>).
func APIKey(cfg APIKeyConfig) fiber.Handler {
	if cfg.Header == "" {
		cfg.Header = "Authorization"
	}
	if cfg.Relation == "" {
		cfg.Relation = "can_access"
	}

	return func(c fiber.Ctx) error {
		raw := c.Get(cfg.Header)
		if raw == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing API key",
			})
		}

		// Strip "Bearer " prefix if present
		key := strings.TrimPrefix(raw, "Bearer ")

		// Check prefix if configured
		if cfg.Prefix != "" && !strings.HasPrefix(key, cfg.Prefix) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "invalid API key format",
			})
		}

		// Try AuthResolver first (manual driver)
		if cfg.AuthResolver != nil {
			auth, err := cfg.AuthResolver(c.Context(), key)
			if err != nil {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"code":    401,
					"message": err.Error(),
				})
			}
			if auth == nil {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"code":    403,
					"message": "API key not authorized",
				})
			}
			injectAuth(c, auth)
			return c.Next()
		}

		// Try OpenFGA Check
		if cfg.Client != nil {
			keyID := deriveKeyID(key)
			subject := fmt.Sprintf("apikey:%s", keyID)
			allowed, err := cfg.Client.Check(c.Context(), openfga.CheckRequest{
				User:     subject,
				Relation: cfg.Relation,
				Object:   cfg.Object,
			})
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"code":    500,
					"message": "authorization check failed",
				})
			}
			if !allowed {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"code":    403,
					"message": "API key not authorized",
				})
			}
			return c.Next()
		}

		return c.Next()
	}
}

// deriveKeyID creates a safe identifier from an API key.
func deriveKeyID(key string) string {
	if len(key) > 12 {
		key = key[:12]
	}
	// Replace non-alphanumeric characters
	var clean strings.Builder
	for _, c := range key {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			clean.WriteRune(c)
		}
	}
	return clean.String()
}

// fiber:context-methods migrated
