package svc

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct {
	JWTSecret        string
	AuthExpiry       int
	Store            *ProductStore
	LockoutThreshold int
	LockoutDuration  time.Duration
	OAuth            *OAuthProvider
	SMSProvider      SMSProvider
	pools            map[string]*pgxpool.Pool
}

func NewServiceContext() *ServiceContext {
	return &ServiceContext{
		JWTSecret:        envOrDefault("JWT_SECRET", "dev-secret-hs256-change-in-prod"),
		AuthExpiry:       envIntOrDefault("AUTH_EXPIRY", 900),
		LockoutThreshold: 5,
		LockoutDuration:  15 * time.Minute,
		Store:            newProductStore(),
		SMSProvider:      NewSMSProvider(),
	}
}

func (s *ServiceContext) InitOAuth(pool *pgxpool.Pool) {
	s.OAuth = NewOAuthProvider(pool, s.JWTSecret)
}

func (s *ServiceContext) SetPool(name string, pool *pgxpool.Pool) {
	if s.pools == nil {
		s.pools = make(map[string]*pgxpool.Pool)
	}
	s.pools[name] = pool
}

func (s *ServiceContext) Pool(name string) *pgxpool.Pool {
	if s.pools == nil {
		return nil
	}
	return s.pools[name]
}

func (s *ServiceContext) HashPassword(password string) (string, error) {
	return auth.HashPassword(password)
}

func (s *ServiceContext) VerifyPassword(hash, password string) bool {
	return auth.VerifyPassword(hash, password)
}

func getAuth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
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

var roleHierarchy = auth.RoleHierarchy{
	"viewer": {},
	"editor": {"viewer"},
	"admin":  {"editor", "viewer"},
}

type ProductStore struct {
	mu sync.RWMutex
}

func newProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) auth(c *runtime.RestCtx) *middleware.AuthContext {
	if a, ok := c.Locals("auth").(*middleware.AuthContext); ok {
		return a
	}
	return nil
}

func (s *ProductStore) hasRole(a *middleware.AuthContext, role string) bool {
	if a == nil {
		return false
	}
	for _, r := range a.Roles {
		if r == role || roleHierarchy.Inherits(r, role) {
			return true
		}
	}
	return false
}

func (s *ProductStore) List(c *runtime.RestCtx) error {
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

func (s *ProductStore) Get(c *runtime.RestCtx) error {
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

func (s *ProductStore) Create(c *runtime.RestCtx) error {
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

func (s *ProductStore) Update(c *runtime.RestCtx) error {
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

func (s *ProductStore) Delete(c *runtime.RestCtx) error {
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

func (s *ProductStore) HardDelete(c *runtime.RestCtx) error {
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

func (s *ProductStore) SetVisibility(c *runtime.RestCtx) error {
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

func (s *ProductStore) GetAuditLog(c *runtime.RestCtx) error {
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

func (s *ProductStore) logAudit(c *runtime.RestCtx, productID, action, changedBy string, oldVal, newVal any) {
	oldJSON := marshalJSON(oldVal)
	newJSON := marshalJSON(newVal)
	pool := c.PoolPG("primary")
	_, _ = pool.Exec(c.Context(),
		`INSERT INTO audit_log (product_id, action, changed_by, old_value, new_value) VALUES ($1,$2,$3,$4,$5)`,
		productID, action, changedBy, oldJSON, newJSON)
}
