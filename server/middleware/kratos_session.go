package middleware

import (
	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/ory"
)

// KratosSessionConfig configures Ory Kratos session validation.
type KratosSessionConfig struct {
	// Client is the Ory client (Kratos + Keto).
	Client *ory.Client
	// ContextKey is the Locals key for the raw session (default "session").
	ContextKey string
}

// KratosSession validates an Ory Kratos session (Bearer token or cookie) via
// /sessions/whoami and injects an AuthContext. Roles are resolved downstream
// by the Keto authorization middleware; identity comes from Kratos.
func KratosSession(cfg KratosSessionConfig) fiber.Handler {
	if cfg.Client == nil {
		panic("kratos session middleware: client is required")
	}
	if cfg.ContextKey == "" {
		cfg.ContextKey = "session"
	}
	return func(c fiber.Ctx) error {
		token, _ := extractToken(c, "header:Authorization")
		if token == "" {
			token = c.Cookies("ory_session")
		}
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing session",
			})
		}

		session, err := cfg.Client.ValidateSession(c.Context(), token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "invalid or expired session",
			})
		}

		c.Locals(cfg.ContextKey, session)
		injectAuth(c, &AuthContext{
			UserID:   session.Identity.ID,
			OrgID:    session.OrgID(),
			Roles:    session.Roles(),
			RawToken: token,
		})
		return c.Next()
	}
}

// fiber:context-methods migrated
