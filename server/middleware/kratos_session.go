package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/ory"
)

// kratosSessionCookie returns the value of the first Kratos session cookie.
// Kratos names it ory_session_<project> (or ory_session in older versions).
func kratosSessionCookie(c fiber.Ctx) string {
	if v := c.Cookies("ory_session"); v != "" {
		return v
	}
	var found string
	for key, value := range c.Request().Header.Cookies() {
		if found == "" && strings.HasPrefix(string(key), "ory_session") {
			found = string(value)
		}
	}
	return found
}

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
			token = c.Get("X-Session-Token")
		}
		if token == "" {
			token = kratosSessionCookie(c)
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
