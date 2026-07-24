# Architecture

## Layers

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         sdk-api                                          │
├──────────┬──────────────┬───────────────┬──────────────┬──────────┬──────┤
│   db/    │   server/    │   events/     │   runtime/   │  infra/  │grpc/ │
│  (pgx)   │   (Fiber)    │  (NATS JS)    │ (orchestr.)  │ (go-zero)│      │
│          │              │               │              │          │      │
│ Table[T] │  20 middle-  │  Producers    │  Service     │  conf    │Server│
│ CRUD     │  wares       │  Consumers    │  GrpcServer  │  logx    │Client│
│ AutoInit │  JWT         │  KV Cache     │  GrpcClient  │  trace   │Interc│
│ Paginate │  CORS        │  Streams      │  YAML cfg    │  breaker │Resolv│
│ Filters  │  Trace       │               │  Hooks       │  discov  │      │
│          │  Breaker     │               │  Graceful    │  redis   │      │
│ PG/Turso │  Shedding    │               │  shutdown    │  mongo   │      │
│ MySQL    │  Prometheus  │               │  OpenAPI     │  cache   │      │
│ Mongo    │  Correlation │               │  Scalar UI   │  metric  │      │
└──────────┴──────────────┴───────────────┴──────────────┴──────────┴──────┘
```

## Communication Flow

```
                    ┌─────────────────┐
                    │  HTTP Client    │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Fiber Server   │
                    │  (port :8080)   │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │  internal/logic │
                    │  (shared logic) │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
    ┌─────────────┐  ┌──────────────┐  ┌──────────┐
    │  pgxpool DB │  │  NATS/Kafka  │  │  gRPC    │
    │  (SQL CRUD) │  │  (events)    │  │  Client  │
    └─────────────┘  └──────────────┘  └──────────┘
                                                │
                                                ▼
                                       ┌─────────────────┐
                                       │  gRPC Server    │
                                       │  (port :8081)   │
                                       └─────────────────┘
```

HTTP and gRPC share `internal/logic/`. The HTTP handler and gRPC server only parse/serialize — the business logic is the same.

## Package Layout

| Package | Technology | Purpose |
|---------|------------|---------|
| `db/` | pgxpool v5, database/sql | PostgreSQL, Turso/SQLite, MySQL, MongoDB |
| `server/` | Fiber (fasthttp) | REST API, 34+ middlewares, SSE, WebSocket |
| `events/` | NATS JetStream, Kafka | Producers, consumers, KV cache |
| `runtime/` | — | Service orchestrator, YAML, hooks, gRPC server/client |
| `infra/` | go-zero core | 45+ packages: conf, logx, trace, breaker, discov, metric |

## Global Middleware Chain Order

The order is critical. Global chain (applied to every request):

1. **Recover** — panic recovery (always on)
2. **HeaderSanitize** — CRLF injection protection (always on)
3. **Correlation** — `server.correlation.enabled` (X-Correlation-ID)
4. **Health** — liveness probe (always on)
5. **Trace** (OTel) — `telemetry.enabled`
6. **PrometheusHandler** — `/metrics` endpoint (always on)
7. **SecurityHeaders** — `server.security_headers` (block)
8. **CSRF** — `server.csrf.enabled`
9. **RateLimit** — `server.rate_limit.enabled`
10. **ContentSecurity** (RSA) — `server.security.content_security.enabled`
11. **Cryption** (AES) — `server.security.cryption.enabled`
12. **Logger** — `server.logger` (default `true`)
13. **Shedding** — `server.load_shedding` (default `true`)
14. **Breaker** — `server.breaker` (default `true`)
15. **MaxConns** — concurrent connection limit (always on)
16. **MaxBytes** — request body size limit (always on)
17. **Gunzip** — decompress gzip bodies (always on)
18. **PrometheusRecorder** — in-process metrics (always on)
20. **CORS** — `server.cors` (block)

**Per-entry middlewares** (applied per-route, not global): `JWT`, `JWTWithZitadel`, `Ory`, `OryJWT`, `OpenFGA`, `APIKey`, `MFA`, `AuthContext`, `ValidateInput`, `RateLimit`, `Timeout`, `Retry`, `Fallback`, `WebSocket`, `SSE`.

**gRPC interceptors** (auto-applied to gRPC server when `grpc_server` is configured): Trace, Breaker, Timeout, Shedding.

**Per-route mode** (`server.middleware` defined): only `Recover` + `Health` + `HeaderSanitize` are global. All others must be listed in `apply:`.
