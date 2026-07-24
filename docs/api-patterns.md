# API Patterns

This document describes all the patterns available in sdk-api for building event-driven microservices and monoliths. Each pattern has a YAML definition and a Go handler.

---

## 1. CRUD (Auto-generated REST)

Full CRUD for a single model. Five endpoints auto-generated.

**YAML:**

```yaml
entry:
  - type: crud
    model: Product
    db: pg-main
    table: products
    path: /products
    cache: cache-main       # optional: enable Redis/Dragonfly caching
```

The `cache` field references a name from the `kv:` YAML section. Each CRUD entry can have its own cache backend.

**Go:**

```go
type Product struct {
    ID    int64   `db:"id,primary,auto" json:"id"`
    Name  string  `db:"name,required" json:"name"`
    Price float64 `db:"price" json:"price"`
}

type ProductHooks struct {
    runtime.DefaultHooks[Product]
}

func (h *ProductHooks) BeforeCreate(ctx context.Context, req Product) (Product, error) {
    // Transform/validate before DB insert
    return req, nil
}

//go:embed service.yaml
var configYAML []byte

func main() {
    svc, _ := runtime.NewFromYAML(configYAML)
    pool := svc.Pool("pg-main").(*pgxpool.Pool)
    table, _ := db.NewTable[Product](pool, "products")
    svc.WithCRUD("Product", runtime.NewCRUDProvider(table, &ProductHooks{}))
    svc.RegisterModel("Product", (*Product)(nil))
    svc.Run()
}
```

**Resulting API:**

| Method | Path | Hook integration |
|--------|------|-----------------|
| GET | `/api/v1/products` | — |
| GET | `/api/v1/products/:id` | — |
| POST | `/api/v1/products` | `BeforeCreate` → DB → `AfterCreate` |
| PATCH | `/api/v1/products/:id` | `BeforeUpdate` → DB → `AfterUpdate` |
| DELETE | `/api/v1/products/:id` | `BeforeDelete` → DB → `AfterDelete` |

---

## 2. CRUD with Overrides

Disable or replace individual CRUD endpoints.

**YAML:**

```yaml
entry:
  - type: crud
    model: Order
    db: pg-main
    table: orders
    overrides:
      list: "onCustomList"     # Replace List handler
      get: ~                   # Default (keep auto-generated)
      create: "-"              # Disable POST /orders
      update: ~                # Default
      delete: "-"              # Disable DELETE /orders/:id
```

**Go:**

```go
svc.WithRest("onCustomList", func(c *runtime.RestCtx) error {
    // Custom pagination logic
    return c.JSON(map[string]any{"data": items, "total": total})
})
```

Override values:
- `""` (empty) or `~` (null) → use auto-generated handler
- `"-"` → don't register this endpoint
- `"handlerName"` → use this handler from the Rest map

### Keyset Pagination

By default, CRUD list uses offset pagination (`LIMIT/OFFSET` + `SELECT COUNT(*)`). For large tables, keyset pagination is faster: `WHERE pk > $1 LIMIT N` (O(log N), no `COUNT(*)`).

**YAML:**
```yaml
entry:
  - type: crud
    model: Product
    path: /products
    pagination: keyset       # "offset" (default) or "keyset"
    page_size: 20            # default and min page size
    max_page_size: 100       # max allowed page size
    sortable: [id]           # allowed sort columns (empty = all)
```

**Request:**
```
GET /api/v1/products?cursor=42&size=20
```

**Response:**
```json
{
  "data": [...],
  "nextCursor": "62",
  "pageSize": 20
}
```

The client uses `nextCursor` from the response as the `cursor` parameter in the next request. No `total` field — keyset pagination does not know the total row count. This eliminates the `SELECT COUNT(*)` bottleneck on large tables.

**Limitations:**
- Sort column must be the primary key (unique, indexed). The `sort` query parameter is ignored in keyset mode.
- No `total` in response — client cannot render "Page 3 of 50".
- No random page access — only sequential next/prev.
- Use offset mode (`pagination: offset`) when you need total counts or arbitrary page jumps.

### Tenant Scoping (Multi-Tenant CRUD)

Automatically filter CRUD data by the authenticated user's tenant. The JWT must contain a tenant claim (default `org_id`).

**YAML:**
```yaml
entry:
  - type: crud
    model: Product
    path: /products
    tenant_scope: org_id        # JWT claim for tenant ID
    tenant_field: tenant_id     # DB column to filter on
```

