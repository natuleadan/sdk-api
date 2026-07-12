# Architecture

## Layers

```
┌────────────────────────────────────────────────────────────────────┐
│                         sdk-api                                   │
├──────────┬──────────────┬───────────────┬──────────────┬───────────┤
│   db/    │   server/    │   events/     │   runtime/   │  infra/   │
│  (pgx)   │   (Fiber)    │  (NATS JS)    │ (orchestr.)  │ (go-zero) │
│          │              │               │              │           │
│ Table[T] │  18 middle-  │  Producers    │  Service[T]  │  conf     │
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
| `server/` | Fiber (fasthttp) | REST API, 18 middlewares, SSE, WebSocket |
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

## Global Middleware Chain Order

The order is critical. Global chain (applied to every request):

1. **Recover** — panic recovery (always on)
2. **HeaderSanitize** — CRLF injection protection (always on)
3. **Health** — liveness probe (always on)
4. **Trace** (OTel) — `telemetry.enabled`
5. **PrometheusHandler** — `/metrics` endpoint (always on)
6. **SecurityHeaders** — `server.security_headers` (block)
7. **CSRF** — `server.csrf.enabled`
8. **RateLimit** — `server.rate_limit.enabled`
9. **ContentSecurity** (RSA) — `server.security.content_security.enabled`
10. **Cryption** (AES) — `server.security.cryption.enabled`
11. **Logger** — `server.logger` (default `true`)
12. **Shedding** — `server.load_shedding` (default `true`)
13. **Breaker** — `server.breaker` (default `true`)
14. **MaxConns** — concurrent connection limit (always on)
15. **MaxBytes** — request body size limit (always on)
16. **Gunzip** — decompress gzip bodies (always on)
17. **PrometheusRecorder** — in-process metrics (always on)
18. **CORS** — `server.cors` (block)

**Per-entry middlewares** (applied per-route, not global): `JWT`, `JWTWithZitadel`, `OpenFGA`, `Ory`, `APIKey`, `ValidateInput`, `RateLimit` (per-entry), `Timeout`.

**Per-route mode** (`server.middleware` defined): only `Recover` + `Health` + `HeaderSanitize` are global. All others must be listed in `apply:`.
