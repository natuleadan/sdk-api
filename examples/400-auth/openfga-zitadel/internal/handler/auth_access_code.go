package handler

import (
	"crypto/rand"
	"log"
	"math/big"
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func generateCode(length int) string {
	code := make([]byte, length)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code[i] = byte('0') + byte(n.Int64())
	}
	return string(code)
}

func handleAccessCode(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.Bind(&body); err != nil || body.Email == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "email required"})
		}

		pool := c.PoolPG("primary")
		var userID string
		err := pool.QueryRow(c.Context(),
			`SELECT id FROM users WHERE username = $1`, body.Email).Scan(&userID)
		if err != nil {
			return c.Status(200).JSON(runtime.Map{"status": "code_sent", "email": body.Email})
		}

		code := generateCode(6)
		expiresAt := time.Now().Add(5 * time.Minute)
		_, err = pool.Exec(c.Context(),
			`INSERT INTO auth_codes (user_id, code, purpose, delivered_to, delivery_method, expires_at)
			 VALUES ($1, $2, 'access', $3, 'console', $4)`,
			userID, code, body.Email, expiresAt)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "code generation failed"})
		}

		log.Printf("[ACCESS CODE] %s: %s", body.Email, code)
		return c.JSON(runtime.Map{"status": "code_sent", "email": body.Email})
	}
}

func handleAccessCodeVerify(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := c.Bind(&body); err != nil || body.Email == "" || body.Code == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "email and code required"})
		}

		pool := c.PoolPG("primary")
		var id, userID string
		var used bool
		var expiresAt time.Time
		err := pool.QueryRow(c.Context(),
			`SELECT id, user_id, used, expires_at FROM auth_codes
			 WHERE code = $1 AND purpose = 'access' AND delivered_to = $2
			 ORDER BY created_at DESC LIMIT 1`,
			body.Code, body.Email).Scan(&id, &userID, &used, &expiresAt)

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

		var username, role string
		pool.QueryRow(c.Context(),
			`SELECT username, role FROM users WHERE id = $1`, userID).Scan(&username, &role)

		sessionToken, err := middleware.SignToken(svcCtx.JWTSecret, "HS256",
			middleware.DefaultClaims(userID, "", []string{role}, nil, svcCtx.AuthExpiry))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session creation failed"})
		}

		c.SetCookie(runtime.NewCookie("token", sessionToken, svcCtx.AuthExpiry))
		log.Printf("[ACCESS CODE] user %s (%s) logged in via access code", username, userID)
		return c.JSON(runtime.Map{"token": sessionToken, "role": role, "user_id": userID})
	}
}