The SDK injects a middleware that extracts `org_id` from JWT claims and appends `WHERE tenant_id = $N` to every CRUD query. Cross-tenant reads return 404 (not 403) to avoid leaking tenant existence.

**Request (with valid JWT containing `org_id: tenant-abc`):**
```
GET /api/v1/products
# Automatically becomes: SELECT * FROM products WHERE tenant_id = 'tenant-abc'
```

**Spoof prevention:** The JWT `org_id` claim is the source of truth. Requests cannot override the tenant field via the request body — the middleware silently overwrites `tenant_id` on create and ignores it on update.

**Go registration:**
```go
svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
    if auth.TenantID == "" {
        return fmt.Errorf("missing tenant")
    }
    return nil
})
```

See `examples/400-auth/manual-pg` for a full tenant-scoped CRUD implementation with tests.

---

### Redis List Pattern (Set Index)

When using Redis/Dragonfly as the primary store (not just cache), avoid `SCAN` for listing. Use a **Set Index** instead:

```go
// On create
r.SADD("link:ids", id)

// On delete
r.SREM("link:ids", id)

// List with pagination (SSCAN)
r.SSCAN("link:ids", cursor, "COUNT", size)

// Total count
r.SCARD("link:ids")
```

This separates key enumeration (O(m) over the set) from data fetching (O(1) per GET). See `examples/200-url-shortener/kv-dragonfly/` for a full implementation.

### Accessing KV from Go

Access any configured KV store via `svc.KV(name string)`, which returns a Redis-compatible client from the `kv:` YAML section:

```go
r := svc.KV("cache-main")
r.Set(ctx, "key", "value", 0)
val, _ := r.Get(ctx, "key").Result()
```

Each `kv:` entry in the YAML config is available by name. The returned client uses the same underlying connection pool defined in the YAML.

---

## 3. REST (Single Endpoint)

A single HTTP endpoint with custom handler.

**YAML:**

```yaml
entry:
  - type: rest
    method: GET
    path: /products/:id/transform
    handler: onTransformProduct
```

**Go:**

```go
svc.WithRest("onTransformProduct", func(c *runtime.RestCtx) error {
    id := c.Params("id")
    return c.JSON(map[string]any{"transformed": true, "id": id})
})
```

---

## 4. REST with Transform Hooks

Wraps a REST handler with `BeforeTransform`/`AfterTransform` hooks for input validation/output processing.

**YAML:**

```yaml
entry:
  - type: rest
    method: POST
    path: /products/convert
    handler: onConvertProduct
```

**Go:**

```go
type ProductHooks struct {
    runtime.DefaultHooks[Product]
}

func (h *ProductHooks) BeforeTransform(ctx context.Context, req Product) (Product, error) {
    req.Price = req.Price * 1.16 // Apply tax
    return req, nil
}

svc.WithRest("onConvertProduct", runtime.WrapTransformHandler(
    func(c *runtime.RestCtx) error {
        input := c.Locals("transformed").(Product)
        return c.JSON(map[string]any{"original": input.Name, "price": input.Price})
    },
    &ProductHooks{},
))
```

BeforeTransform parses the body into `T`, calls the hook, stores the result in `c.Locals("transformed")`. AfterTransform is called with the response bytes.

---

## 5. REST with NATS Publish

Auto-publishes to NATS after a successful response.

**YAML:**

```yaml
entry:
  - type: rest
    method: POST
    path: /orders
    handler: onCreateOrder
    db: pg-main
    event_publish:
      - stream: orders
        subject: orders.created
```

**Go:**

```go
svc.WithRest("onCreateOrder", func(c *runtime.RestCtx) error {
    // Handler runs first
    // If 2xx, SDK auto-publishes c.Body() to NATS
    return c.JSON(map[string]any{"orderID": "123"})
})
```

The `wrapEventPublish` wrapper publishes only when the handler returns no error and the status is < 400.

---

## 6. Webhook

HTTP endpoint that defaults to POST when no method is specified. No JWT by default. Often combined with `event_publish`.

**YAML:**

```yaml
entry:
  - type: webhook
    path: /webhooks/sendgrid
    handler: onInboundEmail
    event_publish:
      - stream: email
        subject: email.received
```

**Go:**

```go
svc.WithRest("onInboundEmail", func(c *runtime.RestCtx) error {
    // Process inbound webhook payload
    log.Printf("received: %s", string(c.Body()))
    return c.JSON(map[string]any{"received": true})
})
```

