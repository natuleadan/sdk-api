# Best Practices

## Benchmarking

- **`docker compose up --abort-on-container-exit`** runs functional tests by default. To run RPS benchmarks, set `RPS_BENCH=1`: `RPS_BENCH=1 docker compose up`.
- **6 endpoints are measured**: expand, list, getbyid, create, update, delete — each with 30s warmup + 30s measure.
- **Turso/Mongo writes are slow at high concurrency**: SQLite is single-writer; MongoDB BSON overhead limits throughput. Use Dragonfly/Redis as an L2 cache for read-heavy workloads.

## Entry vs Exit

- **Entry** = HTTP. User-initiated requests. Runs in `--mode entry`.
- **Exit** = NATS. Event-driven processing. Runs in `--mode exit`.
- A single binary + YAML can run both modes.

## Architecture Guidelines

### Service isolation

One YAML + one binary = one service. The binary can run in two modes:

```bash
./order-service --mode entry    # HTTP server (routes)
./order-service --mode exit     # NATS workers only
```

This allows independent scaling: the entry pod handles HTTP traffic, exit pods process events.

### Communication

- Services communicate via NATS JetStream, NOT HTTP.
- HTTP is for external clients (web, mobile, 3rd-party).
- Between microservices: publish NATS events, subscribe with exit workers.

### Database access

- Entry handlers access the DB for CRUD operations.
- Exit workers can also access the DB if `db:` is specified.
- All DB access goes through `CRUDProvider` wrappers.

## Configuration

### Environment variables in YAML

```yaml
url: "${DATABASE_URL}"
url: "nats://${NATS_HOST}:4222"
```

Don't hardcode secrets in YAML. Use `${VAR}` interpolation for all connection strings.

### Docker-specific config

Use `CONFIG_PATH` env var to switch config per environment:

```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" { cfgPath = "service.yaml" }
```

Create `service.docker.yaml` with Docker hostnames (`postgres:5432`, `nats:4222`) instead of `localhost`.

## Database

### Pool sizing

Don't hardcode `pool.max_conns`. Let the auto-sizing formula work:

```
max(1, (PG_SERVER_MAX_CONNS - reserved_conns) / REPLICA_COUNT)
```

Set `PG_SERVER_MAX_CONNS` and `REPLICA_COUNT` as env vars in your deployment.

### AutoInit

`AutoInit()` creates tables on startup but does NOT run ALTER TABLE migrations. Use a migration tool (golang-migrate, goose) for schema changes.

### Tags

- `db` tags control column names and constraints
- `json` tags control JSON field names in API responses
- They are independent. Don't rely on one for the other.

## Routing

### Path conflicts

Don't mix `rest` and `crud` on the same path. If your CRUD serves `/products` and you add a custom `GET /products`, disable the auto-generated GET and use an override:

```yaml
- type: crud
  model: Product
  db: pg-main
  table: products
  overrides:
    get: "-"        # Disable auto GET /products/:id
```

### ID parameters

CRUD endpoints use `/:id` as the ID parameter. Custom REST endpoints can use any `:param` name.

## NATS

### KV Cache vs Redis

NATS KV is built-in and doesn't need a separate Redis deployment. Use it for service-level caching. Redis may still be needed for share-nothing caches across services.

### Exit workers and streams

Make sure the NATS stream covers the subject your exit worker subscribes to. Stream `orders` covers `[orders, orders.>]`. If you need `orders.confirmed` specifically, either use `orders.confirmed` or change the stream subjects.

### Reply in exit workers

Only set `reply: true` when the publisher expects a response. For fire-and-forget event processing, keep `reply: false`.

## Performance

### Prefork

Enable `prefork: true` for multi-core throughput when the bottleneck is CPU-bound (middleware chain, JSON serialization, cache hits).

Prefork spawns N processes (N = CPUs). Each process has its own memory and goroutine scheduler. Don't use in-process caches when prefork is on — use NATS KV or Redis instead.

When the bottleneck is PostgreSQL (direct SELECT/INSERT per request), prefork does not improve throughput — all processes compete for the same PG connections.

### PgDog (PostgreSQL connection pooler)

For benchmarks or high-concurrency workloads with PostgreSQL, add PgDog between the app and PG. PgDog manages a small pool of connections to PG while accepting many connections from the app:

```
wrk -c1000 → app (pool=500) → PgDog (pool=20) → PG (max_connections=200)
```

This prevents PG from being overwhelmed by connection storms (e.g., 1000 wrk connections × 10 prefork processes = 10k simultaneous connection attempts).

