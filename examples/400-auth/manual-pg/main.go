package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := loadConfig()
	svc, err := runtime.NewFromYAML(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	poolURL := "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
	if u := os.Getenv("DATABASE_URL"); u != "" {
		poolURL = u
	}
	pool, err := initPool(context.Background(), poolURL)
	if err != nil {
		log.Fatalf("pool: %v", err)
	}

	mustInitSchema(pool)
	mustSeedData(pool)

	svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
		return validateJWT(ctx, auth, roles, permissions)
	})

	svc.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
		return resolveAPIKey(ctx, pool, key)
	})

	store := newProductStore(pool)

	svc.WithRest("loginHandler", func(c *runtime.RestCtx) error {
		return handleLogin(c, pool)
	})
	svc.WithRest("listProducts", func(c *runtime.RestCtx) error { return store.list(c) })
	svc.WithRest("createProduct", func(c *runtime.RestCtx) error { return store.create(c) })
	svc.WithRest("getProduct", func(c *runtime.RestCtx) error { return store.get(c) })
	svc.WithRest("updateProduct", func(c *runtime.RestCtx) error { return store.update(c) })
	svc.WithRest("deleteProduct", func(c *runtime.RestCtx) error { return store.delete(c) })
	svc.WithRest("hardDeleteProduct", func(c *runtime.RestCtx) error { return store.hardDelete(c) })
	svc.WithRest("setVisibility", func(c *runtime.RestCtx) error { return store.setVisibility(c) })
	svc.WithRest("getAuditLog", func(c *runtime.RestCtx) error { return store.getAuditLog(c) })
	svc.WithRest("signupHandler", func(c *runtime.RestCtx) error { return handleSignup(c, pool) })
	svc.WithRest("profileHandler", func(c *runtime.RestCtx) error { return handleProfile(c, pool) })
	svc.WithRest("listUsers", func(c *runtime.RestCtx) error { return handleListUsers(c, pool) })
	svc.WithRest("deleteUser", func(c *runtime.RestCtx) error { return handleDeleteUser(c, pool) })
	svc.WithRest("setUserRole", func(c *runtime.RestCtx) error { return handleSetUserRole(c, pool) })
	svc.WithRest("rateLimitedHandler", func(c *runtime.RestCtx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	svc.WithRest("refreshHandler", func(c *runtime.RestCtx) error {
		auth := getAuth(c)
		if auth == nil {
			return c.Status(401).JSON(fiber.Map{"code": 401, "message": "unauthorized"})
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":   auth.UserID,
			"roles": auth.Roles,
			"exp":   time.Now().Add(15 * time.Minute).Unix(),
		})
		signed, err := token.SignedString([]byte("dev-secret-hs256-change-in-prod"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"code": 500, "message": "token generation failed"})
		}
		c.SetCookie(&fiber.Cookie{
			Name:     "token",
			Value:    signed,
			Path:     "/",
			HTTPOnly: true,
			Secure:   true,
			SameSite: "Strict",
			MaxAge:   900,
		})
		return c.JSON(fiber.Map{"access_token": signed, "token_type": "Bearer", "expires_in": 900})
	})

	log.Fatal(svc.Run())
}