---

## 7. WebSocket

Bidirectional real-time communication.

**YAML:**

```yaml
entry:
  - type: websocket
    path: /ws/chat
    handler: onChat
    auth_modes: [jwt]
```

**Go:**

```go
svc.WithWS("onChat", func(ctx context.Context, conn *websocket.Conn) error {
    for {
        mt, msg, err := conn.ReadMessage()
        if err != nil { break }
        conn.WriteMessage(mt, msg)
    }
    return nil
})
```

---

## 8. SSE (Server-Sent Events)

Unidirectional real-time stream.

**YAML:**

```yaml
entry:
  - type: sse
    path: /events/stream
    handler: onStream
    auth_modes: [jwt]
```

**Go:**

```go
svc.WithSSE("onStream", func(ctx context.Context, send func(data string)) error {
    for i := 0; i < 10; i++ {
        send("data: event " + strconv.Itoa(i))
        send("")
        time.Sleep(1 * time.Second)
    }
    return nil
})
```

---

## 9. File Upload/Download

**YAML:**

```yaml
entry:
  - type: file
    method: POST
    path: /files/upload
    handler: onFileUpload
    allowed_types:
      - image/png
      - application/pdf
    max_size: 10MB
    storage:
      mode: local
      path: /data/uploads

  - type: file
    method: GET
    path: /files/:id/download
    handler: onFileDownload
    storage:
      mode: local
      path: /data/uploads
```

**Go:**

```go
svc.WithRest("onFileUpload", func(c fiber.Ctx) error {
    file, _ := c.FormFile("file")
    src, _ := file.Open()
    defer src.Close()
    dst, _ := os.Create("/data/uploads/" + file.Filename)
    io.Copy(dst, src)
    return c.JSON(map[string]any{"filename": file.Filename, "size": file.Size})
})

svc.WithRest("onFileDownload", func(c fiber.Ctx) error {
    id := c.Params("id")
    f, _ := os.Open("/data/uploads/" + id)
    defer f.Close()
    c.Set("Content-Disposition", `attachment; filename="`+id+`"`)
    return c.SendStream(f)
})
```

The `allowed_types` validation returns 415 if Content-Type doesn't match. `max_size` returns 413 if body exceeds limit. Supports wildcard: `image/*`.

S3 with presigned URLs, HTTP pool, and cache (from YAML):

```yaml
entry:
  - type: file
    path: /files/upload
    handler: onFileUpload
    storage:
      mode: s3
      bucket: uploads
      endpoint: http://minio:9000
      access_key: "${S3_ACCESS_KEY}"
      secret_key: "${S3_SECRET_KEY}"
      presign: true
      presign_ttl: 5m
      pool:
        max_idle_conns: 200
      cache:
        l1: ram
        l1_size: 10000
```

Access storage backends in Go via `svc.Storage(path)`:

```go
store := svc.Storage("/files/upload")
store.Upload(ctx, "key", reader, size, contentType)
data, _ := store.Download(ctx, "key")
```

When using a YAML-configured S3 storage, `presign_ttl` is auto-wired into the `S3Storage` backend. Read it at runtime via `PresignTTL()`:

```go
store := svc.Storage("/files/upload").(*server.S3Storage)
ttl := store.PresignTTL() // returns the duration from YAML
```

For presigned URLs, assert the `server.Presigner` interface:

```go
if p, ok := store.(server.Presigner); ok {
    url, _ := p.PresignURL(ctx, "uploads/file.pdf", 5*time.Minute)
}
```

Three download modes:
- **Proxy** — server reads from S3 and streams to client (default, no presign needed)
- **Presign redirect (302)** — server returns a signed S3 URL, client follows redirect
- **Sign-only JSON** — server returns signed URL as JSON, client decides how to use it

Storage backends can also be created manually:

```go
s3store, _ := server.NewS3Storage(server.S3Config{
    Endpoint:  os.Getenv("S3_ENDPOINT"),
    Bucket:    os.Getenv("S3_BUCKET"),
    AccessKey: os.Getenv("S3_ACCESS_KEY"),
    SecretKey: os.Getenv("S3_SECRET_KEY"),
})
local, _ := server.NewLocalStorage("/data/uploads")
```

---

## 10. Exit Worker (Push Consumer)

Process NATS messages as they arrive. Fire-and-forget.

