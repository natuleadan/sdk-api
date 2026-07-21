package handler

import (
	"fmt"
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func handleLogin(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		pool := c.PoolPG("primary")

		var attempts int
		var lockedUntil *time.Time
		_ = pool.QueryRow(c.Context(),
			`SELECT attempts, locked_until FROM failed_logins WHERE username = $1`, body.Username).
			Scan(&attempts, &lockedUntil)
		if lockedUntil != nil && time.Now().Before(*lockedUntil) {
			return c.Status(429).JSON(runtime.Map{"code": 429, "message": "account locked due to too many failed attempts"})
		}

		var userID, passwordHash, role string
		err := pool.QueryRow(c.Context(),
			`SELECT id, password_hash, role FROM users WHERE username = $1`, body.Username).
			Scan(&userID, &passwordHash, &role)
		if err != nil {
			_, _ = pool.Exec(c.Context(), `
INSERT INTO failed_logins (username, attempts, last_attempt, locked_until)
VALUES ($1, 1, now(), NULL)
ON CONFLICT (username) DO UPDATE SET
    attempts = failed_logins.attempts + 1,
    last_attempt = now(),
    locked_until = CASE
        WHEN failed_logins.attempts + 1 >= $2 THEN now() + $3::interval
        ELSE NULL
    END`, body.Username, svcCtx.LockoutThreshold, fmt.Sprintf("%d minutes", int(svcCtx.LockoutDuration.Minutes())))
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid credentials"})
		}
		if !auth.VerifyPassword(passwordHash, body.Password) {
			_, _ = pool.Exec(c.Context(), `
INSERT INTO failed_logins (username, attempts, last_attempt, locked_until)
VALUES ($1, 1, now(), NULL)
ON CONFLICT (username) DO UPDATE SET
    attempts = failed_logins.attempts + 1,
    last_attempt = now(),
    locked_until = CASE
        WHEN failed_logins.attempts + 1 >= $2 THEN now() + $3::interval
        ELSE NULL
    END`, body.Username, svcCtx.LockoutThreshold, fmt.Sprintf("%d minutes", int(svcCtx.LockoutDuration.Minutes())))
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid credentials"})
		}
		_, _ = pool.Exec(c.Context(), `DELETE FROM failed_logins WHERE username = $1`, body.Username)

		var permissions []string
		if role == "admin" {
			permissions = []string{"users:manage"}
		}
		orgID := "org-alfa"
		if role == "viewer" {
			orgID = "org-beta"
		}
		claims := middleware.DefaultClaims(userID, orgID, []string{role}, permissions, svcCtx.AuthExpiry)
		var mfaEnabled bool
		_ = pool.QueryRow(c.Context(),
			`SELECT enabled FROM mfa_secrets WHERE user_id = $1`, userID).Scan(&mfaEnabled)
		if mfaEnabled {
			claims["mfa"] = false
		}
		signed, err := middleware.SignToken(svcCtx.JWTSecret, "HS256", claims)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "token generation failed"})
		}
		c.SetCookie(runtime.NewCookie("token", signed, svcCtx.AuthExpiry))
		return c.JSON(runtime.Map{"token": signed, "role": role})
	}
}
