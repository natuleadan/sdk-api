# Runtime

The `Service` struct is the core orchestrator. It loads YAML config, initializes databases and event streams, registers HTTP routes (entry), starts workers (exit), schedules cron jobs, and configures security — all from a single `Run()` call.

**Stack:** Fiber (fasthttp) + pgxpool (PostgreSQL) + MongoDB + NATS JetStream + Kafka + go-zero infra (45+ packages)

## Service API

```go
// Create service from YAML file
svc, err := runtime.New("service.yaml")

// Create service from embedded YAML (recommended for production)
//go:embed service.yaml
var configYAML []byte
svc, err := runtime.NewFromYAML(configYAML)

// Register entry handlers
svc.WithCRUD("Product", provider)           // CRUD auto-routes
svc.WithRest("hello", func(c *runtime.RestCtx) error { ... })  // REST endpoints (no fiber import)
svc.WithWS("chat", chatHandler)             // WebSocket
svc.WithSSE("stream", sseHandler)           // SSE
svc.WithAsync("processReport", handler)     // Async job (202 Accepted)
svc.RegisterValidation("CreateProduct", CreateProductInput{})  // Input validation

// Register entry hooks
svc.WithHooks("Product", &ProductHooks{})

// Register exit handlers (event workers)
svc.WithExit("onOrderConfirmed", handler)

// Register exit hooks
svc.WithExitHooks(map[string]runtime.ExitHooks{...})

// Register cron handlers
svc.WithCron("onCleanup", handler)

// Register models for OpenAPI/GraphQL schema generation
svc.RegisterModel("Product", (*Product)(nil))
svc.MustRegister("Product", runtime.NewCRUDProvider(table, hooks)) // Panics on error (convenience)

// Seed data (runs after initDatabases, before HTTP starts)
svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
    pool := s.PoolPGTyped("primary")
    _, err := pool.Exec(ctx, "INSERT INTO ...")
    return err
})

// Register gRPC service
svc.WithGRPC("ProductGRPC", grpcHandler)

// Access databases and event streams
svc.Pool("pg-main")              // any — returns the pool by name
svc.PoolPG("pg-main")            // *pgxpool.Pool — typed access
svc.PoolPGTyped("pg-main")       // *pgxpool.Pool — returns nil if not a pgx pool
svc.NATS("primary")              // events.EventBroker — event broker by stream name
svc.Stream("primary")            // events.EventBroker — same as NATS, returns broker by stream name
svc.SafeHTTPClient()             // *middleware.SafeHTTPClient — SSRF-protected HTTP client
svc.KV("cache-main")             // *redis.Redis — KV store by name (from kv: YAML section)
svc.App()                        // *fiber.App — raw Fiber access
svc.Storage("/files/upload")     // server.StorageBackend — storage by entry path
svc.GetGrpcServer()              // *GrpcServer — gRPC server instance (nil if not in micro mode)
svc.GetGRPCClient("user-svc")    // *GrpcClient — named gRPC client connection
svc.Table("Product")             // any — *db.Table[T] registered via MustRegister

// gRPC server and client (micro mode only)
gs := svc.GetGrpcServer()            // *GrpcServer — register proto services, nil in monolith mode
gc := svc.GetGRPCClient("user-svc")  // *GrpcClient — access named gRPC client connection

runtime.NewGrpcServer(cfg, register)  // Create gRPC server with interceptors (trace, breaker, timeout, shedding, recovery, prometheus)
runtime.NewGrpcClient(cfg)            // Create gRPC client with service discovery (direct, etcd), TLS option
runtime.BulkheadGet("openai")         // *syncx.Limit — named semaphore for external API concurrency

// Package-level helpers
runtime.GetTable[Product](svc, "Product")   // *db.Table[T] — typed table by model name
runtime.TableFor[Product](pools, "pg-main", "link") // *db.Table[T] — new table from pools map
runtime.PoolPG(pools, "pg-main")           // *pgxpool.Pool — typed pool from pools map
runtime.PoolSQL(pools, "pg-main")          // *sql.DB — SQL pool by name
runtime.ErrNotFound                        // error — record not found sentinel

// Redis config (re-exported from infra/stores/redis)
runtime.RedisConfig{Host: "localhost:6379", Type: runtime.NodeType}
runtime.RedisConfig{Host: "sentinel1:26379", Type: runtime.SentinelType, MasterName: "mymaster"}
runtime.RedisConfig{Host: "localhost:6379", Type: runtime.NodeType, Database: 1}

// Start everything
svc.Run()
```

