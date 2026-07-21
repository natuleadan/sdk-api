package handler

import (
	"log"

	"auth-roles/internal/svc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func handleMagicLink(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.Bind(&body); err != nil || body.Email == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "email required"})
		}

		pool := c.PoolPG("primary")
		var userID, username string
		err := pool.QueryRow(c.Context(),
			`SELECT id, username FROM users WHERE username = $1`, body.Email).
			Scan(&userID, &username)
		if err != nil {
			return c.Status(200).JSON(runtime.Map{"status": "link_sent", "email": body.Email})
		}

		claims := middleware.DefaultClaims(userID, "", nil, nil, 300)
		claims["purpose"] = "magic_link"
		claims["email"] = body.Email
		token, err := middleware.SignToken(svcCtx.JWTSecret, "HS256", claims)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "token generation failed"})
		}

		link := "http://localhost:23400/api/auth/magic-link/verify?token=" + token
		log.Printf("[MAGIC LINK] %s: %s", body.Email, link)
		log.Printf("[MAGIC LINK CODE] %s: %s", body.Email, token[:32])

		return c.JSON(runtime.Map{"status": "link_sent", "email": body.Email})
	}
}

func handleMagicLinkVerify(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		tokenStr := c.Query("token")
		if tokenStr == "" {
			var body struct {
				Token string `json:"token"`
			}
			if err := c.Bind(&body); err == nil {
				tokenStr = body.Token
			}
		}
		if tokenStr == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token required"})
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			return []byte(svcCtx.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid or expired token"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || claims["purpose"] != "magic_link" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid token purpose"})
		}

		sub, _ := claims["sub"].(string)
		if sub == "" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid token"})
		}

		pool := c.PoolPG("primary")
		var username, role string
		if err := pool.QueryRow(c.Context(),
			`SELECT username, role FROM users WHERE id = $1`, sub).Scan(&username, &role); err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "user not found"})
		}

		sessionToken, err := middleware.SignToken(svcCtx.JWTSecret, "HS256",
			middleware.DefaultClaims(sub, "", []string{role}, nil, svcCtx.AuthExpiry))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session creation failed"})
		}

		c.SetCookie(runtime.NewCookie("token", sessionToken, svcCtx.AuthExpiry))
		log.Printf("[MAGIC LINK] user %s (%s) logged in via magic link", username, sub)
		return c.JSON(runtime.Map{"token": sessionToken, "role": role, "user_id": sub})
	}
}
