package handler

import (
	"log"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleForgotPassword(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		pool := c.PoolPG("primary")
		token := generateToken()
		_, _ = pool.Exec(c.Context(),
			`INSERT INTO password_resets (user_id, token, created_at, expires_at)
			 VALUES ($1,$2,now(),now()+interval '1 hour')
			 ON CONFLICT (user_id) DO UPDATE SET token=$2, created_at=now(), expires_at=now()+interval '1 hour'`,
			"user-"+body.Username, token)
		log.Printf("[EMAIL] Reset: http://localhost:23400/api/v1/auth/reset-password?token=%s", token)
		return c.JSON(runtime.Map{"status": "reset_link_sent"})
	}
}
