package handler

import (
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleVerifyEmail(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		token := c.Query("token")
		if token == "" {
			var body struct {
				Token string `json:"token"`
			}
			if err := c.Bind(&body); err == nil && body.Token != "" {
				token = body.Token
			}
		}
		if token == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token required"})
		}
		pool := c.PoolPG("primary")
		var userID string
		var expiresAt time.Time
		err := pool.QueryRow(c.Context(),
			`SELECT user_id, expires_at FROM email_verifications WHERE token = $1 AND verified = false`, token).
			Scan(&userID, &expiresAt)
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid or expired token"})
		}
		if time.Now().After(expiresAt) {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token expired"})
		}
		_, _ = pool.Exec(c.Context(), `UPDATE email_verifications SET verified = true WHERE user_id = $1`, userID)
		return c.JSON(runtime.Map{"status": "email_verified"})
	}
}
