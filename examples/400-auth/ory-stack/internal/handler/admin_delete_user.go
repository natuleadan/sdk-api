package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// handleDeleteUser removes a Kratos identity (admin API). Identity is owned by
// Kratos, not a local users table.
func handleDeleteUser(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		id := c.Params("id")
		if id == a.UserID {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot delete yourself"})
		}
		if svcCtx.Ory == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "Ory not configured"})
		}
		if err := svcCtx.Ory.DeleteIdentity(c.Context(), id); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "deleted"})
	}
}
