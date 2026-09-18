package handler

import (
	"log"
	"slices"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
)

func handleSignup(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if body.Username == "" || body.Password == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "username and password required"})
		}
		if err := auth.CheckPasswordStrength(body.Password); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
		}
		if body.Role == "" {
			body.Role = "viewer"
		}
		if !slices.Contains(allowedRoles(), body.Role) {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid role (use viewer, editor, or admin)"})
		}
		pool := c.PoolPG("primary")
		userID := "user-" + body.Username
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "internal error"})
		}
		_, err = pool.Exec(c.Context(),
			`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (username) DO NOTHING`,
			userID, body.Username, hash, body.Role)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		token, _ := auth.GenerateToken()
		_, _ = pool.Exec(c.Context(),
			`INSERT INTO email_verifications (user_id, token, verified, created_at, expires_at)
			 VALUES ($1,$2,false,now(),now()+interval '24 hours')
			 ON CONFLICT (user_id) DO UPDATE SET token=$2, verified=false, created_at=now(), expires_at=now()+interval '24 hours'`,
			userID, token)
		log.Printf("[EMAIL] Verify: http://localhost:23400/api/auth/verify-email?token=%s", token)

		return c.Status(201).JSON(runtime.Map{"status": "created", "username": body.Username, "role": body.Role})
	}
}