func loadConfig() []byte {
	cfg := `name: auth-roles
port: 23400

server:
  host: "0.0.0.0"
  prefork: true
  timeout: 30s
  body_limit: 4194304
  max_conns: 10000
  health_path: /healthz
  api_prefix: /api/v1
  shutdown_timeout: 10s
  middleware:
    - path: "/*"
      apply: [logger]
  security:
    encrypt_cookie:
      enabled: true
      key: "diPHoCg5vhBrTHCSJhlud1RRMRFpRo+4N/d32S+48t8="
      except:
        - "csrf_token"

databases:
  - name: primary
    url: "postgres://postgres:postgres@postgres:5432/auth_roles?sslmode=disable"
    pool:
      max_conns: 50
      min_conns: 2
      reserved_conns: 5

auth:
  enabled: true
  driver: manual
  secret: "dev-secret-hs256-change-in-prod"
  algorithm: HS256

entry:
  - type: rest
    method: POST
    path: /login
    handler: loginHandler

  - type: rest
    method: POST
    path: /signup
    handler: signupHandler

  - type: rest
    method: GET
    path: /profile
    handler: profileHandler
    auth_modes: [jwt]

  - type: rest
    method: GET
    path: /products
    handler: listProducts
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: POST
    path: /products
    handler: createProduct
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: GET
    path: /products/:id
    handler: getProduct
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: PATCH
    path: /products/:id
    handler: updateProduct
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: DELETE
    path: /products/:id
    handler: deleteProduct
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: DELETE
    path: /admin/products/:id/hard
    handler: hardDeleteProduct
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: PATCH
    path: /admin/products/:id/visibility
    handler: setVisibility
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: GET
    path: /admin/products/:id/audit
    handler: getAuditLog
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: GET
    path: /admin/users
    handler: listUsers
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: DELETE
    path: /admin/users/:id
    handler: deleteUser
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: PATCH
    path: /admin/users/:id/role
    handler: setUserRole
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: POST
    path: /auth/refresh
    handler: refreshHandler
    auth_modes: [jwt]

  - type: rest
    method: POST
    path: /rate-limited
    handler: rateLimitedHandler
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"
    rate_limit:
      requests_per_second: 1
      burst: 2
`
	if os.Getenv("DOCKER_TEST") != "1" {
		return []byte(cfg)
	}
	return []byte(cfg)
}

func initPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 20; i++ {
		pool, err = pgxpool.New(ctx, url)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool, nil
			}
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("pool: %w (after retries)", err)
}