## Lifecycle

### Standard (file or embedded config)

```
New("service.yaml") ─────────────┐
                                 ├─ LoadConfig() + ParseConfig()
NewFromYAML(content) ────────────┘    │
                                       ├─ loadDotEnv()            ← reads .env file (automatic)
                                       ├─ validateConfigDeploy()  ← checks deploy.target
                                       ├─ validateConfig*()       ← databases, entries, exits, cron
                                       ├─ applyEnvOverrides()     ← resolves PORT env
                                       │
                                    → Run()
                                        1. validateConfigDeploy() — check deploy.target rules
                                        2. validateAuthConfig()   — check auth driver config
                                        3. initDatabases()        — connect PG/Turso/MySQL/Mongo pools (dedup by URL)
                                        4. runSeeds()             — WithSeed callbacks (DDL, data seeding, app setup)
                                        5. initKvConns()          — lazy-init Redis/Dragonfly connections
                                       5. initStreamConns()      — connect NATS/Kafka + create streams
                                       6. initSSRF()             — SafeHTTPClient (if configured)
                                       7. initGrpc()             — gRPC server (if grpc_server configured) + clients
                                       8. initServer()           — Fiber HTTP + middlewares + TLS + security + CSRF + rate limit
                                       8. registerEntryRoutes()  — register all entry routes (9 types)
                                       9. serveStaticFiles()
                                      10. registerDocs()         — OpenAPI + Scalar UI
                                      11. startExitWorkers()     — start all event workers (NATS/Kafka consumers)
                                      12. startCron()            — start cron scheduler
                                       → HTTP server starts
```

All steps are optional. No databases? Skip `databases:` in YAML. No HTTP? Only define `exit:` and `cron:`.

### ParseConfig

`ParseConfig` parses raw YAML bytes into a `*ServiceConfig`. It is called by both `New()` and `NewFromYAML()` internally, but is also available as a standalone public API for advanced use cases:

```go
cfg, err := runtime.ParseConfig(yamlContent)
if err != nil {
    log.Fatalf("parse: %v", err)
}
log.Printf("loaded config: %s on port %d", cfg.Name, cfg.Port)
```

This is useful when you need to inspect or modify the config before creating the service:

```go
cfg, _ := runtime.ParseConfig(yamlContent)
cfg.Port = 9090 // override before building the service
```

### Deploy target validation

Set `deploy.target` in `service.yaml` to enable platform-specific validation at startup:

```yaml
deploy:
  target: vercel    # auto | vercel | docker | kube | bare-metal
```

| Target | Enforced rules |
|--------|---------------|
| `vercel` | `server.prefork` must be `false`, `server.tls.enabled` must be `false` |
| `docker` / `kube` / `bare-metal` | No extra restrictions |
| `auto` (default) | No validation — runtime behavior unchanged |

## Async Jobs

```go
svc.WithAsync("processReport", func(body []byte, job *runtime.JobState) error {
    // Process the job
    job.Result = fiber.Map{"report_url": "https://..."}
    return nil
})
```

- POST `/path` → `202 Accepted` with `job_id` + `status_url`
- GET `/path/:job_id` → JSON with job status and result
- Job states: `pending` → `processing` → `completed` / `failed`

## GraphQL

```go
svc.RegisterModel("Product", (*Product)(nil))
```

- Auto-generates schema from registered `CRUDProvider` instances
- Queries: `Products`, `Product(id)`
- Mutations: `createProduct`, `updateProduct`, `deleteProduct`
- POST `/graphql` endpoint

## Validation

```go
type CreateProductInput struct {
    Name  string  `json:"name" validate:"required,min=3,max=100"`
    Price float64 `json:"price" validate:"required,gt=0"`
}

svc.RegisterValidation("CreateProductInput", CreateProductInput{})
```

