package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleDeleteUser(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		id := c.Params("id")
		if id == a.UserID {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot delete yourself"})
		}
		pool := c.PoolPG("primary")
		_, err := pool.Exec(c.Context(), `DELETE FROM users WHERE id = $1`, id)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "deleted"})
	}
}
