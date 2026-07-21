package handler

import (
	"slices"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleSetUserRole(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
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
		if !slices.Contains(allowedRoles(), body.Role) {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid role (use viewer, editor, admin)"})
		}
		pool := c.PoolPG("primary")
		_, err := pool.Exec(c.Context(), `UPDATE users SET role = $1 WHERE id = $2`, body.Role, id)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "role_updated"})
	}
}