When `entry[].validate` is set in YAML, the input body is validated before the handler runs. Returns `422` with field-level errors on failure.

## SSRF Protection

```go
client := svc.SafeHTTPClient()
if client != nil {
    resp, err := client.Do(req)  // Blocks private/internal IPs
}
```

Returns `nil` if SSRF protection is not configured in YAML (`server.ssrf.enabled: true`).

## Graceful Shutdown

On SIGINT/SIGTERM, two paths run concurrently:

**Service shutdown** (`svc.Shutdown()`):
1. Stop cron scheduler (waits for running jobs)
2. Drain exit workers (waits for in-flight handlers, 5s timeout)
3. Drain all event broker connections
4. Close all DB pools

**HTTP server shutdown** (runs concurrently via a separate signal listener):
- Fiber server stops accepting new requests
- Waits for in-flight requests to complete (up to `server.shutdown_timeout`)

Both paths must finish before the process exits. No manual cleanup needed.

## gRPC

gRPC is available only in `server.mode: micro` for inter-service communication. In `server.mode: monolith` (default), gRPC is disabled and all communication uses direct function calls via `internal/logic/`.

### Architecture

```
┌─ Microservice A ─────────────────┐     ┌─ Microservice B ─────────────────┐
│ HTTP port 23600  (external API)  │     │ HTTP port 23601  (external API)  │
│ gRPC port 50051 (inter-service)  │ ←─→ │ gRPC client → etcd → discover A │
│ etcd key: "users-svc"           │     │ etcd key: "orders-svc"          │
└──────────────────────────────────┘     └──────────────────────────────────┘
```

In micro mode, services expose gRPC for internal calls and HTTP for external clients. Service discovery via etcd or direct endpoint configuration.

### YAML Configuration

**Server (exposes gRPC):**
```yaml
server:
  mode: micro
  grpc_server:
    listen_on: ":50051"
    health: true
    etcd_endpoints: ["etcd:2379"]
    etcd_key: users-svc
```

**Client (consumes gRPC):**
```yaml
server:
  mode: micro
  grpc_clients:
    - name: users-svc
      target: direct:///user-service:50051
      secure: true
```

### Server API

```go
gs := svc.GetGrpcServer()
if gs == nil {
    // gRPC not available (monolith mode)
}
pb.RegisterUserServiceServer(gs.Server(), &userServer{})
```

The gRPC server is created with interceptors: trace, breaker, timeout, adaptive shedding, panic recovery, and Prometheus metrics. Reflection is registered for development tooling.

### Client API

```go
client := svc.GetGRPCClient("users-svc")
conn := client.Conn()  // *grpc.ClientConn
userClient := pb.NewUserServiceClient(conn)
user, err := userClient.GetUser(ctx, &pb.GetUserRequest{Id: 1})
```

### Service Discovery

| Scheme | Example | Description |
|--------|---------|-------------|
| `direct:///` | `direct:///host1:8081,host2:8081` | Static endpoint list |
| `etcd:///` | `etcd:///etcd:2379?key=users-svc` | Dynamic discovery via etcd watch |

The etcd resolver uses `discov.Subscriber` to watch for endpoint changes in real time.

### Interceptors

| Interceptor | Type | Description |
|-------------|------|-------------|
| Trace | Unary + Stream | OpenTelemetry span propagation |
| Breaker | Unary + Stream | Circuit breaker per method |
| Timeout | Unary | Per-request deadline |
| Shedding | Unary | Adaptive CPU load shedding |
| Recover | Unary | Panic recovery → `codes.Internal` |
| Prometheus | Unary | Duration histogram + code counter |

Configured via `GrpcInterceptorsConfig` (all enabled by default).

### Prometheus Metrics

When the Prometheus agent is enabled (`infra/prometheus`), the following metrics are collected automatically:

| Metric | Type | Labels |
|--------|------|--------|
| `rpc_server_requests_duration_ms` | Histogram | `method` |
| `rpc_server_requests_code_total` | Counter | `method`, `code` |
| `rpc_client_requests_duration_ms` | Histogram | `method` |
| `rpc_client_requests_code_total` | Counter | `method`, `code` |

