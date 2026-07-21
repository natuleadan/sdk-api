package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
)

func handleMFAEnable(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := c.PoolPG("primary")
		secret, err := auth.GenerateTOTPSecret()
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "internal error"})
		}
		uri := auth.GenerateTOTPURI(secret, "400-auth", a.UserID)
		_, _ = pool.Exec(c.Context(),
			`INSERT INTO mfa_secrets (user_id, secret, enabled, created_at)
			 VALUES ($1,$2,false,now())
			 ON CONFLICT (user_id) DO UPDATE SET secret=$2, enabled=false, created_at=now()`,
			a.UserID, secret)
		return c.JSON(runtime.Map{"secret": secret, "uri": uri})
	}
}
