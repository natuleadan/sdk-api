package middleware

import (
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/ory"
)

// OryConfig defines the configuration for Ory authorization middleware.
type OryConfig struct {
	Client      *ory.Client
	Roles       []string // YAML-defined roles to check
	Permissions []string // YAML-defined permissions to check
}

// Ory creates a middleware that checks authorization via Ory Keto.
// It requires AuthContext to be present (set by JWT middleware).
func Ory(cfg OryConfig) fiber.Handler {
	if cfg.Client == nil {
		panic("ory middleware: client is required")
	}

	return func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "auth context required",
			})
		}

		if len(cfg.Roles) > 0 {
			authorized, err := checkRolesViaKeto(c, cfg.Client, auth, cfg.Roles)
			if err != nil || !authorized {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"code":    403,
					"message": "insufficient roles",
				})
			}
		}

		if len(cfg.Permissions) > 0 {
			authorized, err := checkPermissionsViaKeto(c, cfg.Client, auth, cfg.Permissions)
			if err != nil || !authorized {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"code":    403,
					"message": "insufficient permissions",
				})
			}
		}

		return c.Next()
	}
}

func checkRolesViaKeto(c fiber.Ctx, client *ory.Client, auth *AuthContext, roles []string) (bool, error) {
	for _, role := range roles {
		allowed, err := client.KetoCheck(c.Context(), ory.KetoCheckRequest{
			Namespace: "roles",
			Object:    role,
			Relation:  "assignee",
			SubjectID: auth.UserID,
		})
		if err != nil {
			return false, fmt.Errorf("ory keto role check: %w", err)
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func checkPermissionsViaKeto(c fiber.Ctx, client *ory.Client, auth *AuthContext, permissions []string) (bool, error) {
	for _, perm := range permissions {
		parts := splitPermission(perm)
		if parts == nil {
			continue
		}
		allowed, err := client.KetoCheck(c.Context(), ory.KetoCheckRequest{
			Namespace: parts[0],
			Object:    parts[1],
			Relation:  "perform",
			SubjectID: auth.UserID,
		})
		if err != nil {
			return false, fmt.Errorf("ory keto permission check: %w", err)
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func splitPermission(perm string) []string {
	for i := 0; i < len(perm); i++ {
		if perm[i] == ':' {
			return []string{perm[:i], perm[i+1:]}
		}
	}
	return nil
}

// fiber:context-methods migrated
