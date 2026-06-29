# Runtime

The `Service` struct is the core orchestrator. It loads YAML config, initializes databases and NATS connections, registers HTTP routes (entry), starts NATS consumers (exit), and schedules cron jobs — all from a single `Run()` call.

## Service API

```go
// Create service from YAML
svc, err := runtime.New("service.yaml")

// Register entry handlers
svc.WithCRUD("Product", provider)           // CRUD auto-routes
svc.WithRest("hello", func(c) { ... })      // REST endpoints
svc.WithWS("chat", chatHandler)             // WebSocket
svc.WithSSE("stream", sseHandler)           // SSE

// Register entry hooks
svc.WithHooks("Product", &ProductHooks{})

// Register exit handlers (NATS workers)
svc.WithExit("onOrderConfirmed", handler)

// Register exit hooks
svc.WithExitHooks(map[string]runtime.ExitHooks{...})

// Register cron handlers
svc.WithCron("onCleanup", handler)

// Register models for OpenAPI schema generation
svc.RegisterModel("Product", (*Product)(nil))

// Access databases and NATS
svc.Pool("pg-main")      // any — returns the pool by name
svc.PoolPG("pg-main")   // *pgxpool.Pool — typed access
svc.NATS("primary")      // *events.Conn — NATS connection
svc.App()                // *fiber.App — raw Fiber access

// Start everything
svc.Run()
```

## Lifecycle

```
New(configPath) → LoadConfig + validate
  → Run()
    1. initDatabases()    — connect PG/Turso/MySQL pools
    2. initNATSList()     — connect NATS + create streams
    3. initServer()       — Fiber HTTP + middlewares + CORS
    4. RegisterEntries()  — register all entry routes
    5. Static files
    6. registerDocs()     — OpenAPI + Scalar UI
    7. initExit()         — start all NATS workers
    8. initCron()         — start cron scheduler
    → HTTP server starts
```

All steps are optional. No databases? Skip `databases:` in YAML. No HTTP? Only define `exit:` and `cron:`.

## Graceful Shutdown

On SIGINT/SIGTERM:

1. Stop cron scheduler (waits for running jobs)
2. Drain exit workers (waits for in-flight handlers, 5s timeout)
3. Drain all NATS connections
4. Close all DB pools
5. Stop HTTP server

No manual cleanup needed.

## RegisterModel

For OpenAPI schema generation, register Go struct types:

```go
type Product struct {
    ID    int64   `db:"id,primary,auto" json:"id"`
    Name  string  `db:"name,required" json:"name"`
}

svc.RegisterModel("Product", (*Product)(nil))
```

The function parses struct tags (`db`, `json`) to build OpenAPI property schemas. Multiple models can be registered, one per entry endpoint.

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

### Exit Hooks (NATS lifecycle)

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

The `CRUDProvider` interface bridges typed `db.Table[T]` to the router:

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
- `NewCRUDProvider[T](table, hooks)` — PostgreSQL (`db.Table[T]`)
- `NewMySQLCRUDProvider[T](table, hooks)` — MySQL (`db.MySQLTable[T]`)
- `NewTursoCRUDProvider[T](table, hooks)` — Turso (`db.TursoTable[T]`)

## Pool helpers

```go
runtime.Pool(pools, name)      // any → returns pool by name
runtime.PoolPG(pools, name)    // *pgxpool.Pool for PostgreSQL
runtime.PoolSQL(pools, name)   // *sql.DB for Turso/MySQL
runtime.TableFor[T](pools, poolName, tableName)  // *db.Table[T] from pool

// On the Service:
svc.Pool("pg-main")           // any
svc.PoolPG("pg-main")         // any (type assertion hidden)
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

Storage backends in YAML are auto-created when `entry[].storage` is specified. The backend is stored in `EntryHandlers.Storage[keyed by path]`.
