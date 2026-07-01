# Best Practices

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

Enable `prefork: true` for multi-core throughput. Each process has its own memory — don't use in-process caches when prefork is on. Use NATS KV instead.

### Benchmarks

Run benchmarks inside Docker with wrk. Running benchmarks on host + Docker data adds 2-4x latency. All built-in benchmarks are dockerized.

## Gotchas

| Pitfall | Fix |
|---------|-----|
| `db` and `json` tags differ | Specify both explicitly. Never rely on auto-inference. |
| Hyphens in model names | Use PascalCase in Go, snake_case in DB. `toSnake("MyModel")` → `"my_model"` |
| NATS KV keys | Must match `[-/_=.[:alnum:]]`. No colons, spaces, or special characters. |
| OpenAPI without models | OpenAPI auto-generation requires `RegisterModel`. Without it, paths are generated but schemas are empty. |
| Cron with seconds | `robfig/cron` uses 5-field expressions. `"0 6 * * *"` works, `"*/10 * * * * *"` (6 fields) does NOT. |
| Service config validation | `LoadConfig` validates that `entry[].db` references exist in `databases:`. If using no databases, reference is skipped. |

## Error Handling

All errors must be handled explicitly. Silent ignores (`_ = funcCall()`, `_, _ = funcCall()`) are prohibited.

| Pattern | Allowed? | Alternative |
|---------|----------|-------------|
| `_ = funcCall()` | ❌ | `if err := funcCall(); err != nil { logx.Errorf(...) }` |
| `_, _ = funcCall()` | ❌ | `if _, err := funcCall(); err != nil { logx.Errorf(...) }` |
| `if _, err := funcCall(); err != nil { return }` | ✅ | — |
| `if err := funcCall(); err != nil { logx.Errorf(...) }` | ✅ | — |

- **HTTP handlers**: return the error to Fiber's error handler
- **Background goroutines**: log via `logx.Errorf` — cannot propagate
- **NATS message processing**: log via `logx.Errorf`, then Nak or Ack appropriately
- **Interface implementations**: if the interface requires unused params, use the param name (not `_`) and document why
- **`crypto/rand` failures**: log via `logx.Errorf` — extremely rare but should not silently produce zero bytes

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
