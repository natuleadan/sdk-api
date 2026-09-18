package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"auth-roles/internal/handler"
	"auth-roles/internal/models"
	"auth-roles/internal/svc"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}

	// Identity is Zitadel. When a machine key is provided, bootstrap the
	// introspection client automatically (no manual setup) and expose its
	// credentials to the YAML through the environment.
	if mkPath := os.Getenv("ZITADEL_MACHINEKEY"); mkPath != "" {
		boot, berr := bootstrapZitadel(os.Getenv("ZITADEL_URL"), mkPath)
		if berr != nil {
			log.Fatalf("zitadel bootstrap: %v", berr)
		}
		if err := os.Setenv("ZITADEL_INTROSPECTION_URL", boot.Issuer+"/oauth/v2/introspect"); err != nil {
			log.Fatalf("zitadel introspection url: %v", err)
		}
		_ = os.Setenv("ZITADEL_INTROSPECTION_CLIENT_ID", boot.ClientID)
		_ = os.Setenv("ZITADEL_INTROSPECTION_CLIENT_SECRET", boot.ClientSecret)
		bootFile := os.Getenv("ZITADEL_BOOTSTRAP_FILE")
		if bootFile == "" {
			bootFile = ".runtime/zitadel.json"
		}
		if err := writeBootstrapInfo(bootFile, boot); err != nil {
			log.Fatalf("zitadel bootstrap file: %v", err)
		}
	}

	// Identity comes from Zitadel; authorization from OpenFGA. The store must
	// exist before the service boots, so create it when not provided and export
	// its id for the YAML (openfga_store: "${OPENFGA_STORE}").
	if os.Getenv("OPENFGA_STORE") == "" {
		store, err := ensureStore(os.Getenv("OPENFGA_URL"), "auth-openfga-zitadel")
		if err != nil {
			log.Fatalf("openfga store: %v", err)
		}
		if err := os.Setenv("OPENFGA_STORE", store); err != nil {
			log.Fatalf("openfga store env: %v", err)
		}
	}

	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svcCtx := svc.NewServiceContext()
	if fga, ferr := openfga.NewClient(openfga.Config{
		APIURL:  os.Getenv("OPENFGA_URL"),
		StoreID: os.Getenv("OPENFGA_STORE"),
	}); ferr == nil {
		svcCtx.SetFGA(fga)
	}

	// API keys are a machine credential: FGA authorizes the subject, not a
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
			{func() error { t, e := db.NewTable[models.User](pool, "users"); return chk(e, t.AutoInit(ctx)) }, "users"},
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

// ensureStore returns the id of the named OpenFGA store, creating it if absent.
func ensureStore(baseURL, name string) (string, error) {
	if baseURL == "" {
		return "", errors.New("OPENFGA_URL not set")
	}
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(baseURL + "/stores")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var list struct {
		Stores []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stores"`
	}
	_ = json.Unmarshal(body, &list)
	for _, s := range list.Stores {
		if s.Name == name {
			return s.ID, nil
		}
	}

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/stores", bytes.NewReader([]byte(`{"name":"`+name+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &created)
	if created.ID == "" {
		return "", errors.New("failed to create store: " + string(body))
	}
	return created.ID, nil
}

func chk(_ error, second error) error {
	return second
}