**YAML:**

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
```

**Go:**

```go
svc.WithExit("onOrderConfirmed", func(ctx context.Context, msg []byte) ([]byte, error) {
    var order OrderEvent
    json.Unmarshal(msg, &order)
    // Send email...
    return nil, nil
})
```

---

## 11. Exit Worker with Reply

Process and respond to a NATS request-reply.

**YAML:**

```yaml
exit:
  - name: order-validator
    subscribe:
      stream: orders
      subject: orders.validate
    handler: onValidateOrder
    reply: true
    reply_timeout: 30s
```

**Go:**

```go
svc.WithExit("onValidateOrder", func(ctx context.Context, msg []byte) ([]byte, error) {
    var req struct { ID string `json:"id"` }
    json.Unmarshal(msg, &req)
    valid := req.ID != ""
    resp, _ := json.Marshal(map[string]any{"valid": valid})
    return resp, nil  // SDK calls msg.Respond(resp)
})
```

Publishing side (Producer):

```go
producer := events.NewProducer[MyType](nc, js, "orders.validate")
resp, err := producer.PublishAndWait(ctx, myData, 5*time.Second)
```

---

## 12. Exit Worker (Pull Consumer)

Batch-fetch messages instead of push delivery.

**YAML:**

```yaml
exit:
  - name: batch-processor
    subscribe:
      stream: orders
      subject: orders.batch
    handler: onBatch
    max_concurrent: 5
    pull_batch: 10           # Fetch 10 messages per poll
    consumer_mode: pull
```

The worker pulls messages in batches of `pull_batch` size with `PullMaxWait` timeout.

---

## 13. Cron (NATS Publish)

Publish to NATS on a schedule.

**YAML:**

```yaml
cron:
  - name: daily-report
    schedule: "0 6 * * *"
    mode: nats
    publish:
      stream: cron
      subject: cron.daily-report
```

The cron publishes an empty payload to the subject. An exit worker can listen for it:

```yaml
exit:
  - name: report-generator
    subscribe:
      stream: cron
      subject: cron.daily-report
    handler: onDailyReport
```

---

## 14. Cron (Handler)

Call a Go function on schedule.

**YAML:**

```yaml
cron:
  - name: cleanup-expired
    schedule: "0 */4 * * *"
    mode: handler
    handler: onCleanupExpired
```

**Go:**

```go
svc.WithCron("onCleanupExpired", func(ctx context.Context) error {
    // Delete expired records
    return nil
})
```

---

## 15. Cron (Internal)

System tick — logs a message, does nothing else.

```yaml
cron:
  - name: health-check
    schedule: "@every 1h"
    mode: internal
```

---

## 16. Retry (Exponential Backoff)

Per-entry retry for idempotent HTTP methods (GET, HEAD, PUT, DELETE, OPTIONS).

**YAML:**
```yaml
entry:
  - type: rest
    method: GET
    path: /products/:id
    handler: onGetProduct
    retry:
      max_retries: 3
      initial_interval: 500ms   # first backoff
      max_backoff: 10s          # ceiling
      multiplier: 2.0           # exponential factor
```

The middleware intercepts transient failures (5xx, network errors) and retries up to `max_retries` times with exponential backoff + jitter. Non-idempotent methods (POST, PATCH) are not retried.

**Go (programmatic retry for DB calls):**
```go
import "github.com/natuleadan/sdk-api/infra/fx"

err := fx.RetryWithBackoff(ctx, func() error {
    return db.Query(ctx, "...")
}, fx.RetryConfig{
    MaxAttempts: 3,
    InitialDelay: 100 * time.Millisecond,
    MaxDelay: 2 * time.Second,
})
```

---

## 17. Fallback (Circuit Breaker Rejection)

When the circuit breaker opens, the entry can serve a degraded response instead of a hard 503.

**YAML:**
```yaml
entry:
  - type: rest
    method: GET
    path: /products
    handler: onListProducts
    breaker: true
    fallback: stale              # "degraded" or "stale"
```

**Strategies:**
| Value | Behavior |
|-------|----------|
| `degraded` | Returns 503 with `{"error": "service unavailable", "retry_after": 30}` |
| `stale` | Returns the last cached 200 response (30s TTL). Falls back to `degraded` if no cache exists |

**Go:**
```go
type StaleCache struct {
    Data      any
    Timestamp time.Time
}

