package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"

	"auth-roles/internal/handler"
	"auth-roles/internal/svc"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

type TenantProduct struct {
	ID       string  `db:"id,primary,auto" json:"id"`
	Name     string  `db:"name" json:"name"`
	Price    float64 `db:"price" json:"price"`
	TenantID string  `db:"tenant_id" json:"tenant_id"`
}

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
	ctx := context.Background()

	poolURL := os.Getenv("DATABASE_URL")
	if poolURL == "" {
		poolURL = "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
	}
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: poolURL})
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	svcCtx.SetPool("primary", pool)
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("pool ping: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer', created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY, key_hash TEXT UNIQUE NOT NULL, label TEXT,
			role TEXT NOT NULL, enabled BOOLEAN DEFAULT true, created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS products (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, name TEXT NOT NULL,
			description TEXT DEFAULT '', price DECIMAL(10,2) DEFAULT 0,
			visibility TEXT DEFAULT 'public', created_by TEXT,
			deleted_at TIMESTAMPTZ, updated_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS tenant_products (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, name TEXT NOT NULL,
			price DECIMAL(10,2) DEFAULT 0, tenant_id TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, product_id TEXT,
			action TEXT NOT NULL, changed_by TEXT NOT NULL,
			old_value JSONB, new_value JSONB, created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS failed_logins (
			username TEXT PRIMARY KEY, attempts INT DEFAULT 1,
			last_attempt TIMESTAMPTZ DEFAULT now(), locked_until TIMESTAMPTZ);`,
		`CREATE TABLE IF NOT EXISTS revoked_tokens (
			token_hash TEXT PRIMARY KEY, revoked_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS mfa_secrets (
			user_id TEXT PRIMARY KEY, secret TEXT NOT NULL,
			enabled BOOLEAN DEFAULT false, created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS email_verifications (
			user_id TEXT PRIMARY KEY, token TEXT NOT NULL,
			verified BOOLEAN DEFAULT false, created_at TIMESTAMPTZ DEFAULT now(),
			expires_at TIMESTAMPTZ NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS password_resets (
			user_id TEXT PRIMARY KEY, token TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT now(), expires_at TIMESTAMPTZ NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS auth_codes (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id TEXT, code TEXT NOT NULL,
			purpose TEXT NOT NULL DEFAULT 'access',
			delivered_to TEXT, delivery_method TEXT NOT NULL DEFAULT 'console',
			expires_at TIMESTAMPTZ NOT NULL,
			attempts INT DEFAULT 0, used BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS linked_accounts (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id TEXT REFERENCES users(id),
			provider TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			email TEXT,
			UNIQUE(provider, provider_id));`,
		`CREATE TABLE IF NOT EXISTS webauthn_users (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id TEXT NOT NULL REFERENCES users(id),
			handle BYTEA NOT NULL UNIQUE,
			created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS webauthn_credentials (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id TEXT NOT NULL REFERENCES users(id),
			kid BYTEA NOT NULL,
			public_key BYTEA NOT NULL,
			attestation_type TEXT NOT NULL,
			attestation_format TEXT NOT NULL,
			transport TEXT DEFAULT '',
			sign_count BIGINT DEFAULT 0,
			aaguid BYTEA,
			clone_warning BOOLEAN DEFAULT false,
			attachment TEXT DEFAULT '',
			flags BYTEA,
			present BOOLEAN DEFAULT false,
			verified BOOLEAN DEFAULT false,
			backup_eligible BOOLEAN DEFAULT false,
			backup_state BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT now(),
			UNIQUE(kid));`,
		`CREATE TABLE IF NOT EXISTS webauthn_sessions (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			user_id TEXT,
			ceremony_type TEXT NOT NULL,
			session_data JSONB NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			id TEXT PRIMARY KEY,
			hashed_secret BYTEA NOT NULL,
			redirect_uris TEXT[] DEFAULT '{}',
			grant_types TEXT[] DEFAULT '{}',
			response_types TEXT[] DEFAULT '{}',
			scopes TEXT DEFAULT '',
			audience TEXT[] DEFAULT '{}',
			is_public BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT now());`,
		`CREATE TABLE IF NOT EXISTS oauth_sessions (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			signature TEXT NOT NULL,
			type TEXT NOT NULL,
			request_id TEXT,
			client_id TEXT NOT NULL,
			requested_scopes TEXT[] DEFAULT '{}',
			granted_scopes TEXT[] DEFAULT '{}',
			requested_audience TEXT[] DEFAULT '{}',
			granted_audience TEXT[] DEFAULT '{}',
			session_data JSONB,
			form JSONB,
			lang TEXT DEFAULT '',
			active BOOLEAN DEFAULT true,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT now(),
			UNIQUE(signature, type));`,
		`CREATE TABLE IF NOT EXISTS oauth_jti_blacklist (
			jti TEXT PRIMARY KEY,
			expires_at TIMESTAMPTZ NOT NULL);`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			log.Fatalf("init table: %v", err)
		}
	}

	seedPass := os.Getenv("SEED_PASSWORD")
	if seedPass == "" {
		seedPass = "pass123"
	}
	hashKey := func(raw string) string {
		h := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(h[:])
	}
	for _, u := range []struct{ id, name, pass, role string }{
		{"user-viewer", "viewer", seedPass, "viewer"},
		{"user-editor", "editor", seedPass, "editor"},
		{"user-admin", "admin", seedPass, "admin"},
	} {
		h, _ := svcCtx.HashPassword(u.pass)
		_, _ = pool.Exec(ctx,
			`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			u.id, u.name, h, u.role)
	}
	for _, k := range []struct{ id, label, key, role string }{
		{"key-viewer", "viewer-key", "sk-viewer_abc123", "viewer"},
		{"key-editor", "editor-key", "sk-editor_abc123", "editor"},
		{"key-admin", "admin-key", "sk-admin_abc123", "admin"},
	} {
		_, _ = pool.Exec(ctx,
			`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			k.id, k.label, hashKey(k.key), k.role)
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
		_, _ = pool.Exec(ctx,
			`INSERT INTO tenant_products (id, name, price, tenant_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			tp.id, tp.name, tp.price, tp.tenantID)
	}

	// Seed OAuth2 client
	clientHash, _ := svcCtx.HashPassword("test-client-secret")
	_, _ = pool.Exec(ctx,
		`INSERT INTO oauth_clients (id, hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public)
		 VALUES ($1, $2, $3, $4, $5, $6, '{}', false)
		 ON CONFLICT (id) DO NOTHING`,
		"test-client", []byte(clientHash), []string{"http://localhost:23400/callback"},
		[]string{"authorization_code", "client_credentials", "refresh_token"},
		[]string{"code"}, "openid profile")

	svcCtx.InitOAuth(pool)

	s.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return handler.ValidateJWT(a, roles, permissions)
	})

	s.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		h := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(h[:])
		pgPool := svcCtx.Pool("primary")
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
		pool := svcCtx.Pool("primary")
		if pool == nil {
			return false
		}
		hash := tokenHash(rawToken)
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
		table, tErr := db.NewTable[TenantProduct](pgPool, "tenant_products")
		if tErr != nil {
			log.Fatalf("tenant table: %v", tErr)
		}
		return runtime.NewCRUDProvider(table, nil)
	})

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
