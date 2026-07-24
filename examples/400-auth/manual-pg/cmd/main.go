package main

import (
	"context"
	"errors"
	"log"
	"os"

	"auth-roles/internal/handler"
	"auth-roles/internal/models"
	"auth-roles/internal/svc"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svcCtx := svc.NewServiceContext()

	s.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return handler.ValidateJWT(a, roles, permissions)
	})

	s.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		keyHash := auth.TokenHash(key)
		pgPool := s.PoolPGTyped("primary")
		var id, role string
		var enabled bool
		if err := pgPool.QueryRow(ctx, `SELECT id, role, enabled FROM api_keys WHERE key_hash = $1`, keyHash).
			Scan(&id, &role, &enabled); err != nil {
			return nil, errors.New("invalid API key")
		}
		if !enabled {
			return nil, errors.New("API key disabled")
		}
		return &middleware.AuthContext{UserID: id, Roles: []string{role}}, nil
	})

	s.WithJWTBlacklist(func(rawToken string) bool {
		pool := s.PoolPGTyped("primary")
		if pool == nil {
			return false
		}
		hash := auth.TokenHash(rawToken)
		var exists bool
		_ = pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_hash = $1)`, hash).Scan(&exists)
		return exists
	})

	s.WithRateLimitMaxFunc(func(c *runtime.RestCtx) int {
		if c.Get("X-Debug") == "true" {
			return 5
		}
		return 0
	})

	handler.RegisterRestRoutes(s, svcCtx)

	s.WithCRUDFactory("TenantProduct", func() runtime.CRUDProvider {
		pgPool := s.PoolPGTyped("primary")
		table, tErr := db.NewTable[models.TenantProduct](pgPool, "tenant_products")
		if tErr != nil {
			log.Fatalf("tenant table: %v", tErr)
		}
		return runtime.NewCRUDProvider(table, nil)
	})

	s.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")

		tables := []struct {
			fn  func() error
			msg string
		}{
			{func() error { t, e := db.NewTable[models.User](pool, "users"); return chk(e, t.AutoInit(ctx)) }, "users"},
			{func() error { t, e := db.NewTable[models.APIKey](pool, "api_keys"); return chk(e, t.AutoInit(ctx)) }, "api_keys"},
			{func() error { t, e := db.NewTable[models.Product](pool, "products"); return chk(e, t.AutoInit(ctx)) }, "products"},
			{func() error { t, e := db.NewTable[models.TenantProduct](pool, "tenant_products"); return chk(e, t.AutoInit(ctx)) }, "tenant_products"},
			{func() error { t, e := db.NewTable[models.AuditLog](pool, "audit_log"); return chk(e, t.AutoInit(ctx)) }, "audit_log"},
			{func() error { t, e := db.NewTable[models.FailedLogin](pool, "failed_logins"); return chk(e, t.AutoInit(ctx)) }, "failed_logins"},
			{func() error { t, e := db.NewTable[models.RevokedToken](pool, "revoked_tokens"); return chk(e, t.AutoInit(ctx)) }, "revoked_tokens"},
			{func() error { t, e := db.NewTable[models.MFASecret](pool, "mfa_secrets"); return chk(e, t.AutoInit(ctx)) }, "mfa_secrets"},
			{func() error { t, e := db.NewTable[models.EmailVerification](pool, "email_verifications"); return chk(e, t.AutoInit(ctx)) }, "email_verifications"},
			{func() error { t, e := db.NewTable[models.PasswordReset](pool, "password_resets"); return chk(e, t.AutoInit(ctx)) }, "password_resets"},
			{func() error { t, e := db.NewTable[models.AuthCode](pool, "auth_codes"); return chk(e, t.AutoInit(ctx)) }, "auth_codes"},
			{func() error { t, e := db.NewTable[models.LinkedAccount](pool, "linked_accounts"); return chk(e, t.AutoInit(ctx)) }, "linked_accounts"},
			{func() error { t, e := db.NewTable[models.WebAuthnUser](pool, "webauthn_users"); return chk(e, t.AutoInit(ctx)) }, "webauthn_users"},
			{func() error { t, e := db.NewTable[models.WebAuthnCredential](pool, "webauthn_credentials"); return chk(e, t.AutoInit(ctx)) }, "webauthn_credentials"},
			{func() error { t, e := db.NewTable[models.WebAuthnSession](pool, "webauthn_sessions"); return chk(e, t.AutoInit(ctx)) }, "webauthn_sessions"},
			{func() error { t, e := db.NewTable[models.OAuthClient](pool, "oauth_clients"); return chk(e, t.AutoInit(ctx)) }, "oauth_clients"},
			{func() error { t, e := db.NewTable[models.OAuthSession](pool, "oauth_sessions"); return chk(e, t.AutoInit(ctx)) }, "oauth_sessions"},
			{func() error { t, e := db.NewTable[models.OAuthJTIS](pool, "oauth_jti_blacklist"); return chk(e, t.AutoInit(ctx)) }, "oauth_jti_blacklist"},
		}
		for _, tbl := range tables {
			if err := tbl.fn(); err != nil {
				log.Fatalf("autoinit %s: %v", tbl.msg, err)
			}
		}

		seedPass := os.Getenv("SEED_PASSWORD")
		if seedPass == "" {
			seedPass = "pass123"
		}
		for _, u := range []struct{ id, name, pass, role string }{
			{"user-viewer", "viewer", seedPass, "viewer"},
			{"user-editor", "editor", seedPass, "editor"},
			{"user-admin", "admin", seedPass, "admin"},
		} {
			h, _ := auth.HashPassword(u.pass)
			if _, err := pool.Exec(ctx,
				`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				u.id, u.name, h, u.role); err != nil {
				return err
			}
		}
		for _, k := range []struct{ id, label, key, role string }{
			{"key-viewer", "viewer-key", "sk-viewer_abc123", "viewer"},
			{"key-editor", "editor-key", "sk-editor_abc123", "editor"},
			{"key-admin", "admin-key", "sk-admin_abc123", "admin"},
		} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				k.id, k.label, auth.TokenHash(k.key), k.role); err != nil {
				return err
			}
		}
		for _, tp := range []struct {
			id       string
			name     string
			price    float64
			tenantID string
		}{
			{"tp-alfa-1", "Alfa One", 10.0, "org-alfa"},
			{"tp-alfa-2", "Alfa Two", 20.0, "org-alfa"},
			{"tp-beta-1", "Beta One", 30.0, "org-beta"},
		} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO tenant_products (id, name, price, tenant_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				tp.id, tp.name, tp.price, tp.tenantID); err != nil {
				return err
			}
		}

		clientHash, _ := auth.HashPassword("test-client-secret")
		if _, err := pool.Exec(ctx,
			`INSERT INTO oauth_clients (id, hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6, '{}', false)
			 ON CONFLICT (id) DO NOTHING`,
			"test-client", []byte(clientHash), []string{"http://localhost:23400/callback"},
			[]string{"authorization_code", "client_credentials", "refresh_token"},
			[]string{"code"}, "openid profile"); err != nil {
			return err
		}

		svcCtx.InitOAuth(pool)
		return nil
	})

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func chk(_ error, second error) error {
	return second
}
