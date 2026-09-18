package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
)

func handleChangePassword(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if err := auth.CheckPasswordStrength(body.NewPassword); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
		}
		pool := c.PoolPG("primary")
		a := getAuth(c)
		var hash string
		err := pool.QueryRow(c.Context(), `SELECT password_hash FROM users WHERE id = $1`, a.UserID).Scan(&hash)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "internal error"})
		}
		if !auth.VerifyPassword(hash, body.OldPassword) {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "wrong password"})
		}
		newHash, _ := auth.HashPassword(body.NewPassword)
		_, _ = pool.Exec(c.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, newHash, a.UserID)
		return c.JSON(runtime.Map{"status": "password_changed"})
	}
}