## RegisterModel

For OpenAPI and GraphQL schema generation, register Go struct types:

```go
type Product struct {
    ID    int64   `db:"id,primary,auto" json:"id"`
    Name  string  `db:"name,required" json:"name"`
}

svc.RegisterModel("Product", (*Product)(nil))
```

The function parses struct tags (`db`, `json`) to build property schemas.

## Hooks

### Entry Hooks (HTTP lifecycle)

```go
type EntryHooks[T any] interface {
    BeforeCreate(ctx, req T) (T, error)
    AfterCreate(ctx, entity *T) error
    BeforeUpdate(ctx, id any, patch map) (map, error)
    AfterUpdate(ctx, entity *T) error
    BeforeDelete(ctx, id any) error
    AfterDelete(ctx, id any) error
    BeforeTransform(ctx, input T) (T, error)
    AfterTransform(ctx, output any) error
}
```

Embed `DefaultHooks[T]` to implement only the methods you need.

### Exit Hooks (event lifecycle)

```go
type ExitHooks interface {
    OnMessage(ctx, msg []byte) ([]byte, error)
    OnSuccess(ctx)
    OnError(ctx, err error)
}
```

### WrapTransformHandler

Wraps a REST handler with `BeforeTransform`/`AfterTransform` hooks. The wrapped handler takes `fiber.Ctx` (the hooks are applied before and after the handler runs):

```go
svc.WithRest("convert", func(c *runtime.RestCtx) error {
    input, _ := runtime.GetTransformed[MyModel](c)
    return c.JSON(runtime.Map{"name": input.Name})
})
```

Note: `WrapTransformHandler` returns `func(fiber.Ctx) error` and the hooks apply transforms on `c.Locals("transformed")`. For new projects, use `GetTransformed[T](c)` in the handler directly.

## RestCtx API

The `RestCtx` type wraps `fiber.Ctx` so REST handlers don't need to import Fiber directly:

```go
func(c *runtime.RestCtx) error {
    c.Body()                 // []byte — request body
    c.Params("id")           // string — URL parameter
    c.Query("sort", "id")    // string — query parameter with default
    c.JSON(data)             // error — send JSON response
    c.Status(code)           // *RestCtx — set status code (chainable)
    c.SendStatus(code)       // error — send status only
    c.Context()              // context.Context
    c.Method()               // string — HTTP method
    c.Locals(key, val...)    // any — get/set locals
    c.SendString(s)          // error — send plain text
    c.Get(key)               // string — request header
    c.Set(key, val)          // set response header
    c.Bind(v)                // error — bind body to struct
    c.StatusCode()           // int — response status code
    c.Path()                 // string — request path
    c.ResponseBody()         // string — response body as string
    c.Redirect(url, code)    // error — HTTP redirect (default 302, or pass 301/308)
    c.SetCookie(cookie)      // set *fiber.Cookie (Name, Value, Path, HTTPOnly, Secure, SameSite, MaxAge)
    c.PoolPG(name)           // *pgxpool.Pool — database pool by database name
    c.PoolSQL(name)          // *sql.DB — MySQL/Turso pool by database name
}
```

### runtime.Map

`type Map = map[string]any` — shorthand for JSON response maps. Replaces `fiber.Map{}`.

```go
return c.JSON(runtime.Map{"status": "ok", "data": items})
```

### runtime.NewCookie

`func NewCookie(name, value string, maxAge int) *Cookie` — builds a cookie with secure defaults (HttpOnly, Secure, SameSite Strict, path /).

```go
c.SetCookie(runtime.NewCookie("token", signed, 900))
```

## CRUD Provider

```go
type CRUDProvider interface {
    List(ctx, params) error
    Get(ctx, id) error
    Create(ctx, body) error
    Update(ctx, id, body) error
    Delete(ctx, id) error
}
```

Four implementations:
- `NewCRUDProvider[T](table, hooks)` — PostgreSQL
- `NewMySQLCRUDProvider[T](table, hooks)` — MySQL
- `NewTursoCRUDProvider[T](table, hooks)` — Turso
- `NewMongoCRUDProvider(model, lookupField)` — MongoDB

