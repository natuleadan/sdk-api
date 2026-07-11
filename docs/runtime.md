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

// Access databases and event streams
svc.Pool("pg-main")              // any — returns the pool by name
svc.PoolPG("pg-main")            // *pgxpool.Pool — typed access
svc.PoolPGTyped("pg-main")       // *pgxpool.Pool — returns nil if not a pgx pool
svc.NATS("primary")              // events.EventBroker — event broker
svc.SafeHTTPClient()             // *middleware.SafeHTTPClient — SSRF-protected HTTP client
svc.App()                        // *fiber.App — raw Fiber access
svc.Storage("/files/upload")     // server.StorageBackend — storage by entry path
svc.Table("Product")             // any — *db.Table[T] registered via MustRegister

// Package-level helpers
runtime.GetTable[Product](svc, "Product")   // *db.Table[T] — typed table by model name
runtime.TableFor[Product](pools, "pg-main", "link") // *db.Table[T] — new table from pools map
runtime.PoolPG(pools, "pg-main")           // *pgxpool.Pool — typed pool from pools map
runtime.PoolSQL(pools, "pg-main")          // *sql.DB — SQL pool by name
runtime.ErrNotFound                        // error — record not found sentinel

// Redis config (re-exported from infra/stores/redis)
runtime.RedisConfig{Host: "localhost:6379", Type: runtime.NodeType}

// Start everything
svc.Run()
```

## Lifecycle

### Standard (file or embedded config)

```
New("service.yaml") ─────────────┐
                                 ├─ LoadConfig() + ParseConfig()
NewFromYAML(content) ────────────┘    │
                                       ├─ validateConfigDeploy()  ← checks deploy.target
                                       ├─ validateConfig*()       ← databases, entries, exits, cron
                                       ├─ applyEnvOverrides()     ← resolves PORT env
                                       │
                                  → Run()
                                      1. initDatabases()        — connect PG/Turso/MySQL/Mongo pools
                                      2. initEventStreams()     — connect NATS/Kafka + create streams
                                      3. initSSRF()             — SafeHTTPClient (if configured)
                                      4. initServer()           — Fiber HTTP + middlewares + TLS + security + CSRF + rate limit
                                      5. RegisterEntries()      — register all entry routes (9 types)
                                      6. Static files
                                      7. registerDocs()         — OpenAPI + Scalar UI
                                      8. initExit()             — start all event workers
                                      9. initCron()             — start cron scheduler
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

On SIGINT/SIGTERM:

1. Stop cron scheduler (waits for running jobs)
2. Drain exit workers (waits for in-flight handlers, 5s timeout)
3. Drain all event broker connections
4. Close all DB pools
5. Stop HTTP server

No manual cleanup needed.

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

Wraps a REST handler with `BeforeTransform`/`AfterTransform` hooks:

```go
svc.WithRest("convert", runtime.WrapTransformHandler(
    func(c *runtime.RestCtx) error {
        input := c.Locals("transformed").(MyModel)
        return c.JSON(fiber.Map{"name": input.Name})
    },
    &MyHooks{},
))
```

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
}
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

### HTTP pool sizing

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
