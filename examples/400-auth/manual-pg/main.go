package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

//go:embed service.yaml
var serviceYAML []byte

type TenantProduct struct {
	ID       string  `db:"id,primary,auto" json:"id"`
	Name     string  `db:"name" json:"name"`
	Price    float64 `db:"price" json:"price"`
	TenantID string  `db:"tenant_id" json:"tenant_id"`
}

var (
	jwtSecret = envOrDefault("JWT_SECRET", "dev-secret-hs256-change-in-prod")
	authExpiry   = envIntOrDefault("AUTH_EXPIRY", 900)
	seedPass     = envOrDefault("SEED_PASSWORD", "pass123")
)

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

const lockoutThreshold = 5
const lockoutDuration = 15 * time.Minute

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

const extraTablesSQL = `
CREATE TABLE IF NOT EXISTS failed_logins (
    username TEXT PRIMARY KEY,
    attempts INT DEFAULT 1,
    last_attempt TIMESTAMPTZ DEFAULT now(),
    locked_until TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS revoked_tokens (
    token_hash TEXT PRIMARY KEY,
    revoked_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS mfa_secrets (
    user_id TEXT PRIMARY KEY,
    secret TEXT NOT NULL,
    enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS email_verifications (
    user_id TEXT PRIMARY KEY,
    token TEXT NOT NULL,
    verified BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS password_resets (
    user_id TEXT PRIMARY KEY,
    token TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);`