func mustInitSchema(pool *pgxpool.Pool) {
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    key_hash TEXT UNIQUE NOT NULL,
    label TEXT,
    role TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    price DECIMAL(10,2) DEFAULT 0,
    visibility TEXT DEFAULT 'public',
    created_by TEXT,
    deleted_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    product_id TEXT,
    action TEXT NOT NULL,
    changed_by TEXT NOT NULL,
    old_value JSONB,
    new_value JSONB,
    created_at TIMESTAMPTZ DEFAULT now()
);
`
	_, err := pool.Exec(context.Background(), schema)
	if err != nil {
		log.Fatalf("init schema: %v", err)
	}
}

func mustSeedData(pool *pgxpool.Pool) {
	pass := func(raw string) string {
		b, _ := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.MinCost)
		return string(b)
	}
	hashKey := func(raw string) string {
		h := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(h[:])
	}

	users := []struct{ id, username, password, role string }{
		{"user-viewer", "viewer", "pass123", "viewer"},
		{"user-editor", "editor", "pass123", "editor"},
		{"user-admin", "admin", "pass123", "admin"},
	}
	for _, u := range users {
		_, _ = pool.Exec(context.Background(),
			`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			u.id, u.username, pass(u.password), u.role)
	}

	keys := []struct{ id, label, key, role string }{
		{"key-viewer", "viewer-key", "sk-viewer_abc123", "viewer"},
		{"key-editor", "editor-key", "sk-editor_abc123", "editor"},
		{"key-admin", "admin-key", "sk-admin_abc123", "admin"},
	}
	for _, k := range keys {
		_, _ = pool.Exec(context.Background(),
			`INSERT INTO api_keys (id, label, key_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			k.id, k.label, hashKey(k.key), k.role)
	}
}

func validateJWT(_ context.Context, auth *middleware.AuthContext, requiredRoles, _ []string) error {
	if len(requiredRoles) == 0 {
		return nil
	}
	for _, r := range auth.Roles {
		if r == requiredRoles[0] || roleInherits(r, requiredRoles[0]) {
			return nil
		}
	}
	return errors.New("insufficient role")
}

func resolveAPIKey(ctx context.Context, pool *pgxpool.Pool, key string) (*middleware.AuthContext, error) {
	h := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(h[:])
	var id, role string
	var enabled bool
	err := pool.QueryRow(ctx,
		`SELECT id, role, enabled FROM api_keys WHERE key_hash = $1`, keyHash).
		Scan(&id, &role, &enabled)
	if err != nil {
		return nil, errors.New("invalid API key")
	}
	if !enabled {
		return nil, errors.New("API key disabled")
	}
	return &middleware.AuthContext{
		UserID: id,
		Roles:  []string{role},
	}, nil
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

func handleLogin(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
	}
	var userID, passwordHash, role string
	err := pool.QueryRow(c.Context(),
		`SELECT id, password_hash, role FROM users WHERE username = $1`, body.Username).
		Scan(&userID, &passwordHash, &role)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"code": 401, "message": "invalid credentials"})
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)) != nil {
		return c.Status(401).JSON(fiber.Map{"code": 401, "message": "invalid credentials"})
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"roles": []string{role},
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("dev-secret-hs256-change-in-prod"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "token generation failed"})
	}
	c.SetCookie(&fiber.Cookie{
		Name:     "token",
		Value:    signed,
		Path:     "/",
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		MaxAge:   900,
	})
	return c.JSON(fiber.Map{"token": signed, "role": role})
}

func handleSignup(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
	}
	if body.Username == "" || body.Password == "" {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "username and password required"})
	}
	if body.Role == "" {
		body.Role = "viewer"
	}
	allowedRoles := []string{"viewer", "editor", "admin"}
	if !slices.Contains(allowedRoles, body.Role) {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid role (use viewer, editor, or admin)"})
	}
	userID := "user-" + body.Username
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.MinCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": "internal error"})
	}
	_, err = pool.Exec(c.Context(),
		`INSERT INTO users (id, username, password_hash, role) VALUES ($1,$2,$3,$4) ON CONFLICT (username) DO NOTHING`,
		userID, body.Username, string(passwordHash), body.Role)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"status": "created", "username": body.Username, "role": body.Role})
}

func handleProfile(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(fiber.Map{"code": 401, "message": "unauthorized"})
	}
	var username, role string
	err := pool.QueryRow(c.Context(), `SELECT username, role FROM users WHERE id = $1`, auth.UserID).Scan(&username, &role)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"code": 404, "message": "user not found"})
	}
	return c.JSON(fiber.Map{"username": username, "role": role, "user_id": auth.UserID})
}

func handleListUsers(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	rows, err := pool.Query(c.Context(), `SELECT id, username, role FROM users ORDER BY username`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	defer rows.Close()
	type user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	var users []user
	for rows.Next() {
		var u user
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			break
		}
		users = append(users, u)
	}
	if users == nil {
		users = []user{}
	}
	return c.JSON(fiber.Map{"data": users})
}

func handleDeleteUser(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(fiber.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == auth.UserID {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "cannot delete yourself"})
	}
	_, err := pool.Exec(c.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "deleted"})
}

func handleSetUserRole(c *runtime.RestCtx, pool *pgxpool.Pool) error {
	auth := getAuth(c)
	if auth == nil {
		return c.Status(401).JSON(fiber.Map{"code": 401, "message": "unauthorized"})
	}
	id := c.Params("id")
	if id == auth.UserID {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "cannot change your own role"})
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
	}
	allowedRoles := []string{"viewer", "editor", "admin"}
	if !slices.Contains(allowedRoles, body.Role) {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid role (use viewer, editor, admin)"})
	}
	_, err := pool.Exec(c.Context(), `UPDATE users SET role = $1 WHERE id = $2`, body.Role, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "role_updated"})
}

func getAuth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

type productStore struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
}

func newProductStore(pool *pgxpool.Pool) *productStore {
	return &productStore{pool: pool}
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
	rows, err := s.pool.Query(c.Context(),
		`SELECT id, name, description, price, visibility, created_by, deleted_at, updated_at
		 FROM products ORDER BY updated_at DESC`)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	defer rows.Close()
	type product struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Price       string `json:"price"`
		Visibility  string `json:"visibility"`
		CreatedBy   string `json:"created_by"`
		UpdatedAt   string `json:"updated_at"`
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
	return c.JSON(fiber.Map{"data": products})
}

func (s *productStore) get(c *runtime.RestCtx) error {
	auth := s.auth(c)
	id := c.Params("id")
	var idOut, name, description, visibility, createdBy string
	var price float64
	var deletedAt *time.Time
	var updatedAt time.Time
	err := s.pool.QueryRow(c.Context(),
		`SELECT id, name, description, price, visibility, created_by, deleted_at, updated_at
		 FROM products WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&idOut, &name, &description, &price, &visibility, &createdBy, &deletedAt, &updatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"code": 404, "message": err.Error()})
	}
	if visibility == "confidential" && !s.hasRole(auth, "admin") {
		description = "[restricted]"
	}
	return c.JSON(fiber.Map{
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
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "name required"})
	}
	var id string
	err := s.pool.QueryRow(c.Context(),
		`INSERT INTO products (name, description, price, created_by) VALUES ($1,$2,$3,$4) RETURNING id`,
		body.Name, body.Description, body.Price, auth.UserID).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "create", auth.UserID, nil, fiber.Map{"name": body.Name, "price": body.Price})
	return c.Status(201).JSON(fiber.Map{"id": id})
}

