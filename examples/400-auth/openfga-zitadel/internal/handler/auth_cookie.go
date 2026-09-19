package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// handleCookieProfile reads the caller identity straight from the auth context.
// It backs the shared contract's cookie-based JWT check: a request whose token
// arrives in a cookie (jwt_from: "cookie:token") must resolve to an identity.
func handleCookieProfile(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		return c.JSON(runtime.Map{
			"user_id": a.UserID,
			"org_id":  a.OrgID,
			"auth":    "cookie",
		})
	}
}