func main() {
	svc, err := runtime.NewFromYAML(serviceYAML)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	poolURL := os.Getenv("DATABASE_URL")
	if poolURL == "" {
		poolURL = "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
	}

	ctx := context.Background()

	var pool any
	hashKey := func(raw string) string {
		h := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(h[:])
	}
	for i := 0; i < 20; i++ {
		p, e := db.NewPool(ctx, db.PoolConfig{URL: poolURL})
		if e != nil {
			err = e
			goto next
		}
		if pe := p.Ping(ctx); pe != nil {
			p.Close()
			err = pe
			goto next
		}
		if _, se := p.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer', created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY, key_hash TEXT UNIQUE NOT NULL, label TEXT,
    role TEXT NOT NULL, enabled BOOLEAN DEFAULT true, created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, name TEXT NOT NULL,
    description TEXT DEFAULT '', price DECIMAL(10,2) DEFAULT 0,
    visibility TEXT DEFAULT 'public', created_by TEXT,
    deleted_at TIMESTAMPTZ, updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tenant_products (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, name TEXT NOT NULL,
    price DECIMAL(10,2) DEFAULT 0, tenant_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, product_id TEXT,
    action TEXT NOT NULL, changed_by TEXT NOT NULL,
    old_value JSONB, new_value JSONB, created_at TIMESTAMPTZ DEFAULT now()
);`); se != nil {
			p.Close()
			err = se
			goto next
		}
		if _, se := p.Exec(ctx, extraTablesSQL); se != nil {
			p.Close()
			err = se
			goto next
		}
		for _, u := range []struct{ id, name, pass, role string }{
			{"user-viewer", "viewer", seedPass, "viewer"},
			{"user-editor", "editor", seedPass, "editor"},
			{"user-admin", "admin", seedPass, "admin"},
		} {
			h, _ := auth.HashPassword(u.pass)
			_, _ = p.Exec(ctx,
				`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				u.id, u.name, h, u.role)
		}
		for _, k := range []struct{ id, label, key, role string }{
			{"key-viewer", "viewer-key", "sk-viewer_abc123", "viewer"},
			{"key-editor", "editor-key", "sk-editor_abc123", "editor"},
			{"key-admin", "admin-key", "sk-admin_abc123", "admin"},
		} {
			_, _ = p.Exec(ctx,
				`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				k.id, k.label, hashKey(k.key), k.role)
		}
		for _, tp := range []TenantProduct{
			{"tp-alfa-1", "Alfa One", 10.0, "org-alfa"},
			{"tp-alfa-2", "Alfa Two", 20.0, "org-alfa"},
			{"tp-beta-1", "Beta One", 30.0, "org-beta"},
		} {
			_, _ = p.Exec(ctx,
				`INSERT INTO tenant_products (id, name, price, tenant_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				tp.ID, tp.Name, tp.Price, tp.TenantID)
		}
		pool = p
		err = nil
		break
	next:
		select {
		case <-ctx.Done():
			log.Fatalf("pool: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	if pool == nil {
		log.Fatalf("pool: timeout after retries")
	}

	svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
		return validateJWT(ctx, auth, roles, permissions)
	})

	svc.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		h := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(h[:])
		pgPool := svc.PoolPGTyped("primary")
		var id, role string
		var enabled bool
		if err := pgPool.QueryRow(ctx,
			`SELECT id, role, enabled FROM api_keys WHERE key_hash = $1`, keyHash).
			Scan(&id, &role, &enabled); err != nil {
			return nil, errors.New("invalid API key")
		}
		if !enabled {
			return nil, errors.New("API key disabled")
		}
		return &middleware.AuthContext{UserID: id, Roles: []string{role}}, nil
	})

	svc.WithRateLimitMaxFunc(func(c *runtime.RestCtx) int {
		if c.Get("X-Debug") == "true" {
			return 5
		}
		return 0
	})

	store := newProductStore()

	svc.WithRest("loginHandler", func(c *runtime.RestCtx) error { return handleLogin(c) })
	svc.WithRest("signupHandler", func(c *runtime.RestCtx) error { return handleSignup(c) })
	svc.WithRest("forgotPasswordHandler", func(c *runtime.RestCtx) error { return handleForgotPassword(c) })
	svc.WithRest("resetPasswordHandler", func(c *runtime.RestCtx) error { return handleResetPassword(c) })
	svc.WithRest("verifyEmailHandler", func(c *runtime.RestCtx) error { return handleVerifyEmail(c) })
	svc.WithRest("mfaEnableHandler", func(c *runtime.RestCtx) error { return handleMFAEnable(c) })
	svc.WithRest("mfaVerifyHandler", func(c *runtime.RestCtx) error { return handleMFAVerify(c) })
	svc.WithRest("mfaProtectedHandler", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"data": "sensitive-data"}) })
	svc.WithRest("changePasswordHandler", func(c *runtime.RestCtx) error { return handleChangePassword(c) })
	svc.WithRest("noCSRFHandler", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "no-csrf"}) })
	svc.WithRest("profileHandler", func(c *runtime.RestCtx) error { return handleProfile(c) })
	svc.WithRest("listProducts", func(c *runtime.RestCtx) error { return store.list(c) })
	svc.WithRest("createProduct", func(c *runtime.RestCtx) error { return store.create(c) })
	svc.WithRest("getProduct", func(c *runtime.RestCtx) error { return store.get(c) })
	svc.WithRest("updateProduct", func(c *runtime.RestCtx) error { return store.update(c) })
	svc.WithRest("deleteProduct", func(c *runtime.RestCtx) error { return store.delete(c) })
	svc.WithRest("hardDeleteProduct", func(c *runtime.RestCtx) error { return store.hardDelete(c) })
	svc.WithJWTBlacklist(func(rawToken string) bool {
		pool := svc.PoolPGTyped("primary")
		if pool == nil {
			return false
		}
		hash := tokenHash(rawToken)
		var exists bool
		_ = pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_hash = $1)`, hash).Scan(&exists)
		return exists
	})
	svc.WithRest("revokeTokenHandler", func(c *runtime.RestCtx) error { return handleRevokeToken(c) })
	svc.WithRest("blacklistProtectedHandler", func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"data": "sensitive"})
	})
	svc.WithRest("setVisibility", func(c *runtime.RestCtx) error { return store.setVisibility(c) })
	svc.WithRest("getAuditLog", func(c *runtime.RestCtx) error { return store.getAuditLog(c) })
	svc.WithRest("listUsers", func(c *runtime.RestCtx) error { return handleListUsers(c) })
	svc.WithRest("deleteUser", func(c *runtime.RestCtx) error { return handleDeleteUser(c) })
	svc.WithRest("setUserRole", func(c *runtime.RestCtx) error { return handleSetUserRole(c) })
	svc.WithRest("rateLimitedHandler", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "ok"}) })
	svc.WithRest("perUserLimited", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "ok"}) })
	svc.WithRest("perKeyLimited", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "ok"}) })
	svc.WithRest("perRoleLimited", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "ok"}) })
	svc.WithRest("viewerDataHandler", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"data": "viewer-only"}) })
	svc.WithRest("maxFuncLimited", func(c *runtime.RestCtx) error { return c.JSON(runtime.Map{"status": "ok"}) })

	svc.WithCRUDFactory("TenantProduct", func() runtime.CRUDProvider {
		pgPool := svc.PoolPGTyped("primary")
		table, tErr := db.NewTable[TenantProduct](pgPool, "tenant_products")
		if tErr != nil {
			log.Fatalf("tenant table: %v", tErr)
		}
		return runtime.NewCRUDProvider(table, nil)
	})

	log.Fatal(svc.Run())
}

func validateJWT(_ context.Context, a *middleware.AuthContext, requiredRoles, requiredPermissions []string) error {
	if len(requiredRoles) > 0 {
		allowed := false
		for _, r := range a.Roles {
			for _, req := range requiredRoles {
				if r == req || roleInherits(r, req) {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return errors.New("insufficient role")
		}
	}
	if len(requiredPermissions) > 0 {
		allowed := false
		for _, p := range a.Permissions {
			for _, req := range requiredPermissions {
				if p == req {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return errors.New("insufficient permissions")
		}
	}
	return nil
}

var roleHierarchy = map[string][]string{
	"viewer": {},
	"editor": {"viewer"},
	"admin":  {"editor", "viewer"},
}

func roleInherits(userRole, requiredRole string) bool {
	if userRole == requiredRole {
		return true
	}
	inherited, ok := roleHierarchy[userRole]
	if !ok {
		return false
	}
	for _, r := range inherited {
		if r == requiredRole || roleInherits(r, requiredRole) {
			return true
		}
	}
	return false
}

func checkPasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain an uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain a lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain a digit")
	}
	return nil
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func handleRevokeToken(c *runtime.RestCtx) error {
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

func handleLogin(c *runtime.RestCtx) error {
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
    END`, body.Username, lockoutThreshold, fmt.Sprintf("%d minutes", int(lockoutDuration.Minutes())))
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
    END`, body.Username, lockoutThreshold, fmt.Sprintf("%d minutes", int(lockoutDuration.Minutes())))
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
	claims := middleware.DefaultClaims(userID, orgID, []string{role}, permissions, authExpiry)
	var mfaEnabled bool
	_ = pool.QueryRow(c.Context(),
		`SELECT enabled FROM mfa_secrets WHERE user_id = $1`, userID).Scan(&mfaEnabled)
	if mfaEnabled {
		claims["mfa"] = false
	}
	signed, err := middleware.SignToken(jwtSecret, "HS256", claims)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "token generation failed"})
	}
	c.SetCookie(runtime.NewCookie("token", signed, authExpiry))
	return c.JSON(runtime.Map{"token": signed, "role": role})
}

func handleSignup(c *runtime.RestCtx) error {
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
	if err := checkPasswordStrength(body.Password); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
	}
	if body.Role == "" {
		body.Role = "viewer"
	}
	allowedRoles := []string{"viewer", "editor", "admin"}
	if !slices.Contains(allowedRoles, body.Role) {
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

	token := generateToken()
	_, _ = pool.Exec(c.Context(),
		`INSERT INTO email_verifications (user_id, token, verified, created_at, expires_at)
		 VALUES ($1,$2,false,now(),now()+interval '24 hours')
		 ON CONFLICT (user_id) DO UPDATE SET token=$2, verified=false, created_at=now(), expires_at=now()+interval '24 hours'`,
		userID, token)
	log.Printf("[EMAIL] Verify: http://localhost:23400/api/v1/auth/verify-email?token=%s", token)

	return c.Status(201).JSON(runtime.Map{"status": "created", "username": body.Username, "role": body.Role})
}

func handleForgotPassword(c *runtime.RestCtx) error {
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

func handleResetPassword(c *runtime.RestCtx) error {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	if err := checkPasswordStrength(body.Password); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
	}
	pool := c.PoolPG("primary")
	var userID string
	var expiresAt time.Time
	err := pool.QueryRow(c.Context(),
		`SELECT user_id, expires_at FROM password_resets WHERE token = $1`, body.Token).
		Scan(&userID, &expiresAt)
	if err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid or expired token"})
	}
	if time.Now().After(expiresAt) {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token expired"})
	}
	hash, _ := auth.HashPassword(body.Password)
	_, _ = pool.Exec(c.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, userID)
	_, _ = pool.Exec(c.Context(), `DELETE FROM password_resets WHERE user_id = $1`, userID)
	return c.JSON(runtime.Map{"status": "password_updated"})
}

func handleVerifyEmail(c *runtime.RestCtx) error {
	token := c.Query("token")
	if token == "" {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.Bind(&body); err == nil && body.Token != "" {
			token = body.Token
		}
	}
	if token == "" {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token required"})
	}
	pool := c.PoolPG("primary")
	var userID string
	var expiresAt time.Time
	err := pool.QueryRow(c.Context(),
		`SELECT user_id, expires_at FROM email_verifications WHERE token = $1 AND verified = false`, token).
		Scan(&userID, &expiresAt)
	if err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid or expired token"})
	}
	if time.Now().After(expiresAt) {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "token expired"})
	}
	_, _ = pool.Exec(c.Context(), `UPDATE email_verifications SET verified = true WHERE user_id = $1`, userID)
	return c.JSON(runtime.Map{"status": "email_verified"})
}

func handleChangePassword(c *runtime.RestCtx) error {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	if err := checkPasswordStrength(body.NewPassword); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": err.Error()})
	}
	pool := c.PoolPG("primary")
	a := getAuth(c)
	var hash string
	err := pool.QueryRow(c.Context(), `SELECT password_hash FROM users WHERE id = $1`, a.UserID).Scan(&hash)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "internal error"})
	}
	if !auth.VerifyPassword(hash, body.OldPassword) {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "wrong password"})
	}
	newHash, _ := auth.HashPassword(body.NewPassword)
	_, _ = pool.Exec(c.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, newHash, a.UserID)
	return c.JSON(runtime.Map{"status": "password_changed"})
}

func handleMFAEnable(c *runtime.RestCtx) error {
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

func handleMFAVerify(c *runtime.RestCtx) error {
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

	claims := middleware.DefaultClaims(a.UserID, a.OrgID, a.Roles, a.Permissions, authExpiry)
	claims["mfa"] = true
	signed, err := middleware.SignToken(jwtSecret, "HS256", claims)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "token generation failed"})
	}
	return c.JSON(runtime.Map{"token": signed, "mfa": true})
}

func handleProfile(c *runtime.RestCtx) error {
	a := getAuth(c)
	if a == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	pool := c.PoolPG("primary")
	var username, role string
	err := pool.QueryRow(c.Context(), `SELECT username, role FROM users WHERE id = $1`, a.UserID).Scan(&username, &role)
	if err != nil {
		return c.Status(404).JSON(runtime.Map{"code": 404, "message": "user not found"})
	}
	return c.JSON(runtime.Map{"username": username, "role": role, "user_id": a.UserID})
}

func handleListUsers(c *runtime.RestCtx) error {
	pool := c.PoolPG("primary")
	rows, err := pool.Query(c.Context(), `SELECT id, username, role FROM users ORDER BY username`)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	defer rows.Close()
	var users []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	for rows.Next() {
		var u struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			break
		}
		users = append(users, u)
	}
	if users == nil {
		users = []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		}{}
	}
	return c.JSON(runtime.Map{"data": users})
}

func handleDeleteUser(c *runtime.RestCtx) error {
	a := getAuth(c)
	if a == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == a.UserID {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot delete yourself"})
	}
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	return c.JSON(runtime.Map{"status": "deleted"})
}

func handleSetUserRole(c *runtime.RestCtx) error {
	a := getAuth(c)
	if a == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == a.UserID {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot change your own role"})
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	allowedRoles := []string{"viewer", "editor", "admin"}
	if !slices.Contains(allowedRoles, body.Role) {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid role (use viewer, editor, admin)"})
	}
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(), `UPDATE users SET role = $1 WHERE id = $2`, body.Role, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	return c.JSON(runtime.Map{"status": "role_updated"})
}

func getAuth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

type productStore struct {
	mu sync.RWMutex
}

func newProductStore() *productStore {
	return &productStore{}
}

func (s *productStore) auth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

func (s *productStore) hasRole(a *middleware.AuthContext, role string) bool {
	if a == nil {
		return false
	}
	for _, r := range a.Roles {
		if r == role || roleInherits(r, role) {
			return true
		}
	}
	return false
}

func (s *productStore) list(c *runtime.RestCtx) error {
	a := s.auth(c)
	pool := c.PoolPG("primary")
	rows, err := pool.Query(c.Context(),
		`SELECT id, name, description, price, visibility, created_by, deleted_at, updated_at
		 FROM products ORDER BY updated_at DESC`)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	defer rows.Close()
	type product struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       string  `json:"price"`
		Visibility  string  `json:"visibility"`
		CreatedBy   string  `json:"created_by"`
		UpdatedAt   string  `json:"updated_at"`
	}
	var products []product
	for rows.Next() {
		var p product
		var deletedAt *time.Time
		var updatedAt time.Time
		var price float64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &price, &p.Visibility, &p.CreatedBy, &deletedAt, &updatedAt); err != nil {
			break
		}
		p.Price = fmt.Sprintf("%.2f", price)
		p.UpdatedAt = updatedAt.Format(time.RFC3339)
		if p.Visibility == "confidential" && !s.hasRole(a, "admin") {
			p.Description = "[restricted]"
		}
		products = append(products, p)
	}
	if products == nil {
		products = []product{}
	}
	return c.JSON(runtime.Map{"data": products})
}

func (s *productStore) get(c *runtime.RestCtx) error {
	a := s.auth(c)
	id := c.Params("id")
	pool := c.PoolPG("primary")
	var idOut, name, description, visibility, createdBy string
	var price float64
	var deletedAt *time.Time
	var updatedAt time.Time
	err := pool.QueryRow(c.Context(),
		`SELECT id, name, description, price, visibility, created_by, deleted_at, updated_at
		 FROM products WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&idOut, &name, &description, &price, &visibility, &createdBy, &deletedAt, &updatedAt)
	if err != nil {
		return c.Status(404).JSON(runtime.Map{"code": 404, "message": err.Error()})
	}
	if visibility == "confidential" && !s.hasRole(a, "admin") {
		description = "[restricted]"
	}
	return c.JSON(runtime.Map{
		"id":          idOut,
		"name":        name,
		"description": description,
		"price":       price,
		"visibility":  visibility,
		"created_by":  createdBy,
		"updated_at":  updatedAt.Format(time.RFC3339),
	})
}

