package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

var roleHierarchy = map[string][]string{
	"viewer": {},
	"editor": {"viewer"},
	"admin":  {"editor", "viewer"},
}

func ValidateJWT(a *middleware.AuthContext, requiredRoles, requiredPermissions []string) error {
	if len(requiredRoles) > 0 {
		allowed := false
		for _, r := range a.Roles {
			for _, req := range requiredRoles {
				if r == req || roleInherits(r, req) {
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

func roleInherits(userRole, requiredRole string) bool {
	if userRole == requiredRole {
		return true
	}
	inherited, ok := roleHierarchy[userRole]
	if !ok {
		return false
	}
	for _, r := range inherited {
		if r == requiredRole || roleInherits(r, requiredRole) {
			return true
		}
	}
	return false
}

func getAuth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func allowedRoles() []string {
	return []string{"viewer", "editor", "admin"}
}

func checkPasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain an uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain a lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain a digit")
	}
	return nil
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
