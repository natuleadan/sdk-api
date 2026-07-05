package middleware

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/server/auth/openfga"
)

// OpenFGAConfig defines the configuration for OpenFGA authorization middleware.
type OpenFGAConfig struct {
	Client      openfga.Checker // interface (supports caching)
	Relation    string          // e.g., "can_read", "can_write", "can_delete"
	Object      string          // e.g., "product:123", "order:456"
	Roles       []string        // YAML-defined roles to check
	Permissions []string        // YAML-defined permissions to check
}

// OpenFGA creates a middleware that checks authorization against OpenFGA.
// It requires AuthContext to be present (set by JWT middleware).
func OpenFGA(cfg OpenFGAConfig) fiber.Handler {
	if cfg.Client == nil {
		panic("openfga middleware: client is required")
	}

	return func(c fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "auth context required",
			})
		}

		user := buildUserIdentifier(auth)

		authorized, err := checkOpenFGARoles(c, cfg.Client, user, auth, cfg.Roles)
		if err != nil || !authorized {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": "insufficient roles",
			})
		}

		authorized, err = checkOpenFGAPermissions(c, cfg.Client, user, cfg.Permissions)
		if err != nil || !authorized {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": "insufficient permissions",
			})
		}

		if err := checkOpenFGARelation(c, cfg, user); err != nil {
			return err
		}

		return c.Next()
	}
}

func checkOpenFGARoles(c fiber.Ctx, client openfga.Checker, user string, auth *AuthContext, roles []string) (bool, error) {
	if len(roles) == 0 {
		return true, nil
	}
	if hasAnyRole(auth, roles) {
		return true, nil
	}
	return checkRolesViaFGA(c, client, user, roles)
}

func checkOpenFGAPermissions(c fiber.Ctx, client openfga.Checker, user string, permissions []string) (bool, error) {
	if len(permissions) == 0 {
		return true, nil
	}
	return checkPermissionsViaFGA(c, client, user, permissions)
}

func checkOpenFGARelation(c fiber.Ctx, cfg OpenFGAConfig, user string) error {
	if cfg.Object == "" && cfg.Relation == "" {
		return nil
	}
	object := cfg.Object
	if object == "" {
		if id := c.Params("id"); id != "" {
			object = fmt.Sprintf("%s:%s", c.Route().Name, id)
		}
	}
	if object == "" || cfg.Relation == "" {
		return nil
	}
	allowed, err := cfg.Client.Check(c.Context(), openfga.CheckRequest{
		User:     user,
		Relation: cfg.Relation,
		Object:   object,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "authorization check failed")
	}
	if !allowed {
		return fiber.NewError(fiber.StatusForbidden, "access denied")
	}
	return nil
}

// buildUserIdentifier constructs the user identifier for OpenFGA.
func buildUserIdentifier(auth *AuthContext) string {
	if auth.OrgID != "" {
		return fmt.Sprintf("user:%s:%s", auth.OrgID, auth.UserID)
	}
	return fmt.Sprintf("user:%s", auth.UserID)
}

func hasAnyRole(auth *AuthContext, requiredRoles []string) bool {
	for _, required := range requiredRoles {
		if slices.Contains(auth.Roles, required) {
			return true
		}
	}
	return false
}

func checkRolesViaFGA(c fiber.Ctx, client openfga.Checker, user string, roles []string) (bool, error) {
	for _, role := range roles {
		allowed, err := client.Check(c.Context(), openfga.CheckRequest{
			User:     user,
			Relation: fmt.Sprintf("role:%s", role),
			Object:   "role-assignment",
		})
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func checkPermissionsViaFGA(c fiber.Ctx, client openfga.Checker, user string, permissions []string) (bool, error) {
	for _, perm := range permissions {
		parts := strings.SplitN(perm, ":", 2)
		if len(parts) != 2 {
			continue
		}
		allowed, err := client.Check(c.Context(), openfga.CheckRequest{
			User:     user,
			Relation: fmt.Sprintf("can_%s", parts[1]),
			Object:   fmt.Sprintf("%s:%s", parts[0], parts[1]),
		})
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func ParseObject(object string) (objType, objID string, err error) {
	parts := strings.SplitN(object, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid object format: %s (expected type:id)", object)
	}
	return parts[0], parts[1], nil
}

// fiber:context-methods migrated
