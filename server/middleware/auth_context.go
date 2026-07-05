package middleware

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const authCtxKey ctxKey = "auth"

type AuthContext struct {
	UserID      string
	OrgID       string
	Roles       []string
	Permissions []string
	RawToken    string
	Claims      jwt.MapClaims
}

func buildAuthContext(claims jwt.MapClaims, rawToken string) *AuthContext {
	auth := &AuthContext{
		UserID:   stringOr(claims, "sub"),
		OrgID:    stringOr(claims, "org_id"),
		RawToken: rawToken,
		Claims:   claims,
	}
	if roles, ok := claims["roles"].([]any); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok {
				auth.Roles = append(auth.Roles, s)
			}
		}
	}
	if perms, ok := claims["permissions"].([]any); ok {
		for _, p := range perms {
			if s, ok := p.(string); ok {
				auth.Permissions = append(auth.Permissions, s)
			}
		}
	}
	return auth
}

func injectAuth(c fiber.Ctx, auth *AuthContext) {
	c.Locals("auth", auth)
	ctx := context.WithValue(c.Context(), authCtxKey, auth)
	c.SetContext(ctx)
}

func GetAuth(c fiber.Ctx) *AuthContext {
	if a, ok := c.Locals("auth").(*AuthContext); ok {
		return a
	}
	return nil
}

func AuthFromContext(ctx context.Context) *AuthContext {
	if a, ok := ctx.Value(authCtxKey).(*AuthContext); ok {
		return a
	}
	return nil
}

func stringOr(claims jwt.MapClaims, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

// fiber:context-methods migrated