### WithCRUDFactory

Preferred registration pattern for lazy-init CRUD tables. The factory function is called once during `Run()`, avoiding the nil-pool problem before startup:

```go
svc.WithCRUDFactory("Product", func() runtime.CRUDProvider {
    return runtime.NewCRUDProvider(table, nil)
})
```

### MustRegister

Shortcut to create a table + provider + factory in one call:

```go
runtime.MustRegister[Product](svc, "Product", "pg-main", "products", nil)
runtime.MySQLMustRegister[Order](svc, "Order", "mysql-main", "orders", nil)
runtime.TursoMustRegister[Item](svc, "Item", "turso-main", "items", nil)
runtime.MongoMustRegister(svc, "Profile", "mongo-main", "profiles", "profiles", "user_id")
```

### CachedCRUD

`CachedCRUD[T any](svc *Service, name, poolName, tableName string, kvName string, keyPrefix string, l2TTL, l1TTL time.Duration)` — PostgreSQL CRUD with a two-level cache (L1 RAM, L2 Redis). The `kvName` references a name from the `kv:` YAML section. Registers the provider internally via `WithCRUDFactory` — no return value.

```go
runtime.CachedCRUD[Product](svc, "cachedProducts", "pg-main", "products", "cache-main", "product:", 5*time.Minute, 30*time.Second)
```

### MySQLCachedCRUD

`MySQLCachedCRUD[T any](svc *Service, name, poolName, tableName string, kvName string, keyPrefix string, l2TTL, l1TTL time.Duration)` — MySQL CRUD with the same two-level cache pattern.

```go
runtime.MySQLCachedCRUD[Order](svc, "cachedOrders", "mysql-main", "orders", "cache-main", "order:", 5*time.Minute, 30*time.Second)
```

## Pool helpers

```go
runtime.Pool(pools, name)      // any — returns pool by name
runtime.PoolPG(pools, name)    // *pgxpool.Pool
runtime.PoolSQL(pools, name)   // *sql.DB for Turso/MySQL
runtime.TableFor[T](pools, poolName, tableName)  // *db.Table[T]

// On the Service:
svc.Pool("pg-main")
svc.PoolPG("pg-main")
```

### svc.KV

`func (s *Service) KV(name string) *redis.Redis` — returns a KV store connection by name (defined in the `kv:` YAML section). Creates the connection lazily on first use.

```go
rdb := svc.KV("cache-main")
```

## Database-backed pooling formula

```
max(1, (PG_SERVER_MAX_CONNS - pool.reserved_conns) / REPLICA_COUNT)
```

| Env | Default | Description |
|-----|---------|-------------|
| `PG_SERVER_MAX_CONNS` | `100` | Server max connections |
| `REPLICA_COUNT` | `1` | Number of replicas sharing the pool |

## CRUD Override values

| YAML value | Behavior |
|------------|----------|
| `""` or `~` | Use default auto-generated handler |
| `"-"` | Do not register this endpoint |
| `"handlerName"` | Use custom handler from Rest map |

## Storage backends

```go
// Local filesystem
store, _ := server.NewLocalStorage("/data/uploads")

// S3-compatible (AWS S3, MinIO, R2)
store, _ := server.NewS3Storage(server.S3Config{
    Endpoint:  os.Getenv("S3_ENDPOINT"),
    Bucket:    os.Getenv("S3_BUCKET"),
    AccessKey: os.Getenv("S3_ACCESS_KEY"),
    SecretKey: os.Getenv("S3_SECRET_KEY"),
})
```

Storage backends in YAML are auto-created when `entry[].storage` is specified. Access them in handlers via `svc.Storage(path)`:

```go
store := svc.Storage("/files/upload")
store.Upload(ctx, "key", reader, size, contentType)
reader, _ := store.Download(ctx, "key")
```

### Cached storage (L1 RAM + L2 disk)

When `cache:` is configured in YAML, the SDK wraps the backend with `cachedStorage`. L1 and L2 are independent — either, both, or none can be active:

