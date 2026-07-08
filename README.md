# Natuleadan SDK API

<p align="center">
  <img src="https://avatars.githubusercontent.com/u/210283438?s=400&u=1afe4cf2a1a5347c739f4efc60b86e3c1564cb6&v=4" width="120" height="120" style="border-radius: 50%;">
  <br>
  <b>CLI:</b> <code>sdk-api</code> — <b>Module:</b> <code>github.com/natuleadan/sdk-api</code>
</p>

<p align="center">
  <a href="https://github.com/natuleadan/sdk-api/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/natuleadan/sdk-api/ci.yml?style=for-the-badge&label=CI&logo=github"></a>
  <a href="https://github.com/natuleadan/sdk-api/releases/latest"><img src="https://img.shields.io/github/v/release/natuleadan/sdk-api?style=for-the-badge&label=Release&logo=github"></a>
  <br>
  <a href="https://pkg.go.dev/github.com/natuleadan/sdk-api"><img src="https://img.shields.io/badge/Go-Reference-00ADD8?style=for-the-badge&logo=go"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/github/go-mod/go-version/natuleadan/sdk-api?style=for-the-badge&logo=go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge"></a>
  <a href="https://conventionalcommits.org"><img src="https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge"></a>
</p>

A production-ready Go SDK for building event-driven microservices and monoliths. **Fork of [go-zero](https://github.com/zeromicro/go-zero)** with **Fiber** (fasthttp), **pgx** (PostgreSQL), **NATS JetStream** + **Kafka** (messaging), per-route middleware, security, and YAML-driven configuration.

---

## 1. Install

```bash
go install github.com/natuleadan/sdk-api/cmd/sdk-api@latest
```

Or download a pre-built binary from the [releases page](https://github.com/natuleadan/sdk-api/releases).

## 2. Quick Start

Create a new service with a single command:

```bash
sdk-api new products-svc --model Product --fields "name:string,price:float64"
cd products-svc
```

This generates `service.yaml`, `main.go`, `models/product.go`, and `models/hooks.go`:

```yaml
name: products-svc
port: 8080

databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"

entry:
  - type: crud
    model: Product
    db: pg-main
    table: products

server:
  openapi:
    enabled: true
```

Start PostgreSQL and run:

```bash
docker run -d --name pg -p 5432:5432 \
    -e POSTGRES_USER=dev -e POSTGRES_PASSWORD=devpass \
    postgres:18-alpine

DATABASE_URL="postgres://dev:devpass@localhost:5432/postgres?sslmode=disable" \
    go run .
```

Your service is live with auto-generated CRUD, OpenAPI docs, and Prometheus metrics:

```bash
curl http://localhost:8080/api/v1/products
open http://localhost:8080/docs
curl http://localhost:8080/health
```

## 3. Features

| Category | Feature | Description |
|----------|---------|-------------|
| **Entry types** | `crud` | Auto-generated List/Get/Create/Update/Delete with hooks |
| | `rest` | Custom handler, any method (GET/POST/PUT/PATCH/DELETE) |
| | `webhook` | Defaults to POST, no JWT by default |
| | `websocket` | WebSocket upgrade handler |
| | `sse` | Server-Sent Events streaming |
| | `file` | Upload/download/delete, S3 or local storage |
| | `async` | 202 Accepted + status polling for long-running jobs |
| | `graphql` | Auto-generated schema + CRUD resolvers |
| **Event streams** | NATS JetStream | Push/pull consumers, KV, request-reply |
| | Kafka | Topic-based pub/sub, consumer groups |
| | `EventBroker` | Unified interface for NATS/Kafka duality |
| **Security** | Security headers | X-Frame-Options, CSP, HSTS, Permissions-Policy, etc. |
| | CSRF | Double-submit cookie pattern |
| | Rate limiting | Token bucket (memory or Redis), per IP/user/global |
| | TLS | Manual certs or Let's Encrypt autocert |
| | SSRF protection | Blocks private/loopback/metadata IPs |
| | Input validation | Struct tag validation via `go-playground/validator/v10` |
| | Error sanitization | Generic 500 messages, no internal leak |
| **Auth** | JWT per-entry | Algorithm pinning, claim validation, AuthContext extraction |
| | 4 auth modes | `none`, `manual`, `openfga-zitadel`, `ory` |
| | OpenFGA + Zitadel | ReBAC authorization + OIDC authentication |
| | Ory Kratos + Keto | Identity management + fine-grained authorization |
| | API keys | Service-to-service with scoped permissions |
| | Token refresh | Delegated to IDP (Zitadel/Kratos) or manual re-sign |
| **Cron** | `nats` mode | Publish message to NATS on schedule |
| | `handler` mode | Call a Go function on schedule |
| | `internal` mode | System tick without handler |
| **Databases** | PostgreSQL | `pgxpool` with auto-sizing pool |
| | MySQL | via `go-sql-driver/mysql` |
| | Turso | Embedded SQLite via libsql |
| | Multi-DB | Each entry/exit can use a different database |
| **Server** | 14 built-in middlewares | Logger, breaker, JWT, CORS, tracing, shedding, timeout, etc. |
| | Per-route middleware | Configure middlewares per path in YAML |
| | Security headers + CSRF + Rate limit | YAML-driven security layers |
| | TLS (manual + autocert) | HTTPS + HTTP→HTTPS redirect |
| | Error sanitization + CRLF protection | Built-in, always on |
| | OpenAPI 3.0.3 + Scalar | Auto-generated spec + UI `/docs` |
| | Health + Metrics | `/health`, `/metrics` |
| **Storage** | S3-compatible | MinIO, R2, AWS S3 |
| | Local filesystem | Path-constrained uploads |

### Benchmarks

| Scenario | Stack | RPS |
|----------|-------|-----|
| Minimal HTTP | raw Fiber | ~700k |
| URL expand | SDK (PG + Redis) | ~31k |
| URL expand | SDK (PG + NATS KV) | ~31k |
| Product by ID | SDK (MySQL) | ~25k |
| Product by ID | SDK (Turso) | ~20k |
| Product by ID | SDK (MongoDB) | ~18k |

Full benchmarks in [`docs/benchmarks.md`](docs/benchmarks.md).

## 4. Use as Go SDK

```go
import "github.com/natuleadan/sdk-api"

// Option 1: YAML-driven (config embedded in binary)
//go:embed service.yaml
var configYAML []byte
svc, _ := runtime.NewFromYAML(configYAML)
pool := svc.Pool("pg-main").(*pgxpool.Pool)
table, _ := db.NewTable[Product](pool, "products")
svc.WithCRUD("Product", runtime.NewCRUDProvider(table, &ProductHooks{}))
svc.WithExit("onOrderConfirmed", func(ctx, msg []byte) ([]byte, error) {
    log.Printf("received: %s", string(msg))
    return nil, nil
})
svc.RegisterModel("Product", (*Product)(nil))
svc.Run()

// Option 2: Client mode (connect to existing service)
client, _ := runtime.NewClient("service.yaml")
resp, _ := client.Request("orders", "orders.transform", []byte(`{"id": 1}`))
```

## 5. Server Config

### Server fields

| Field | Default | Description |
|-------|---------|-------------|
| `host` | `0.0.0.0` | Bind address |
| `prefork` | `false` | Fiber prefork (SO_REUSEPORT, Linux only) |
| `body_limit` | `4194304` | Max body size |
| `timeout` | `30s` | Read/write/idle timeout |
| `api_prefix` | `/api/v1` | Prefix for all entry paths |
| `health_path` | `/health` | Liveness probe |
| `metrics_path` | `/metrics` | Prometheus endpoint |
| `shutdown_timeout` | `10s` | Graceful shutdown wait |

### Entry types

| Type | HTTP Methods | What you write | DB required |
|------|-------------|----------------|-------------|
| `crud` | GET, POST, PATCH, DELETE | Nothing | Yes |
| `rest` | Any | Single handler | Optional |
| `webhook` | Any (default POST) | Single handler | Optional |
| `websocket` | GET (upgrade) | WS handler | No |
| `sse` | GET | SSE handler | No |
| `file` | GET, POST, PUT, PATCH, DELETE | Upload handler | No |
| `async` | POST, GET | Async handler | No |
| `graphql` | POST | Nothing | No |

### Exit worker fields

| Field | Default | Description |
|-------|---------|-------------|
| `subscribe.stream` | required | NATS stream name |
| `subscribe.subject` | stream name | Subject to subscribe to |
| `handler` | required | Go function name |
| `max_concurrent` | 1 | Max concurrent messages |
| `reply` | false | Enable request-reply via `msg.Respond()` |
| `reply_timeout` | `30s` | Reply timeout |
| `consumer_mode` | `push` | `push` or `pull` |
| `pull_batch` | 0 | Batch size for pull consumers |

## 6. Event Flow

```
HTTP Entry → event_publish → NATS Stream → Exit Worker → DB/NATS/Email
      ↑                                      │
      └────── Request-Reply (reply: true) ───┘

Cron → Publish NATS → Exit Worker (subscribed)
```

Packages involved:

```
┌──────────┬─────────────┬──────────────┬─────────────┬───────────┐
│   db/    │   server/   │   events/    │  runtime/   │  auth/    │
│  (pgx)   │  (Fiber)    │  (NATS JS)   │ (orchestr.) │ (ext.)    │
│          │             │              │             │           │
│ Table[T] │ 14+ middle- │  Producers   │ Service     │ OpenFGA   │
│ CRUD     │ wares       │  Exit Workers│ YAML cfg    │ Zitadel   │
│ AutoInit │ JWT/CORS    │  KV Cache    │ Entry routes│ Ory       │
│ PG/Turso │ SSE/WS      │  Request-    │ Exit workers│ API Keys  │
│ MySQL    │ OpenAPI      │  Reply       │ Cron        │ AuthCtx   │
│          │ Auth (4 modes)│             │             │           │
└──────────┴─────────────┴──────────────┴─────────────┴───────────┘
```

## 7. Documentation

| File | Contents |
|------|----------|
| [docs/configuration.md](docs/configuration.md) | Full YAML schema reference (all entry types, event streams, security) |
| [docs/security.md](docs/security.md) | Consolidated security guide (headers, CSRF, rate limit, TLS, SSRF, validation, auth, JWT, OpenFGA, Zitadel, Ory, API keys) |
| [docs/http-server.md](docs/http-server.md) | Server config, middlewares, TLS, security headers, CSRF, rate limit |
| [docs/runtime.md](docs/runtime.md) | Runtime API: `Service`, `CRUDProvider`, `RegisterValidation`, `SafeHTTPClient` |
| [docs/messaging.md](docs/messaging.md) | NATS + Kafka, EventBroker, producers, exit workers, KV cache |
| [docs/database.md](docs/database.md) | PG/Turso/MySQL, pool sizing, multi-DB, Table[T] CRUD |
| [docs/entry-graphql.md](docs/entry-graphql.md) | Auto-generated GraphQL schema + resolvers |
| [docs/entry-async.md](docs/entry-async.md) | Async jobs: 202 Accepted + status polling |
| [docs/secrets.md](docs/secrets.md) | Secrets management: ${VAR}, SOPS/age, best practices |
| [docs/api-patterns.md](docs/api-patterns.md) | Entry types, exit workers, cron, hooks with YAML + Go examples |
| [docs/best-practices.md](docs/best-practices.md) | Gotchas, patterns, anti-patterns |
| [docs/cli.md](docs/cli.md) | `sdk-api new/docker/kube/vercel/client` subcommands |
| [docs/deployment-vercel.md](docs/deployment-vercel.md) | Vercel deployment guide with embed config and validation |
| [docs/benchmarks.md](docs/benchmarks.md) | Full benchmark results and methodology |
| [docs/architecture.md](docs/architecture.md) | Architecture, entry router, exit system, cron design |
| [docs/getting-started.md](docs/getting-started.md) | Step-by-step tutorial for first service |

## 8. Examples

### 8.1 CRUD with hooks

```yaml
entry:
  - type: crud
    model: Product
    db: pg-main
    table: products
    path: /products
```

```go
type ProductHooks struct{ runtime.DefaultHooks[Product] }

func (h *ProductHooks) BeforeCreate(ctx context.Context, req Product) (Product, error) {
    req.CreatedAt = time.Now()
    return req, nil
}

pool := svc.Pool("pg-main").(*pgxpool.Pool)
table, _ := db.NewTable[Product](pool, "products")
svc.WithCRUD("Product", runtime.NewCRUDProvider(table, &ProductHooks{}))
```

### 8.2 REST endpoint with event_publish

```yaml
entry:
  - type: rest
    method: POST
    path: /orders/transform
    handler: onTransform
    event_publish:
      - stream: orders
        subject: orders.transformed
```

```go
svc.WithRest("onTransform", func(c *runtime.RestCtx) error {
    var req Order
    jsonx.Unmarshal(c.Body(), &req)
    req.Status = "transformed"
    return c.JSON(req)
})
```

### 8.3 Exit worker with reply

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onEmail
    reply: true
    reply_timeout: 30s
```

```go
svc.WithExit("onEmail", func(ctx, msg []byte) ([]byte, error) {
    log.Printf("sending email for: %s", string(msg))
    return []byte(`{"sent": true}`), nil
})
```

### 8.4 Cron job

```yaml
cron:
  - name: daily-report
    schedule: "0 6 * * *"
    mode: handler
    handler: onDailyReport
```

```go
svc.WithCron("onDailyReport", func(ctx context.Context) error {
    log.Println("generating daily report...")
    return nil
})
```

## 9. Project Structure

```
cmd/sdk-api/     # CLI generator (new/docker/kube/client)
runtime/         # Service orchestrator, entry router, exit workers, cron, hooks
server/          # Fiber HTTP + 14 middlewares + storage backends
db/              # Table[T] CRUD (PG, Turso, MySQL) + AutoInit
events/          # NATS JetStream producers, consumers, KV cache, request-reply
infra/           # 45+ go-zero packages (conf, logx, trace, breaker, redis, discover, ...)
docs/            # Documentation + API patterns
examples/        # 8 dockerized example services + benchmarks
```

## 10. License

MIT — see [LICENSE](LICENSE). Forked from [go-zero](https://github.com/zeromicro/go-zero) (MIT).