Add PgDog to `docker-compose.yml`:
```yaml
pgdog:
  image: ghcr.io/pgdogdev/pgdog:main
  volumes:
    - ./pgdog.toml:/pgdog/pgdog.toml:ro
    - ./users.toml:/pgdog/users.toml:ro
  depends_on:
    postgres:
      condition: service_healthy
```

Point the service DATABASE_URL to PgDog:
```yaml
environment:
  DATABASE_URL: postgres://user:pass@pgdog:6432/db?sslmode=disable
```

### Benchmarks

Run benchmarks inside Docker with wrk. Running benchmarks on host + Docker data adds 2-4x latency. All built-in benchmarks are dockerized.

**Cache vs no-cache throughput difference:** Endpoints backed by a cache layer (Redis, NATS KV) can reach 150k+ RPS because 99% of requests never hit PostgreSQL. Endpoints that query PG on every request are limited to ~30k RPS in Docker Desktop (Mac) due to PG throughput. See `docs/benchmarks.md` for measured results.

## Gotchas

| Pitfall | Fix |
|---------|-----|
| `db` and `json` tags differ | Specify both explicitly. Never rely on auto-inference. |
| Hyphens in model names | Use PascalCase in Go, snake_case in DB. `toSnake("MyModel")` → `"my_model"` |
| NATS KV keys | Must match `[-/_=.[:alnum:]]`. No colons, spaces, or special characters. |
| OpenAPI without models | OpenAPI auto-generation requires `RegisterModel`. Without it, paths are generated but schemas are empty. |
| Cron with seconds | `robfig/cron` uses 5-field expressions. `"0 6 * * *"` works, `"*/10 * * * * *"` (6 fields) does NOT. |
| Service config validation | `LoadConfig` validates that `entry[].db` references exist in `databases:`. If using no databases, reference is skipped. |
| `svc.Pool()` before `svc.Run()` | `svc.Pool()` returns nil until `initDatabases()` completes inside `Run()`. Use `WithCRUDFactory()` (preferred, avoids nil pool) or `sync.Once` + lazy init. See `examples/400-auth/manual-pg` for the factory pattern. |
| CRUD path conflict with REST path | CRUD registers `GET /:id` which catches any sub-path like `/posts-fast`. Place custom REST endpoints on a different base path (e.g., `/debug/items` instead of `/posts/list`). |
| Error messages with IPs/conn strings | Error sanitizer redacts IPs (`10.0.0.5` → `[redacted]`), connection URLs, and file paths from all error responses. 5xx returns `"internal server error"`. |
| Multi-tenant CRUD | Use `tenant_scope` + `tenant_field` on CRUD entries. The SDK automatically injects `WHERE tenant_field = <org_id>` on all queries. Client-supplied `tenant_id` values are ignored on create — the JWT claim takes precedence. |

## Error Handling

All errors must use the `errcode` package to return structured errors with machine-readable codes and user-safe messages.

### Error codes

| Code | HTTP Status | When |
|------|-------------|------|
| `ERR_NOT_FOUND` | 404 | Resource not found |
| `ERR_VALIDATION` | 400 | Invalid input |
| `ERR_UNAUTHORIZED` | 401 | Missing or invalid credentials |
| `ERR_FORBIDDEN` | 403 | Insufficient permissions |
| `ERR_RATE_LIMITED` | 429 | Rate limit exceeded |
| `ERR_TIMEOUT` | 504 | Operation timed out |
| `ERR_DB_QUERY` | 500 | Database query failure |
| `ERR_DB_CONNECTION` | 500 | Database connection failure |
| `ERR_NATS` | 500 | NATS messaging failure |
| `ERR_INTERNAL` | 500 | Unexpected internal error |

### Return structured errors

```go
import "github.com/natuleadan/sdk-api/runtime/errcode"

// Inside a handler or middleware:
return errcode.ErrNotFound("product", id)
return errcode.ErrValidation("email", "required", input.Email)
return errcode.ErrUnauthorized("invalid token")
return errcode.ErrDBQuery("select", "users", err)
return errcode.ErrRateLimited(5)
```

### Response format

```json
// 4xx — descriptive message
{"code": 401, "error": "ERR_UNAUTHORIZED", "message": "invalid token"}

// 5xx — safe message, details in logs
{"code": 500, "error": "ERR_DB_QUERY", "message": "Database operation failed"}
```

### Log output

Errors are logged automatically by the error handler with full stack traces
and attributes via `logx.Errorw`. Do NOT log before returning the error.

### Panic recovery

Panics in handlers are caught by the `recover` middleware. Panics in background
goroutines should use `crypto/rand` failures log via `logx.Errorf`.

### Error handling rules

