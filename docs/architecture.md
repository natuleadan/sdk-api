# Architecture

## Layers

```
┌────────────────────────────────────────────────────────────────────┐
│                         sdk-api                                   │
├──────────┬──────────────┬───────────────┬──────────────┬───────────┤
│   db/    │   server/    │   events/     │   runtime/   │  infra/   │
│  (pgx)   │   (Fiber)    │  (NATS JS)    │ (orchestr.)  │ (go-zero) │
│          │              │               │              │           │
│ Table[T] │  14 middle-  │  Producers    │  Service[T]  │  conf     │
│ CRUD     │  wares       │  Consumers    │  App (mono)  │  logx     │
│ AutoInit │  JWT         │  KV Cache     │  YAML cfg    │  trace    │
│ Paginate │  CORS        │  Streams      │  Hooks       │  breaker  │
│ Filters  │  Trace       │               │  Graceful    │  discov   │
│          │  Breaker     │               │  shutdown    │  redis    │
│ PG/Turso │  Shedding    │               │  OpenAPI     │  mongo    │
│ MySQL    │  Prometheus  │               │  Scalar UI   │  cache    │
└──────────┴──────────────┴───────────────┴──────────────┴───────────┘
```

## Communication Flow

```
HTTP Client → Fiber Server → Hooks → pgxpool DB → NATS Producer
                                        ↓
NATS Consumer → Fiber Server → Hooks → pgxpool DB → NATS Producer (next step)
```

**No direct HTTP between services.** All coordination is event-driven via NATS JetStream.

## Package Layout

| Package | Technology | Purpose |
|---------|------------|---------|
| `db/` | pgxpool v5, database/sql | PostgreSQL, Turso/SQLite, MySQL |
| `server/` | Fiber (fasthttp) | REST API, 14 middlewares, SSE, WebSocket |
| `events/` | NATS JetStream | Producers, consumers, KV cache |
| `runtime/` | — | Service orchestrator, YAML, hooks |
| `infra/` | go-zero core | 45 packages: conf, logx, trace, etc. |

## Service Modes

### Service[T] — Single-model microservice

One model → one table → one DB. Auto-generates CRUD endpoints. Best for microservices.

```go
svc, _ := runtime.New[Product]("service.yaml")
svc.Run()
```

### App — Multi-DB monolith

N models → N tables → N databases (PG + Turso + MySQL + Mongo) in one process.

```go
app, _ := runtime.NewApp("monolith.yaml")
app.AddDB(ctx, "pg-main", "postgres", pgURL)
app.AddDB(ctx, "local", "turso", tursoURL)
app.Run()
```

## Middleware Chain Order

The order is critical. Built-in chain:

1. **Trace** (OTel) — optional
2. **Logger** — always on
3. **Shedding** — adaptive load
4. **Breaker** — circuit breaker per route
5. **MaxConns** — concurrent connection limit
6. **MaxBytes** — request body size limit
7. **Gunzip** — decompress gzip bodies
8. **Prometheus** — metrics
9. **CORS** — optional
10. **Recover** — panic recovery
11. **Health** — liveness probe
12. **ContentSecurity** (RSA) — optional
13. **Cryption** (AES) — optional
14. **JWT** — optional
