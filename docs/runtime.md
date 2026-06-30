# Runtime

The `Service` struct is the core orchestrator. It loads YAML config, initializes databases and event streams, registers HTTP routes (entry), starts workers (exit), schedules cron jobs, and configures security — all from a single `Run()` call.

## Service API

```go
// Create service from YAML
svc, err := runtime.New("service.yaml")

// Register entry handlers
svc.WithCRUD("Product", provider)           // CRUD auto-routes
svc.WithRest("hello", func(c) { ... })      // REST endpoints
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
svc.NATS("primary")              // events.EventBroker — event broker
svc.SafeHTTPClient()             // *middleware.SafeHTTPClient — SSRF-protected HTTP client
svc.App()                        // *fiber.App — raw Fiber access

// Start everything
svc.Run()
```

## Lifecycle

```
New(configPath) → LoadConfig + validate
  → Run()
    1. initDatabases()        — connect PG/Turso/MySQL pools
    2. initEventStreams()     — connect NATS/Kafka + create streams
    3. initSSRF()             — SafeHTTPClient (if configured)
    4. initServer()           — Fiber HTTP + middlewares + TLS + security headers + CSRF + rate limit
    5. RegisterEntries()      — register all entry routes (9 types)
    6. Static files
    7. registerDocs()         — OpenAPI + Scalar UI
    8. initExit()             — start all event workers
    9. initCron()             — start cron scheduler
    → HTTP server starts
```

All steps are optional. No databases? Skip `databases:` in YAML. No HTTP? Only define `exit:` and `cron:`.

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
    func(c *fiber.Ctx) error {
        input := c.Locals("transformed").(MyModel)
        return c.JSON(fiber.Map{"name": input.Name})
    },
    &MyHooks{},
))
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

Three implementations:
- `NewCRUDProvider[T](table, hooks)` — PostgreSQL
- `NewMySQLCRUDProvider[T](table, hooks)` — MySQL
- `NewTursoCRUDProvider[T](table, hooks)` — Turso

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
    Endpoint:  "http://minio:9000",
    Bucket:    "uploads",
    AccessKey: "minioadmin",
    SecretKey: "minioadmin",
})
```

Storage backends in YAML are auto-created when `entry[].storage` is specified.