func (s *productStore) create(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "editor") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "name required"})
	}
	pool := c.PoolPG("primary")
	var id string
	err := pool.QueryRow(c.Context(),
		`INSERT INTO products (name, description, price, created_by) VALUES ($1,$2,$3,$4) RETURNING id`,
		body.Name, body.Description, body.Price, a.UserID).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "create", a.UserID, nil, runtime.Map{"name": body.Name, "price": body.Price})
	return c.Status(201).JSON(runtime.Map{"id": id})
}

func (s *productStore) update(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "editor") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	var body struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Price       *float64 `json:"price"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	fields := []string{}
	args := []any{}
	argIdx := 1
	if body.Name != nil {
		fields = append(fields, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *body.Name)
		argIdx++
	}
	if body.Description != nil {
		fields = append(fields, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *body.Description)
		argIdx++
	}
	if body.Price != nil {
		fields = append(fields, fmt.Sprintf("price = $%d", argIdx))
		args = append(args, *body.Price)
		argIdx++
	}
	if len(fields) == 0 {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "no fields to update"})
	}
	fields = append(fields, "updated_at = now()")
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE products SET %s WHERE id = $%d AND deleted_at IS NULL`, strings.Join(fields, ", "), argIdx)
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(), q, args...)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "update", a.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "updated"})
}