svc.WithRest("onListProducts", func(c *runtime.RestCtx) error {
    data, err := fetchProducts(c.UserContext())
    if err != nil {
        runtime.SetStaleFallback(c, staleData) // Store fallback data
        return err
    }
    return c.JSON(data)
})
```

---

## 18. Bulkhead (Concurrency Isolation)

Limit concurrent external API calls per named resource.

**YAML:**
```yaml
server:
  bulkhead:
    openai: 5     # max 5 concurrent calls to OpenAI
    stripe: 10    # max 10 concurrent calls to Stripe

entry:
  - type: rest
    method: POST
    path: /chat
    handler: onChat
    bulkhead: openai
```

**Go:**
```go
// Programmatic access via runtime.Bulkhead
func onChat(c *runtime.RestCtx) error {
    bh := bulkhead.Get("openai")
    if !bh.TryAcquire() {
        return c.Status(503).JSON(map[string]any{
            "error": "rate limited",
        })
    }
    defer bh.Release()
    // Make OpenAI call...
}
```

Bulkhead is ideal for protecting the service from cascading failures when external dependencies are slow or saturated.

---

## 19. Multi-Database

Different entry endpoints can use different databases.

**YAML:**

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${PG_URL}"
  - name: mysql-audit
    driver: mysql
    url: "${MYSQL_URL}"

entry:
  - type: crud
    model: Product
    db: pg-main           # PostgreSQL for products
  - type: crud
    model: AuditLog
    db: mysql-audit       # MySQL for audit
```

**Go:**

```go
// PostgreSQL table
pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
pgTable, _ := db.NewTable[Product](pgPool, "products")
svc.WithCRUD("Product", runtime.NewCRUDProvider(pgTable, nil))

// MySQL table
sqlPool := runtime.PoolSQL(nil, "mysql-audit")
mysqlTable, _ := db.NewMySQLTable[AuditLog](sqlPool, "audit_logs")
svc.WithCRUD("AuditLog", runtime.NewMySQLCRUDProvider(mysqlTable, nil))
```

---

## 20. MySQL CRUDProvider

Same CRUD pattern but using MySQL driver instead of PostgreSQL.

```go
sqlDB := svc.Pool("mysql-main").(*sql.DB)
table, _ := db.NewMySQLTable[Product](sqlDB, "products")
svc.WithCRUD("Product", runtime.NewMySQLCRUDProvider(table, &ProductHooks{}))
```

`NewMySQLCRUDProvider[T]` wraps `db.MySQLTable[T]` into the same `CRUDProvider` interface.

---

## 21. Turso CRUDProvider

Same pattern for Turso (SQLite-compatible).

```go
t, _ := db.NewTursoTable[Product]("file://bench.db", "products")
svc.WithCRUD("Product", runtime.NewTursoCRUDProvider(t, &ProductHooks{}))
```

---

## 22. Docker Compose Config per Environment

Use different YAML for Docker vs local:

```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    cfgPath = "service.yaml"
}
svc, _ := runtime.New(cfgPath)
```

Example `service.docker.yaml` uses Docker hostnames (`postgres:5432`, `nats:4222`) instead of `localhost`.

---

## 23. Auth: Manual JWT + API Keys

Full authentication example with JWT, API keys, role hierarchy, rate limits, CSRF, and security headers.

**Example:** `examples/400-auth/manual-pg` — 103 integration tests, Dockerized.

**YAML:**
```yaml
auth:
  enabled: true
  driver: manual
  secret: "${JWT_SECRET}"
  algorithm: HS256
  expiry: 900
  cookie:
    access_token_name: token
    http_only: true
    secure: false
    same_site: Lax
  refresh:
    enabled: true

server:
  csrf:
    enabled: true
    json_check: true
  security_headers:
    frame_options: DENY
    referrer_policy: no-referrer
    csp_config:
      level: basic
      default_src: ["'self'"]

entry:
  - type: rest
    method: POST
    path: /login
    handler: loginHandler

  - type: rest
    method: GET
    path: /profile
    handler: profileHandler
    auth_modes: [jwt]

  - type: rest
    method: GET
    path: /cookie/profile
    handler: profileHandler
    auth_modes: [jwt]
    jwt_from: "cookie:token"

  - type: rest
    method: GET
    path: /products
    handler: listProducts
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"

  - type: rest
    method: DELETE
    path: /admin/products/:id/hard
    handler: hardDeleteProduct
    auth_modes: [jwt]
    roles: ["admin"]

  - type: rest
    method: GET
    path: /admin/users
    handler: listUsers
    auth_modes: [jwt]
    roles: ["admin"]
    permissions: ["users:manage"]

  - type: rest
    method: POST
    path: /rate-limited
    handler: rateLimitedHandler
    auth_modes: [jwt, apikey]
    api_key_prefix: "sk-"
    rate_limit:
      requests_per_second: 10
      burst: 20
    rate_limit_per_user:
      requests_per_second: 5
      burst: 10
    rate_limit_per_role:
      admin:
        requests_per_second: 5
        burst: 10
      viewer:
        requests_per_second: 1
        burst: 2
```