```yaml
storage:
  mode: s3
  cache:
    l1: ram
    l1_ttl: 5m
    l1_size: 10000
    l2: disk
    l2_path: /data/cache
```

First request hits S3, a goroutine populates the cache. Subsequent requests served from RAM (~50x faster). L2 disk provides persistence across restarts.

## Auth validators

### WithAuthValidator

Registers the JWT auth callback for `driver: manual`. The function receives the auth context from the token, the entry's required roles, and the entry's required permissions:

```go
svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
    if len(roles) > 0 {
        // Check that auth.Roles satisfies at least one required role
    }
    if len(permissions) > 0 {
        // Check that auth.Permissions satisfies at least one required permission
    }
    return nil
})
```

The `permissions` slice comes from `entry[].permissions` in YAML. See `examples/400-auth/manual-pg` for a complete implementation.

### WithAPIKeyValidator

Registers the API key validation callback for `driver: manual`:

```go
svc.WithAPIKeyValidator(func(ctx context.Context, key string) (*middleware.AuthContext, error) {
    // Look up key in DB, return auth context or error
    return &middleware.AuthContext{UserID: "user", Roles: []string{"viewer"}}, nil
})
```

The returned `AuthContext` must include at least `UserID` and `Roles`. The function receives the raw key value (already stripped of any configured prefix).

### Auth Utilities

The `runtime/auth` package provides password, token, and role utilities:

| Function | Description |
|----------|-------------|
| `HashPassword(password string) (string, error)` | bcrypt hash (cost 10) |
| `VerifyPassword(hash, password string) bool` | bcrypt verify |
| `GenerateToken() (string, error)` | 32-byte crypto/rand token as hex |
| `TokenHash(raw string) string` | SHA-256 hex digest for storing API keys, tokens |
| `CheckPasswordStrength(password string) error` | Validates 8+ chars, upper+lower+digit |

```go
token, _ := auth.GenerateToken()
hash := auth.TokenHash(token)          // store this
auth.TokenHash(token) == storedHash    // verify

err := auth.CheckPasswordStrength("Weak1")  // too short
```

**RoleHierarchy:**

```go
type RoleHierarchy map[string][]string

roles := auth.RoleHierarchy{
    "viewer": {},
    "editor": {"viewer"},
    "admin":  {"editor", "viewer"},
}
roles.Inherits("admin", "viewer")  // true
```

Implement your own hierarchy and use it in `WithAuthValidator`.

### TOTP Helpers

The `runtime/auth` package also provides TOTP (Time-based One-Time Password) utilities for MFA:

| Function | Description |
|----------|-------------|
| `GenerateTOTPSecret()` | Generates a random base32 secret (160 bits). Returns `(string, error)` |
| `ValidateTOTP(secret, code string) bool` | Validates a 6-digit code against the secret. Checks ±1 time step for clock drift |
| `GenerateTOTPURI(secret, issuer, accountName string) string` | Builds an `otpauth://` URI for authenticator apps |
| `GenerateTOTPCode(secret string) string` | Generates the current TOTP code for testing |

All functions use stdlib only (no external dependencies). See `examples/400-auth/manual-pg` for MFA enable/verify endpoints.

## Rate limit

### WithRateLimitMaxFunc

Registers a dynamic rate limit resolver that can override per-entry rate limits at runtime:

```go
svc.WithRateLimitMaxFunc(func(c *runtime.RestCtx) int {
    if c.Get("X-Debug") == "true" {
        return 5 // override RequestsPerSecond and Burst to 5
    }
    return 0 // use static YAML config
})
```

When the function returns > 0, both `requests_per_second` and `burst` are set to the returned value, overriding the YAML configuration. See `examples/400-auth/manual-pg` for a working example.

### WithJWTBlacklist

Registers a callback that checks if a JWT has been revoked. Called after JWT validation succeeds, before the request is processed. Works with all auth drivers: manual, ory, openfga-zitadel. Return `true` to reject the token (401):

