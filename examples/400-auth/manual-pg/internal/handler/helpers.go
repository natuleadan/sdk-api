package handler

import (
	"errors"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

var roleHierarchy = auth.RoleHierarchy{
	"viewer": {},
	"editor": {"viewer"},
	"admin":  {"editor", "viewer"},
}

func ValidateJWT(a *middleware.AuthContext, requiredRoles, requiredPermissions []string) error {
	if len(requiredRoles) > 0 {
		allowed := false
		for _, r := range a.Roles {
			for _, req := range requiredRoles {
				if r == req || roleHierarchy.Inherits(r, req) {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return errors.New("insufficient role")
		}
	}
	if len(requiredPermissions) > 0 {
		allowed := false
		for _, p := range a.Permissions {
			for _, req := range requiredPermissions {
				if p == req {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return errors.New("insufficient permissions")
		}
	}
	return nil
}

func getAuth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

func allowedRoles() []string {
	return []string{"viewer", "editor", "admin"}
}

func profileFromDB(c *runtime.RestCtx) (string, string, error) {
	a := getAuth(c)
	if a == nil {
		return "", "", errors.New("unauthorized")
	}
	pool := c.PoolPG("primary")
	var username, role string
	err := pool.QueryRow(c.Context(), `SELECT username, role FROM users WHERE id = $1`, a.UserID).Scan(&username, &role)
	if err != nil {
		return "", "", errors.New("user not found")
	}
	return username, role, nil
}
