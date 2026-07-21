package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleRevokeToken(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil || a.RawToken == "" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		hash := tokenHash(a.RawToken)
		pool := c.PoolPG("primary")
		_, _ = pool.Exec(c.Context(),
			`INSERT INTO revoked_tokens (token_hash, revoked_at) VALUES ($1, now()) ON CONFLICT (token_hash) DO NOTHING`, hash)
		return c.JSON(runtime.Map{"status": "revoked"})
	}
}