| Pattern | Allowed? | Alternative |
|---------|----------|-------------|
| `_ = funcCall()` | ❌ | `if err := funcCall(); err != nil { logx.Errorf(...) }` |
| `_, _ = funcCall()` | ❌ | `if _, err := funcCall(); err != nil { logx.Errorf(...) }` |
| `if _, err := funcCall(); err != nil { return }` | ✅ | — |
| `if err := funcCall(); err != nil { logx.Errorf(...) }` | ✅ | — |

- **HTTP handlers**: return the error (using `errcode.*`) to Fiber's error handler
- **Background goroutines**: log via `logx.Errorf` — errors cannot propagate
- **NATS message processing**: log via `logx.Errorf`, then Nak or Ack appropriately

## Deployment

### Config embedding

The generated `main.go` uses `//go:embed service.yaml` with `runtime.NewFromYAML()`. This embeds the config into the binary, eliminating filesystem dependencies at runtime.

```go
//go:embed service.yaml
var configYAML []byte

svc, err := runtime.NewFromYAML(configYAML)
```

This works on all platforms — Vercel, Docker, Kubernetes, bare-metal — without extra deployment steps.

### S3 storage best practices

**HTTP pool sizing:** Configure `storage.pool` in `type: file` entries to match expected concurrency. Default `max_idle_conns=200` handles `wrk -c1000`. Without it, `MaxIdleConnsPerHost=2` limits throughput to ~500 req/s.

```yaml
storage:
  mode: s3
  pool:
    max_idle_conns: 200
    max_idle_conns_per_host: 100
    max_conns_per_host: 250
```

**Cache strategy:** Use `cache:` for read-heavy workloads. L1 RAM (hot data) + L2 disk (warm data) + S3 (cold data). First request hits S3, subsequent requests served from RAM (~50x faster).

**Presigned URLs vs proxy:** Use `presign: true` for downloads where the client can follow a redirect. Server bandwidth drops to zero. Use proxy (default) when you need access control, logging, or transformation on every request.

**Provider compatibility:** All S3-compatible providers work (AWS, MinIO, R2, Backblaze B2, DigitalOcean Spaces). Set `endpoint` to your provider's S3 URL.

### Platform-specific validation

Set `deploy.target` in `service.yaml` to validate platform-specific constraints at startup:

```yaml
deploy:
  target: vercel    # auto | vercel | docker | kube | bare-metal
```

| Target | CLI command | Enforced rules |
|--------|-------------|---------------|
| Vercel | `sdk-api vercel` | prefork=false, tls=false |
| Docker | `sdk-api docker` | Project structure |
| Kubernetes | `sdk-api kube` | name + image required |
| bare-metal | — | No restrictions |

## Linting Rules

The project enforces these rules via `golangci-lint`:

| Rule | Enforced by | Why |
|------|-------------|-----|
| 0 `//nolint` comments | Project policy | Every issue must be fixed, not silenced |
| 0 `_ =` error ignores | `errcheck` | Every error must be handled or logged |
| 0 unused params | `unparam` | Dead parameters removed or used |
| Complexity < 15 | `gocyclo` | Functions must be testable and maintainable |
| No deprecated APIs | `staticcheck` SA1019 | Prevent build breaks on dependency upgrades |
| Custom context keys | `staticcheck` SA1029 | Prevent key collisions across packages |

### Linter removals

## Pool health check

Use `runtime.CheckPoolHealth(name, driver, pool)` to inspect database connection pool status:

```go
h := runtime.CheckPoolHealth("pg-main", "postgres", pool)
// → {Name, Driver, TotalConns, InUseConns, UtilizationPct, Status}
```

Supported pool types: `*pgxpool.Pool` (PostgreSQL), `*sql.DB` (MySQL, Turso).
Status: `"healthy"` (<60% utilization), `"degraded"` (60-80%), `"saturated"` (>80%).

## Testing patterns

See `docs/testing.md` for the complete testing guide. Key points:

- Unit tests: `go test -short ./...` (no Docker required)
- Integration tests: `make test-integration` (requires Docker)
- Use testify (`assert`/`require`) for all assertions
- Add `t.Parallel()` to every test function
- Fuzz parsers and validators with `go test -fuzz`

Two linters were evaluated against go-zero's philosophy and intentionally removed:

**`wrapcheck`** — requires all errors from external packages to be wrapped with `%w`.
Removed because go-zero does not enforce error wrapping. The SDK uses structured logging
via `logx` and trace IDs for debugging. `errorx.Wrap()` remains available as an optional
utility for user code.

**`revive`** — enforces naming conventions and documentation comments on exported symbols.
Removed because all issues were style-only (`exported X should have comment or be unexported`).
Comments on exported symbols remain a best practice but are not enforced by CI.