```go
svc.WithJWTBlacklist(func(rawToken string) bool {
    pool := svc.PoolPGTyped("primary")
    hash := sha256Hex(rawToken)
    var exists bool
    pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_hash=$1)`, hash).Scan(&exists)
    return exists
})
```

The callback receives the raw token string (e.g. `"Bearer eyJ..."`). See `examples/400-auth/manual-pg` for a complete revoke + blacklist implementation.

### Presigned URLs

For S3 backends with `presign: true`, assert the `Presigner` interface to generate temporary S3 URLs:

```go
if p, ok := store.(server.Presigner); ok {
    url, err := p.PresignURL(ctx, "uploads/file.pdf", 5*time.Minute)
    // url is valid for 5 minutes, client downloads directly from S3
}
```

Three download modes:
- **Proxy** — server reads from S3 and streams to client (no presign needed)
- **Redirect (302)** — server returns a signed URL, client follows redirect (zero server bandwidth)
- **Sign-only JSON** — server returns signed URL as JSON, client decides how to use it

### Async Job Store

The `JobStore` interface persists job state across submissions and polls:

```go
type JobStore interface {
    Create(id string) *JobState
    Get(id string) (*JobState, bool)
    Update(id string, status JobStatus, result any, errMsg string)
    Delete(id string)
    List() ([]*JobState, error)
    ReapStale(ctx context.Context, timeout time.Duration, maxRetries int) (int, error)
    Cleanup(ctx context.Context, ttl time.Duration) (int, error)
}
```

Built-in implementations:

| Implementation | Driver | Constructor |
|----------------|--------|-------------|
| In-memory | `memory` | `newMemoryJobStore()` |
| PostgreSQL | `postgres` | `newPGJobStore(pool, table)` |
| Redis | `redis` | `newRedisJobStore(client, prefix)` |
| NATS KV | `nats_kv` | `newNATSKVJobStore(conn, bucket)` |

The store is selected via YAML `async_store.driver`. The `ReapStale` method is called by the `Reaper` to recover jobs stuck in `processing` status. The `Cleanup` method is called by the Reaper when `result_ttl` is configured.

### AsyncJobManager

```go
mgr := runtime.NewAsyncJobManager(store, processor)
mgr := runtime.NewAsyncJobManagerWithRetry(store, processor, maxRetries)
```

The manager is created automatically when registering an async entry. It exposes handler methods:

- `HandleSubmit()` — POST handler, returns 202
- `HandleStatus()` — GET `/:job_id` handler
- `HandleCancel()` — DELETE `/:job_id` handler, returns 204 or 409
- `HandleList()` — GET `/` handler, returns job list
- `HandleStatusSSE()` — GET `/:job_id/status` handler, SSE stream

### VerifyCallbackSignature

```go
if runtime.VerifyCallbackSignature(payload, secret, signature) {
    // signature is valid
}
```

Verifies an HMAC-SHA256 callback signature. Used on the receiving end of async callbacks. The SDK sends callbacks with `X-Job-Signature: <hex>` header when `callback.secret` is configured.

### Reaper

```go
r := runtime.NewReaper(store, timeout, interval, maxRetries)
r.Start()  // background goroutine
r.Stop()   // cancel
```

The reaper periodically calls `ReapStale` on the store. When a job's `processing_deadline` has passed, the reaper resets it to `pending` (or `failed` if `maxRetries` exceeded). This enables automatic recovery when a service instance crashes while processing a job.

When `result_ttl` is configured, the reaper also calls `Cleanup` on each cycle to remove old completed/failed jobs.

Configured via YAML:

```yaml
async_store:
  driver: nats_kv
  stream: default
  result_ttl: 24h
  reassign:
    enabled: true
    processing_timeout: 5m
    reap_interval: 30s
    max_retries: 3
  callback:
    url: "${CALLBACK_URL}"
    secret: "${CALLBACK_SECRET}"
```

## HTTP pool sizing

Configure the S3 HTTP client pool under `storage.pool` in YAML to match expected concurrency:

```yaml
storage:
  mode: s3
  pool:
    max_idle_conns: 200
    max_idle_conns_per_host: 100
    max_conns_per_host: 250
    idle_timeout: 90s
```

Without pool config, Go's default `MaxIdleConnsPerHost=2` limits throughput to ~500 req/s under load.
