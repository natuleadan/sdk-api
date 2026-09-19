package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"

	"auth-roles/internal/auth"
	"auth-roles/internal/handler"
	"auth-roles/internal/models"
	"auth-roles/internal/svc"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	sdkauth "github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/auth/oauthstore"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}

	// DB-agnostic: the primary database driver selects the SQL dialect used to
	// translate the example's queries (postgres, turso, mysql, ...).
	svc.SetDriver(os.Getenv("DB_DRIVER"))
	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svcCtx := svc.NewServiceContext()

	// Role → permission table for delegated grants (wildcards allowed). Kept in
	// sync with the role_permissions seed and the YAML role_permissions map.
	rolePerms := auth.ParseRolePermissions(map[string][]string{
		"admin":               {"users:manage", "products:read"},
		"facturacion-lectura": {"users:manage"},
	})
	svcCtx.SetRolePermissions(rolePerms)

	s.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return handler.ValidateRolesDB(svc.DBOf(s), rolePerms, ctx, a, roles, permissions)
	})

	s.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		var id, role string
		var enabled bool
		if err := svc.DBOf(s).QueryRow(ctx,
			`SELECT id, role, enabled FROM api_keys WHERE key_hash = $1`, sdkauth.TokenHash(key)).
			Scan(&id, &role, &enabled); err != nil {
			return nil, errors.New("invalid API key")
		}
		if !enabled {
			return nil, errors.New("API key disabled")
		}
		return &middleware.AuthContext{UserID: id, Roles: []string{role}}, nil
	})

	s.WithJWTBlacklist(func(rawToken string) bool {
		var exists bool
		if err := svc.DBOf(s).QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_hash = $1)`, sdkauth.TokenHash(rawToken)).
			Scan(&exists); err != nil {
			return false
		}
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
		switch svc.Driver() {
		case "mysql", "mariadb":
			table, terr := db.NewMySQLTable[models.TenantProduct](s.PoolSQLTyped("primary"), "tenant_products")
			if terr != nil {
				log.Fatalf("tenant table: %v", terr)
			}
			return runtime.NewMySQLCRUDProvider(table, nil)
		case "postgres":
			table, terr := db.NewTable[models.TenantProduct](s.PoolPGTyped("primary"), "tenant_products")
			if terr != nil {
				log.Fatalf("tenant table: %v", terr)
			}
			return runtime.NewCRUDProvider(table, nil)
		default:
			table, terr := db.NewTursoTableFrom[models.TenantProduct](s.PoolSQLTyped("primary"), "tenant_products", nil)
			if terr != nil {
				log.Fatalf("tenant table: %v", terr)
			}
			return runtime.NewTursoCRUDProvider(table, nil)
		}
	})

	s.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		q := svc.DBOf(s)

		tables := []struct {
			msg string
			fn  func() error
		}{
			{"users", func() error { return autoInit[models.User](s, "users") }},
			{"api_keys", func() error { return autoInit[models.APIKey](s, "api_keys") }},
			{"products", func() error { return autoInit[models.Product](s, "products") }},
			{"tenant_products", func() error { return autoInit[models.TenantProduct](s, "tenant_products") }},
			{"audit_log", func() error { return autoInit[models.AuditLog](s, "audit_log") }},
			{"failed_logins", func() error { return autoInit[models.FailedLogin](s, "failed_logins") }},
			{"revoked_tokens", func() error { return autoInit[models.RevokedToken](s, "revoked_tokens") }},
			{"mfa_secrets", func() error { return autoInit[models.MFASecret](s, "mfa_secrets") }},
			{"email_verifications", func() error { return autoInit[models.EmailVerification](s, "email_verifications") }},
			{"password_resets", func() error { return autoInit[models.PasswordReset](s, "password_resets") }},
			{"auth_codes", func() error { return autoInit[models.AuthCode](s, "auth_codes") }},
			{"linked_accounts", func() error { return autoInit[models.LinkedAccount](s, "linked_accounts") }},
			{"webauthn_users", func() error { return autoInit[models.WebAuthnUser](s, "webauthn_users") }},
			{"webauthn_credentials", func() error { return autoInit[models.WebAuthnCredential](s, "webauthn_credentials") }},
			{"webauthn_sessions", func() error { return autoInit[models.WebAuthnSession](s, "webauthn_sessions") }},
			{"oauth_clients", func() error { return autoInit[models.OAuthClient](s, "oauth_clients") }},
			{"oauth_sessions", func() error { return autoInit[models.OAuthSession](s, "oauth_sessions") }},
			{"oauth_jti_blacklist", func() error { return autoInit[models.OAuthJTIS](s, "oauth_jti_blacklist") }},
			{"role_assignments", func() error { return autoInit[models.RoleAssignment](s, "role_assignments") }},
			{"role_permissions", func() error { return autoInit[models.RolePermission](s, "role_permissions") }},
		}
		for _, tbl := range tables {
			if err := tbl.fn(); err != nil {
				log.Fatalf("autoinit %s: %v", tbl.msg, err)
			}
		}

		// Delegated grants + teams (driver-agnostic store).
		grantStore := auth.NewSQLStore(q)
		if err := grantStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("autoinit auth_grants: %v", err)
		}

		for _, rp := range []struct{ role, perm string }{
			{"admin", "users:manage"},
			{"facturacion-lectura", "users:manage"},
		} {
			if _, err := q.Exec(ctx,
				`INSERT INTO role_permissions (role, permission) VALUES ($1,$2) ON CONFLICT (role, permission) DO NOTHING`,
				rp.role, rp.perm); err != nil {
				return err
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
			{"user-roles-admin", "rolesadmin", seedPass, "admin"},
			{"user-roles", "rolesuser", seedPass, "editor"},
			{"user-contract", "contract-role", seedPass, ""},
		} {
			h, _ := sdkauth.HashPassword(u.pass)
			if _, err := q.Exec(ctx,
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
			if _, err := q.Exec(ctx,
				`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				k.id, k.label, sdkauth.TokenHash(k.key), k.role); err != nil {
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
			if _, err := q.Exec(ctx,
				`INSERT INTO tenant_products (id, name, price, tenant_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				tp.id, tp.name, tp.price, tp.tenantID); err != nil {
				return err
			}
		}

		clientHash, _ := sdkauth.HashPassword("test-client-secret")
		if _, err := q.Exec(ctx,
			`INSERT INTO oauth_clients (id, hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, false)
			 ON CONFLICT (id) DO NOTHING`,
			"test-client", []byte(clientHash),
			oauthstore.StringSlice{"http://localhost:23400/callback"},
			oauthstore.StringSlice{"authorization_code", "client_credentials", "refresh_token"},
			oauthstore.StringSlice{"code"},
			"openid profile",
			oauthstore.StringSlice{}); err != nil {
			return err
		}

		// The OAuth2 provider runs on the same database through a *sql.DB:
		// Postgres via the pgx stdlib driver, Turso/MySQL via the opened pool.
		var oauthDB *sql.DB
		if svc.Driver() == "postgres" {
			oauthDB, err = db.OpenStdlib("postgres", os.Getenv("DATABASE_URL"))
			if err != nil {
				return err
			}
		} else {
			oauthDB = s.PoolSQLTyped("primary")
		}
		svcCtx.InitOAuth(oauthstore.NewStore(oauthDB, svc.Driver()))
		return nil
	})

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

// autoInit creates the table for T using the driver-appropriate constructor.
func autoInit[T any](s *runtime.Service, table string) error {
	ctx := context.Background()
	switch svc.Driver() {
	case "mysql", "mariadb":
		t, err := db.NewMySQLTable[T](s.PoolSQLTyped("primary"), table)
		if err != nil {
			return err
		}
		return t.AutoInit(ctx)
	case "postgres":
		t, err := db.NewTable[T](s.PoolPGTyped("primary"), table)
		if err != nil {
			return err
		}
		return t.AutoInit(ctx)
	default:
		t, err := db.NewTursoTableFrom[T](s.PoolSQLTyped("primary"), table, nil)
		if err != nil {
			return err
		}
		return t.AutoInit(ctx)
	}
}
