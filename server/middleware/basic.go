package middleware

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// BasicValidator resolves HTTP Basic credentials into an AuthContext.
// Return nil (no error) to reject with 403; return an error for 401.
type BasicValidator func(ctx context.Context, user, pass string) (*AuthContext, error)

// BasicConfig configures HTTP Basic authentication (RFC 7617).
type BasicConfig struct {
	// Validator resolves credentials. Required.
	Validator BasicValidator
	// Realm is advertised in the WWW-Authenticate challenge.
	Realm string
}

// Basic validates an Authorization: Basic header via Validator and injects
// the resulting AuthContext for roles, per-user rate limiting and handlers.
func Basic(cfg BasicConfig) fiber.Handler {
	realm := cfg.Realm
	if realm == "" {
		realm = "restricted"
	}
	return func(c fiber.Ctx) error {
		user, pass, ok := parseBasicAuth(c.Get("Authorization"))
		if !ok {
			c.Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing basic credentials",
			})
		}
		if cfg.Validator == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"code":    500,
				"message": "basic validator not configured",
			})
		}
		auth, err := cfg.Validator(c.Context(), user, pass)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": err.Error(),
			})
		}
		if auth == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": "basic credentials not authorized",
			})
		}
		injectAuth(c, auth)
		return c.Next()
	}
}

// parseBasicAuth splits an "Basic base64(user:pass)" header value.
func parseBasicAuth(raw string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(raw, prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	if err != nil {
		return "", "", false
	}
	user, pass, ok = strings.Cut(string(decoded), ":")
	return user, pass, ok && user != ""
}