**Key features demonstrated:**
- JWT login with bcrypt password hashing (`auth.HashPassword`, `auth.VerifyPassword`)
- API keys with SHA-256 hashing and prefix validation (`api_key_prefix: "sk-"`)
- Cookie-based JWT (`jwt_from: "cookie:token"`)
- Role hierarchy (viewer → editor → admin) with inheritance
- Permissions (`entry[].permissions`) separate from roles
- Token refresh auto-wire (`auth.refresh.enabled: true`)
- Rate limiting per-user, per-key, per-role
- Dynamic rate limit override (`WithRateLimitMaxFunc`)
- CSRF with JSON check (`json_check: true`)
- Security headers (X-Frame-Options, CSP, Referrer-Policy)
- Encrypted cookies (`encrypt_cookie`)
- Soft delete + audit log
- Stateless JWT behavior (valid after user deletion)
- MFA with TOTP (`/auth/mfa/enable`, `/auth/mfa/verify`, `requires_mfa: true`)
- Token blacklist with revoke endpoint (`svc.WithJWTBlacklist()`)
- Account lockout after 5 failed login attempts (DB-persisted)
- Password strength validation (8+ chars, upper, lower, digit)
- Email verification flow (token-based, mock)
- Password reset flow (forgot/reset with expiry)

**Go:**
```go
svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
    // Check roles with inheritance
    if len(roles) > 0 { /* check auth.Roles */ }
    // Check permissions separately
    if len(permissions) > 0 { /* check auth.Permissions */ }
    return nil
})

svc.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
    // Look up key hash in DB
    return &middleware.AuthContext{UserID: id, Roles: []string{role}}, nil
})

svc.WithRateLimitMaxFunc(func(c fiber.Ctx) int {
    if c.Get("X-Debug") == "true" { return 5 }
    return 0
})
```

---

## 24. gRPC Service

Define a gRPC service that shares business logic with HTTP handlers.

**YAML:**

```yaml
entry:
  - type: grpc
    service_name: ProductService
    handler: onProductGRPC

server:
  grpc_server:
    listen_on: ":8081"
    health: true
  grpc_clients:
    - name: product-service
      target: direct:///product-svc:8081
      timeout: 5000
```

**Go (generated scaffold):**

```go
// grpcserver/product.go — delegates to internal/logic/
type ProductServer struct {
    svcCtx *svc.ServiceContext
    pb.UnimplementedProductServiceServer
}

func (s *ProductServer) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
    l := logic.NewProductLogic(s.svcCtx)
    items, err := l.List(ctx)
    // ... map to proto response
}
```

HTTP and gRPC share the same `internal/logic/` package. The gRPC server auto-registers interceptors: tracing, circuit breaker, timeout, and CPU shedding.

## 25. Lifecycle Hooks

Hooks are called before/after CRUD and REST operations. They live in a separate file from the model struct.

```
models/
├── product.go           # struct definition
└── products_hooks.go    # BeforeCreate, AfterCreate, BeforeUpdate, AfterUpdate, BeforeDelete, AfterDelete
```

```go
// models/products_hooks.go
func (h *ProductHooks) BeforeCreate(ctx context.Context, req Product) (Product, error) {
    if req.Price <= 0 {
        return req, fmt.Errorf("price must be positive")
    }
    return req, nil
}

func (h *ProductHooks) AfterCreate(ctx context.Context, entity *Product) error {
    // send event, invalidate cache, etc.
    return nil
}
```

## 26. Graceful Shutdown

The runtime handles shutdown automatically on SIGINT/SIGTERM:

1. Stops cron scheduler (waits for running jobs)
2. Drains exit workers (waits for in-flight handlers, timeout 5s)
3. Drains all NATS connections
4. Closes all DB pools
5. Stops HTTP server

No manual shutdown code needed.
