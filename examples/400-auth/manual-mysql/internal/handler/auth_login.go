package handler

import (
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
		pool := svc.DB(c)

		var attempts int
		var lockedUntil *string
		_ = pool.QueryRow(c.Context(),
			`SELECT attempts, locked_until FROM failed_logins WHERE username = $1`, body.Username).
			Scan(&attempts, &lockedUntil)
		if lockedUntil != nil && *lockedUntil != "" {
			if t, perr := time.Parse(time.RFC3339, *lockedUntil); perr == nil && time.Now().Before(t) {
				return c.Status(429).JSON(runtime.Map{"code": 429, "message": "account locked due to too many failed attempts"})
			}
		}

		var userID, passwordHash, role string
		err := pool.QueryRow(c.Context(),
			`SELECT id, password_hash, role FROM users WHERE username = $1`, body.Username).
			Scan(&userID, &passwordHash, &role)
		nowStr := time.Now().UTC().Format(time.RFC3339)
		lockStr := time.Now().UTC().Add(svcCtx.LockoutDuration).Format(time.RFC3339)
		if err != nil {
			_, _ = pool.Exec(c.Context(), `
INSERT INTO failed_logins (username, attempts, last_attempt, locked_until)
VALUES ($1, 1, $2, NULL)
ON CONFLICT (username) DO UPDATE SET
    locked_until = CASE
        WHEN failed_logins.attempts + 1 >= $3 THEN $4
        ELSE NULL
    END,
    attempts = failed_logins.attempts + 1,
    last_attempt = $2`, body.Username, nowStr, svcCtx.LockoutThreshold, lockStr)
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid credentials"})
		}
		if !auth.VerifyPassword(passwordHash, body.Password) {
			_, _ = pool.Exec(c.Context(), `
INSERT INTO failed_logins (username, attempts, last_attempt, locked_until)
VALUES ($1, 1, $2, NULL)
ON CONFLICT (username) DO UPDATE SET
    locked_until = CASE
        WHEN failed_logins.attempts + 1 >= $3 THEN $4
        ELSE NULL
    END,
    attempts = failed_logins.attempts + 1,
    last_attempt = $2`, body.Username, nowStr, svcCtx.LockoutThreshold, lockStr)
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
