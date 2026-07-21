package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleProfile(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		username, role, err := profileFromDB(c)
		if err != nil {
			if err.Error() == "user not found" {
				return c.Status(404).JSON(runtime.Map{"code": 404, "message": "user not found"})
			}
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": err.Error()})
		}
		a := getAuth(c)
		return c.JSON(runtime.Map{"username": username, "role": role, "user_id": a.UserID})
	}
}
