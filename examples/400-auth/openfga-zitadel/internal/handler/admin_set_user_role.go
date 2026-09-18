package handler

import (
	"context"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// handleSetUserRole writes the role assignment to OpenFGA (user → member →
// role:<role>). Role names are free-form; authorization is FGA-backed.
func handleSetUserRole(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		id := c.Params("id")
		if id == a.UserID {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot change your own role"})
		}
		var body struct {
			Role string `json:"role"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if body.Role == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "role required"})
		}
		if svcCtx.FGA == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "FGA not configured"})
		}
		if err := svcCtx.FGA.AssignRole(context.Background(), "user:"+id, body.Role); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "role_updated"})
	}
}
