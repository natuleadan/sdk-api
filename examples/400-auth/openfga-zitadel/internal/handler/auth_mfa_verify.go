package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func handleMFAVerify(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		pool := c.PoolPG("primary")
		var secret string
		err := pool.QueryRow(c.Context(),
			`SELECT secret FROM mfa_secrets WHERE user_id = $1`, a.UserID).Scan(&secret)
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "MFA not enabled"})
		}
		if !auth.ValidateTOTP(secret, body.Code) {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid code"})
		}
		_, _ = pool.Exec(c.Context(),
			`UPDATE mfa_secrets SET enabled = true WHERE user_id = $1`, a.UserID)

		claims := middleware.DefaultClaims(a.UserID, a.OrgID, a.Roles, a.Permissions, svcCtx.AuthExpiry)
		claims["mfa"] = true
		signed, err := middleware.SignToken(svcCtx.JWTSecret, "HS256", claims)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "token generation failed"})
		}
		return c.JSON(runtime.Map{"token": signed, "mfa": true})
	}
}
