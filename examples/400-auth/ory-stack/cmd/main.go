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
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}

	// Identity is Ory Kratos. Bootstrap an identity with a password (no manual
	// Ory setup) and expose the endpoints to the YAML through the environment.
	kratosAdmin := os.Getenv("KRATOS_ADMIN_URL")
	kratosPublic := os.Getenv("KRATOS_PUBLIC_URL")
	if kratosPublic == "" {
		kratosPublic = "http://localhost:14433"
	}
	if kratosAdmin != "" {
		boot, berr := bootstrapOry(kratosAdmin, "admin@example.com", "Admin123!")
		if berr != nil {
			log.Fatalf("ory bootstrap: %v", berr)
		}
		boot.KetoURL = os.Getenv("KETO_READ_URL")
		bootFile := os.Getenv("ORY_BOOTSTRAP_FILE")
		if bootFile == "" {
			bootFile = ".runtime/ory.json"
		}
		if err := writeOryBootstrap(bootFile, boot); err != nil {
			log.Fatalf("ory bootstrap file: %v", err)
		}
	}

	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svcCtx := svc.NewServiceContext()
	// Authorization is 100% Ory Keto; the client also manages Kratos identities
	// and assigns roles.
	svcCtx.SetOry(ory.NewClient(ory.Config{
		KratosPublicURL:    kratosPublic,
		KratosAdminURL:     kratosAdmin,
		KetoReadURL:        os.Getenv("KETO_READ_URL"),
		KetoWriteURL:       os.Getenv("KETO_WRITE_URL"),
		RoleNamespace:      "roles",
		RoleRelation:       "assignee",
		PermissionRelation: "perform",
	}))

	// API keys are a machine credential: Keto authorizes the subject, not a
	// local role column.
	s.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		pool := s.PoolPGTyped("primary")
		var id string
		var enabled bool
		if err := pool.QueryRow(ctx, `SELECT id, enabled FROM api_keys WHERE key_hash = $1`, auth.TokenHash(key)).
			Scan(&id, &enabled); err != nil {
			return nil, errors.New("invalid API key")
		}
		if !enabled {
			return nil, errors.New("API key disabled")
		}
		return &middleware.AuthContext{UserID: id}, nil
	})

	s.WithRateLimitMaxFunc(func(c *runtime.RestCtx) int {
		if c.Get("X-Debug") == "true" {
			return 5
		}
		return 0
	})

	handler.RegisterRestRoutes(s, svcCtx)

	s.WithCRUDFactory("TenantProduct", func() runtime.CRUDProvider {
		table, tErr := db.NewTable[models.TenantProduct](s.PoolPGTyped("primary"), "tenant_products")
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
			{func() error { t, e := db.NewTable[models.APIKey](pool, "api_keys"); return chk(e, t.AutoInit(ctx)) }, "api_keys"},
			{func() error { t, e := db.NewTable[models.Product](pool, "products"); return chk(e, t.AutoInit(ctx)) }, "products"},
			{func() error {
				t, e := db.NewTable[models.TenantProduct](pool, "tenant_products")
				return chk(e, t.AutoInit(ctx))
			}, "tenant_products"},
			{func() error { t, e := db.NewTable[models.AuditLog](pool, "audit_log"); return chk(e, t.AutoInit(ctx)) }, "audit_log"},
		}
		for _, tbl := range tables {
			if err := tbl.fn(); err != nil {
				log.Fatalf("autoinit %s: %v", tbl.msg, err)
			}
		}

		if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS auth_teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
)`); err != nil {
			log.Fatalf("autoinit auth_teams: %v", err)
		}

		for _, p := range []struct {
			id, name, desc, vis string
			price               float64
		}{
			{"prod-1", "Alfa One", "public item", "public", 10},
			{"prod-2", "Beta Two", "internal item", "internal", 20},
			{"prod-3", "Gamma Three", "confidential item", "confidential", 30},
		} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO products (id, name, description, price, visibility, created_by) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
				p.id, p.name, p.desc, p.price, p.vis, "seed"); err != nil {
				return err
			}
		}

		for _, k := range []struct{ id, label, key string }{
			{"key-reader", "reader-key", "sk-reader_abc123"},
			{"key-editor", "editor-key", "sk-editor_abc123"},
		} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,'') ON CONFLICT (id) DO NOTHING`,
				k.id, k.label, auth.TokenHash(k.key)); err != nil {
				return err
			}
		}

		for _, tp := range []struct {
			id, name string
			price    float64
			tenant   string
		}{
			{"tp-alfa-1", "Alfa One", 10, "org-alfa"},
			{"tp-alfa-2", "Alfa Two", 20, "org-alfa"},
			{"tp-beta-1", "Beta One", 30, "org-beta"},
		} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO tenant_products (id, name, price, tenant_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				tp.id, tp.name, tp.price, tp.tenant); err != nil {
				return err
			}
		}
		return nil
	})

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func chk(_ error, second error) error {
	return second
}