func (s *productStore) update(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "editor") {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	var body struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Price       *float64 `json:"price"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
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
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "no fields to update"})
	}
	fields = append(fields, "updated_at = now()")
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE products SET %s WHERE id = $%d AND deleted_at IS NULL`, strings.Join(fields, ", "), argIdx)
	_, err := s.pool.Exec(c.Context(), q, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "update", auth.UserID, nil, nil)
	return c.JSON(fiber.Map{"status": "updated"})
}

func (s *productStore) delete(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "editor") {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	_, err := s.pool.Exec(c.Context(),
		`UPDATE products SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "soft_delete", auth.UserID, nil, nil)
	return c.JSON(fiber.Map{"status": "deleted"})
}

func (s *productStore) hardDelete(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	_, err := s.pool.Exec(c.Context(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "hard_delete", auth.UserID, nil, nil)
	return c.JSON(fiber.Map{"status": "hard_deleted"})
}

func (s *productStore) setVisibility(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := c.Bind(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "invalid body"})
	}
	allowed := []string{"public", "internal", "confidential"}
	if !slices.Contains(allowed, body.Visibility) {
		return c.Status(400).JSON(fiber.Map{"code": 400, "message": "visibility must be public, internal, or confidential"})
	}
	var oldVis string
	_ = s.pool.QueryRow(c.Context(), `SELECT visibility FROM products WHERE id = $1`, id).Scan(&oldVis)
	_, err := s.pool.Exec(c.Context(),
		`UPDATE products SET visibility = $1, updated_at = now() WHERE id = $2`, body.Visibility, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
	}
	s.logAudit(c, id, "set_visibility", auth.UserID,
		fiber.Map{"visibility": oldVis}, fiber.Map{"visibility": body.Visibility})
	return c.JSON(fiber.Map{"status": "visibility_updated"})
}

func (s *productStore) getAuditLog(c *runtime.RestCtx) error {
	auth := s.auth(c)
	if !s.hasRole(auth, "admin") {
		return c.Status(403).JSON(fiber.Map{"code": 403, "message": "forbidden"})
	}
	id := c.Params("id")
	rows, err := s.pool.Query(c.Context(),
		`SELECT id, product_id, action, changed_by, old_value, new_value, created_at
		 FROM audit_log WHERE product_id = $1 ORDER BY created_at DESC`, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
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
	return c.JSON(fiber.Map{"data": entries})
}

func (s *productStore) logAudit(c *runtime.RestCtx, productID, action, changedBy string, oldVal, newVal any) {
	oldJSON := marshalJSON(oldVal)
	newJSON := marshalJSON(newVal)
	_, _ = s.pool.Exec(c.Context(),
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
