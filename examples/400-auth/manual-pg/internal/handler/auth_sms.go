package handler

import (
	"fmt"
	"log"
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func handleSMSSend(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Phone string `json:"phone"`
		}
		if err := c.Bind(&body); err != nil || body.Phone == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "phone required"})
		}

		pool := c.PoolPG("primary")
		var userID string
		err := pool.QueryRow(c.Context(),
			`SELECT id FROM users WHERE username = $1`, body.Phone).Scan(&userID)
		if err != nil {
			userID = "anon-" + body.Phone
		}

	code := generateCode(6)
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = pool.Exec(c.Context(),
		`INSERT INTO auth_codes (user_id, code, purpose, delivered_to, delivery_method, expires_at)
		 VALUES ($1, $2, 'sms_verify', $3, 'sms', $4)`,
		userID, code, body.Phone, expiresAt)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "code generation failed"})
	}

	msg := fmt.Sprintf("Your 400-auth verification code is: %s", code)
	svcCtx.SMSProvider.Send(c.Context(), body.Phone, msg)

	log.Printf("[SMS CODE] %s: %s", body.Phone, code)
		return c.JSON(runtime.Map{"status": "code_sent", "phone": body.Phone})
	}
}

func handleSMSVerify(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Phone string `json:"phone"`
			Code  string `json:"code"`
		}
		if err := c.Bind(&body); err != nil || body.Phone == "" || body.Code == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "phone and code required"})
		}

		pool := c.PoolPG("primary")
		var id, userID string
		var used bool
		var expiresAt time.Time
		err := pool.QueryRow(c.Context(),
			`SELECT id, user_id, used, expires_at FROM auth_codes
			 WHERE code = $1 AND purpose = 'sms_verify' AND delivered_to = $2
			 ORDER BY created_at DESC LIMIT 1`,
			body.Code, body.Phone).Scan(&id, &userID, &used, &expiresAt)

		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid code"})
		}
		if used {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "code already used"})
		}
		if time.Now().After(expiresAt) {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "code expired"})
		}

		_, _ = pool.Exec(c.Context(), `UPDATE auth_codes SET used = true WHERE id = $1`, id)

		if userID == "" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "user not found"})
		}

		var username, role string
		pool.QueryRow(c.Context(),
			`SELECT username, role FROM users WHERE id = $1`, userID).Scan(&username, &role)

		sessionToken, err := middleware.SignToken(svcCtx.JWTSecret, "HS256",
			middleware.DefaultClaims(userID, "", []string{role}, nil, svcCtx.AuthExpiry))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session creation failed"})
		}

		c.SetCookie(runtime.NewCookie("token", sessionToken, svcCtx.AuthExpiry))
		log.Printf("[SMS CODE] user %s (%s) logged in via SMS code", username, userID)
		return c.JSON(runtime.Map{"token": sessionToken, "role": role, "user_id": userID})
	}
}