func (s *productStore) delete(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "editor") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(),
		`UPDATE products SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "soft_delete", a.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "deleted"})
}

func (s *productStore) hardDelete(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "admin") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "hard_delete", a.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "hard_deleted"})
}

func (s *productStore) setVisibility(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "admin") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	allowed := []string{"public", "internal", "confidential"}
	if !slices.Contains(allowed, body.Visibility) {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "visibility must be public, internal, or confidential"})
	}
	pool := c.PoolPG("primary")
	var oldVis string
	_ = pool.QueryRow(c.Context(), `SELECT visibility FROM products WHERE id = $1`, id).Scan(&oldVis)
	_, err := pool.Exec(c.Context(),
		`UPDATE products SET visibility = $1, updated_at = now() WHERE id = $2`, body.Visibility, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "set_visibility", a.UserID,
		runtime.Map{"visibility": oldVis}, runtime.Map{"visibility": body.Visibility})
	return c.JSON(runtime.Map{"status": "visibility_updated"})
}

func (s *productStore) getAuditLog(c *runtime.RestCtx) error {
	a := s.auth(c)
	if !s.hasRole(a, "admin") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("primary")
	rows, err := pool.Query(c.Context(),
		`SELECT id, product_id, action, changed_by, old_value, new_value, created_at
		 FROM audit_log WHERE product_id = $1 ORDER BY created_at DESC`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	defer rows.Close()
	type entry struct {
		ID        string `json:"id"`
		ProductID string `json:"product_id"`
		Action    string `json:"action"`
		ChangedBy string `json:"changed_by"`
		OldValue  any    `json:"old_value,omitempty"`
		NewValue  any    `json:"new_value,omitempty"`
		CreatedAt string `json:"created_at"`
	}
	var entries []entry
	for rows.Next() {
		var e entry
		var createdAt time.Time
		var oldVal, newVal *string
		if err := rows.Scan(&e.ID, &e.ProductID, &e.Action, &e.ChangedBy, &oldVal, &newVal, &createdAt); err != nil {
			break
		}
		e.CreatedAt = createdAt.Format(time.RFC3339)
		if oldVal != nil {
			var v any
			if err := json.Unmarshal([]byte(*oldVal), &v); err == nil {
				e.OldValue = v
			}
		}
		if newVal != nil {
			var v any
			if err := json.Unmarshal([]byte(*newVal), &v); err == nil {
				e.NewValue = v
			}
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []entry{}
	}
	return c.JSON(runtime.Map{"data": entries})
}

func (s *productStore) logAudit(c *runtime.RestCtx, productID, action, changedBy string, oldVal, newVal any) {
	oldJSON := marshalJSON(oldVal)
	newJSON := marshalJSON(newVal)
	pool := c.PoolPG("primary")
	_, _ = pool.Exec(c.Context(),
		`INSERT INTO audit_log (product_id, action, changed_by, old_value, new_value) VALUES ($1,$2,$3,$4,$5)`,
		productID, action, changedBy, oldJSON, newJSON)
}

func marshalJSON(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}
