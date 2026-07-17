package main

import (
	"context"
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
	ID       string  `db:"id,primary"`
	Name     string  `db:"name"`
	Price    float64 `db:"price"`
	TenantID string  `db:"tenant_id"`
}

var (
	jwtSecret  = envOrDefault("JWT_SECRET", "dev-secret-hs256-change-in-prod")
	authExpiry = envIntOrDefault("AUTH_EXPIRY", 900)
	seedPass   = envOrDefault("SEED_PASSWORD", "pass123")
)

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

	// init pool with retry + schema + seed
	// p is inferred as *pgxpool.Pool from db.NewPool — no pgxpool import needed
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
		// seed users
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
		// seed api keys
		for _, k := range []struct{ id, label, key, role string }{
			{"key-viewer", "viewer-key", "sk-viewer_abc123", "viewer"},
			{"key-editor", "editor-key", "sk-editor_abc123", "editor"},
			{"key-admin", "admin-key", "sk-admin_abc123", "admin"},
		} {
			_, _ = p.Exec(ctx,
				`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
				k.id, k.label, hashKey(k.key), k.role)
		}
		// seed tenant products
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
		pgPool := svc.PoolPGTyped("default")
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
	svc.WithRest("profileHandler", func(c *runtime.RestCtx) error { return handleProfile(c) })
	svc.WithRest("listProducts", func(c *runtime.RestCtx) error { return store.list(c) })
	svc.WithRest("createProduct", func(c *runtime.RestCtx) error { return store.create(c) })
	svc.WithRest("getProduct", func(c *runtime.RestCtx) error { return store.get(c) })
	svc.WithRest("updateProduct", func(c *runtime.RestCtx) error { return store.update(c) })
	svc.WithRest("deleteProduct", func(c *runtime.RestCtx) error { return store.delete(c) })
	svc.WithRest("hardDeleteProduct", func(c *runtime.RestCtx) error { return store.hardDelete(c) })
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
		pgPool := svc.PoolPGTyped("default")
		table, tErr := db.NewTable[TenantProduct](pgPool, "tenant_products")
		if tErr != nil {
			log.Fatalf("tenant table: %v", tErr)
		}
		return runtime.NewCRUDProvider(table, nil)
	})

	log.Fatal(svc.Run())
}

func validateJWT(_ context.Context, auth *middleware.AuthContext, requiredRoles, requiredPermissions []string) error {
	if len(requiredRoles) > 0 {
		allowed := false
		for _, r := range auth.Roles {
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
		for _, p := range auth.Permissions {
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

func handleLogin(c *runtime.RestCtx) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
	}
	pool := c.PoolPG("default")
	var userID, passwordHash, role string
	err := pool.QueryRow(c.Context(),
		`SELECT id, password_hash, role FROM users WHERE username = $1`, body.Username).
		Scan(&userID, &passwordHash, &role)
	if err != nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid credentials"})
	}
	if !auth.VerifyPassword(passwordHash, body.Password) {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid credentials"})
	}
	var permissions []string
	if role == "admin" {
		permissions = []string{"users:manage"}
	}
	orgID := "org-alfa"
	if role == "viewer" {
		orgID = "org-beta"
	}
	claims := middleware.DefaultClaims(userID, orgID, []string{role}, permissions, authExpiry)
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
	if body.Role == "" {
		body.Role = "viewer"
	}
	allowedRoles := []string{"viewer", "editor", "admin"}
	if !slices.Contains(allowedRoles, body.Role) {
		return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid role (use viewer, editor, or admin)"})
	}
	pool := c.PoolPG("default")
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
	return c.Status(201).JSON(runtime.Map{"status": "created", "username": body.Username, "role": body.Role})
}

func handleProfile(c *runtime.RestCtx) error {
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	pool := c.PoolPG("default")
	var username, role string
	err := pool.QueryRow(c.Context(), `SELECT username, role FROM users WHERE id = $1`, auth.UserID).Scan(&username, &role)
	if err != nil {
		return c.Status(404).JSON(runtime.Map{"code": 404, "message": "user not found"})
	}
	return c.JSON(runtime.Map{"username": username, "role": role, "user_id": auth.UserID})
}

func handleListUsers(c *runtime.RestCtx) error {
	pool := c.PoolPG("default")
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
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == auth.UserID {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot delete yourself"})
	}
	pool := c.PoolPG("default")
	_, err := pool.Exec(c.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	return c.JSON(runtime.Map{"status": "deleted"})
}

func handleSetUserRole(c *runtime.RestCtx) error {
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == auth.UserID {
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
	pool := c.PoolPG("default")
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

func (s *productStore) hasRole(auth *middleware.AuthContext, role string) bool {
	if auth == nil {
		return false
	}
	for _, r := range auth.Roles {
		if r == role || roleInherits(r, role) {
			return true
		}
	}
	return false
}

func (s *productStore) list(c *runtime.RestCtx) error {
	auth := s.auth(c)
	pool := c.PoolPG("default")
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
		if p.Visibility == "confidential" && !s.hasRole(auth, "admin") {
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
	auth := s.auth(c)
	id := c.Params("id")
	pool := c.PoolPG("default")
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
	if visibility == "confidential" && !s.hasRole(auth, "admin") {
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
	auth := s.auth(c)
	if !s.hasRole(auth, "editor") {
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
	pool := c.PoolPG("default")
	var id string
	err := pool.QueryRow(c.Context(),
		`INSERT INTO products (name, description, price, created_by) VALUES ($1,$2,$3,$4) RETURNING id`,
		body.Name, body.Description, body.Price, auth.UserID).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "create", auth.UserID, nil, runtime.Map{"name": body.Name, "price": body.Price})
	return c.Status(201).JSON(runtime.Map{"id": id})
}

func (s *productStore) update(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "editor") {
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
	pool := c.PoolPG("default")
	_, err := pool.Exec(c.Context(), q, args...)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "update", auth.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "updated"})
}

func (s *productStore) delete(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "editor") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("default")
	_, err := pool.Exec(c.Context(),
		`UPDATE products SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "soft_delete", auth.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "deleted"})
}

func (s *productStore) hardDelete(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("default")
	_, err := pool.Exec(c.Context(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "hard_delete", auth.UserID, nil, nil)
	return c.JSON(runtime.Map{"status": "hard_deleted"})
}

func (s *productStore) setVisibility(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
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
	pool := c.PoolPG("default")
	var oldVis string
	_ = pool.QueryRow(c.Context(), `SELECT visibility FROM products WHERE id = $1`, id).Scan(&oldVis)
	_, err := pool.Exec(c.Context(),
		`UPDATE products SET visibility = $1, updated_at = now() WHERE id = $2`, body.Visibility, id)
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "set_visibility", auth.UserID,
		runtime.Map{"visibility": oldVis}, runtime.Map{"visibility": body.Visibility})
	return c.JSON(runtime.Map{"status": "visibility_updated"})
}

func (s *productStore) getAuditLog(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
		return c.Status(403).JSON(runtime.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	pool := c.PoolPG("default")
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
	pool := c.PoolPG("default")
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
