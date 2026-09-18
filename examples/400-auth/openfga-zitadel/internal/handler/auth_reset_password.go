package handler

import (
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
)

func handleResetPassword(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if err := auth.CheckPasswordStrength(body.Password); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
		}
		pool := c.PoolPG("primary")
		var userID string
		var expiresAt time.Time
		err := pool.QueryRow(c.Context(),
			`SELECT user_id, expires_at FROM password_resets WHERE token = $1`, body.Token).
			Scan(&userID, &expiresAt)
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid or expired token"})
		}
		if time.Now().After(expiresAt) {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token expired"})
		}
		hash, _ := auth.HashPassword(body.Password)
		_, _ = pool.Exec(c.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, userID)
		_, _ = pool.Exec(c.Context(), `DELETE FROM password_resets WHERE user_id = $1`, userID)
		return c.JSON(runtime.Map{"status": "password_updated"})
	}
}
