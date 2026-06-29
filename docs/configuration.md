# Configuration

sdk-api is **YAML-driven**. Everything is declared in a single `service.yaml` — no code scaffolding for config, no magic. The YAML defines databases, NATS connections, entry endpoints (HTTP), exit workers (NATS consumers), cron jobs, and server settings.

## Full Schema

```yaml
name: my-service              # Service name
port: 8080                    # HTTP port (env override: $PORT)

# ---- Databases (multiple, by driver) ----
databases:
  - name: pg-main             # Reference name used by entry: db:
    driver: postgres          # postgres | turso | mysql
    url: "${DATABASE_URL}"    # Connection string (env interpolation)
    pool:
      max_conns: 10           # Auto-calculated from PG_SERVER_MAX_CONNS if <= 0
      min_conns: 2
      max_conn_lifetime: 30m
      max_conn_idle_time: 5m
      health_check_period: 1m
      reserved_conns: 10

# ---- NATS (multiple connections) ----
nats:
  - name: primary             # Reference name used by exit workers
    url: "${NATS_URL}"
    max_reconnects: 10
    reconnect_wait: 2s
    timeout: 5s
    retry_on_fail: true
    streams:
      - name: orders          # Stream name
        max_age: 24h
        max_bytes: 1073741824 # 1GB
        storage: file          # file | memory
        compression: s2       # s2 | none

# ---- Entry endpoints (HTTP) ----
entry:
  # CRUD — auto-generates List/Get/Create/Update/Delete
  - type: crud
    model: Product
    db: pg-main
    table: products
    path: /products            # Defaults to /{resource}
    overrides:
      list: ~                 # ~ = use default handler
      get: "onCustomGet"      # Use custom handler from Rest map
      create: "-"             # "-" = disable this endpoint
      update: ~
      delete: ~

  # REST — single endpoint, no auto-generation
  - type: rest
    method: GET
    path: /products/:id/transform
    handler: onTransformProduct
    auth: true                 # JWT required
    nats_publish:              # Auto-publish to NATS after handler
      - stream: orders
        subject: orders.transformed

  # Webhook — POST endpoint (JWT not required by default)
  - type: webhook
    path: /webhooks/sendgrid
    handler: onInboundEmail
    nats_publish:
      - stream: email
        subject: email.received

  # WebSocket
  - type: websocket
    path: /ws/chat
    handler: onChat

  # SSE — Server-Sent Events
  - type: sse
    path: /events/stream
    handler: onStream

  # File upload/download
  - type: file
    method: POST
    path: /files/upload
    handler: onFileUpload
    allowed_types:
      - image/png
      - image/jpeg
      - application/pdf
    max_size: 10MB
    storage:
      mode: s3                  # s3 | local
      bucket: uploads
      endpoint: "http://minio:9000"
      access_key: "${MINIO_ACCESS_KEY}"
      secret_key: "${MINIO_SECRET_KEY}"

# ---- Exit workers (NATS consumers) ----
exit:
  - name: email-sender
    subscribe:
      stream: orders           # NATS stream name
      subject: orders.confirmed # defaults to stream name
    handler: onOrderConfirmed
    max_concurrent: 10          # Goroutine pool per worker
    db: pg-main                 # Workers can also access databases
    reply: false                # true = handler returns data via Respond()
    reply_timeout: 30s
    pull_batch: 0               # > 0 = use pull consumer (fetch batch)
    consumer_mode: push         # push | pull

# ---- Cron (scheduled jobs) ----
cron:
  - name: daily-report
    schedule: "0 6 * * *"       # Standard 5-field cron
    mode: nats                  # nats | handler | internal
    publish:
      stream: cron
      subject: cron.daily-report

  - name: cleanup-expired
    schedule: "0 */4 * * *"
    mode: handler               # Calls Go handler
    handler: onCleanupExpired

  - name: health-check
    schedule: "@every 1h"
    mode: internal              # Internal system tick (log only)

# ---- Server config ----
server:
  host: "0.0.0.0"
  prefork: false               # Fiber SO_REUSEPORT
  body_limit: 4194304          # 4 MB
  timeout: 30s
  max_conns: 1000
  max_bytes: 4194304
  metrics_path: /metrics
  health_path: /health
  shutdown_timeout: 10s
  recover_stack: true
  api_prefix: /api/v1
  cors:
    origins:
      - "*"
    methods:
      - GET
      - POST
      - PUT
      - PATCH
      - DELETE
    credentials: false
    max_age: 300
  static:
    - prefix: /static
      dir: ./public
  middleware:
    - path: "/api/v1/*"
      apply:
        - logger
        - breaker
        - cors
  openapi:                     # Auto-generated API docs
    enabled: true
    version: "1.0.0"
    spec_path: /openapi.json
    docs_path: /docs
    theme: moon
    dark_mode: true
```

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | Service name (required) |
| `port` | int | `8080` | HTTP port. Overridden by `$PORT` env var |
| `databases` | array | `[]` | Database connections (see below) |
| `nats` | array | `[]` | NATS connections (see below) |
| `entry` | array | `[]` | HTTP endpoint definitions (6 types) |
| `exit` | array | `[]` | NATS worker definitions |
| `cron` | array | `[]` | Scheduled job definitions |
| `server` | object | (defaults) | HTTP server configuration |
| `telemetry` | object | — | OpenTelemetry (not yet wired) |

## Databases

Each entry in `databases:` connects one database. Multiple entries = multiple databases.

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 10
      min_conns: 2
```

| Field | Description |
|-------|-------------|
| `name` | Reference name. Used by `entry[].db` and `exit[].db` |
| `driver` | `postgres` (or `pg`), `turso`, `mysql` |
| `url` | Connection string. Supports `${VAR}` env interpolation |
| `pool.max_conns` | Max connections. If `<= 0`, auto-calculated: `max(1, (PG_SERVER_MAX_CONNS - reserved) / REPLICA_COUNT)` |
| `pool.min_conns` | Min idle connections |
| `pool.reserved_conns` | Reserved connections for other services |

## NATS

```yaml
nats:
  - name: primary
    url: "${NATS_URL}"
    streams:
      - name: orders
```

| Field | Description |
|-------|-------------|
| `name` | Reference name. Used internally |
| `url` | NATS server URL |
| `streams[].name` | Stream name. Created on startup if not exists |
| `streams[].max_age` | Message max age (default: `24h`) |
| `streams[].max_bytes` | Stream max bytes (default: 1GB) |
| `streams[].storage` | `file` (default) or `memory` |
| `streams[].compression` | `s2` (default) or `none` |

Default subjects for a stream named `orders`: `[orders, orders.>]` (wildcard for all sub-subjects).

## Entry (HTTP endpoints)

6 types of entry endpoints:

### `type: crud`
Auto-generates 5 endpoints for a model:

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/resource` | List (paginated, filterable, sortable) |
| GET | `/resource/:id` | Get by primary key |
| POST | `/resource` | Create (with entry hooks) |
| PATCH | `/resource/:id` | Partial update |
| DELETE | `/resource/:id` | Delete |

CRUD overrides control individual endpoints:

```yaml
overrides:
  list: ~              # (empty) → use auto-generated handler
  get: "onCustomGet"   # string → override with custom handler
  create: "-"          # "-" → disable this endpoint
  update: ~
  delete: "-"
```

Required fields: `model`, `db`, `table`. Auto-resolves `path` from `resource`, `resource` from `table` (pluralizes).

### `type: rest`
Single endpoint with any HTTP method. No auto-generation.

```yaml
- type: rest
  method: GET
  path: /products/:id/transform
  handler: onTransformProduct
  auth: true
  nats_publish:
    - stream: orders
      subject: orders.transformed
```

Required: `method`, `path`, `handler`. Optional: `auth`, `nats_publish`.

### `type: webhook`
Defaults to POST if omitted. No JWT validation by default.

```yaml
- type: webhook
  path: /webhooks/slack
  handler: onSlackCommand
  nats_publish:
    - stream: events
      subject: events.slack
```

Required: `path`, `handler`.

### `type: websocket`
Upgrades HTTP to WebSocket via `gofiber/contrib/websocket`.

```yaml
- type: websocket
  path: /ws/chat
  handler: onChat
  auth: true
```

Required: `path`, `handler`.

### `type: sse`
Server-Sent Events streaming endpoint.

```yaml
- type: sse
  path: /events/stream
  handler: onStream
  auth: true
```

Required: `path`, `handler`.

### `type: file`
File upload/download with middleware validation.

```yaml
- type: file
  method: POST
  path: /files/upload
  handler: onFileUpload
  allowed_types:
    - image/png
    - application/pdf
  max_size: 10MB
  storage:
    mode: local          # s3 | local
    path: /data/uploads
```

| Field | Description |
|-------|-------------|
| `allowed_types` | Content-Type whitelist. Supports `type/*` wildcard. Returns 415 on mismatch |
| `max_size` | Max body size. Supports `KB`, `MB`, `GB` suffixes. Returns 413 if exceeded |
| `storage.mode` | `s3` (minio-compatible: AWS S3, R2, MinIO) or `local` (filesystem) |
| `storage.bucket` | S3 bucket name (required for `mode: s3`) |
| `storage.path` | Filesystem path (required for `mode: local`) |

Required: `method`, `path`, `handler`, `storage`.

## Exit (NATS workers)

Each exit worker subscribes to a NATS JetStream subject and processes messages:

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
    db: pg-main
    reply: false
    reply_timeout: 30s
    pull_batch: 0
    consumer_mode: push
```

| Field | Description |
|-------|-------------|
| `name` | Worker name (must be unique) |
| `subscribe.stream` | NATS stream to consume from |
| `subscribe.subject` | Subject within stream. Defaults to stream name |
| `subscribe.durable` | Durable name. Defaults to `{name}-worker` |
| `handler` | Go function name registered via `svc.WithExit()` |
| `max_concurrent` | Max concurrent goroutines (default: 1) |
| `db` | Optional database reference for worker access |
| `reply` | If `true`, handler response is sent via `msg.Respond()` |
| `reply_timeout` | Timeout for reply (default: `30s`) |
| `pull_batch` | If `> 0`, uses pull consumer fetching this many messages per batch |
| `consumer_mode` | `push` (default) or `pull`. Overridden by `pull_batch` |

## Cron

```yaml
cron:
  - name: daily-report
    schedule: "0 6 * * *"
    mode: nats
    publish:
      stream: cron
      subject: cron.daily-report
```

| Mode | Behavior | Requires |
|------|----------|----------|
| `nats` | Publishes to NATS on schedule | `publish.stream` |
| `handler` | Calls Go function directly | `handler` |
| `internal` | System tick (logs only) | Nothing |

Schedule format: standard 5-field cron (`min hour dom month dow`) or `@every 1s`, `@every 1h`, etc.

## Server

```yaml
server:
  host: "0.0.0.0"
  api_prefix: /api/v1
  cors:
    origins: ["*"]
  static:
    - prefix: /static
      dir: ./public
  middleware:
    - path: "/api/v1/*"
      apply:
        - logger
        - breaker
        - cors
  openapi:
    enabled: true
    theme: moon
```

### Middleware

Available middleware names for `apply:`:

| Name | Description |
|------|-------------|
| `logger` | Request logging (method, path, status, duration) |
| `shedding` | Adaptive load shedding |
| `breaker` | Circuit breaker (per route) |
| `maxconns` | Concurrency limiter |
| `maxbytes` | Request body size limiter |
| `gunzip` | Auto-decompress gzip bodies |
| `prometheus` | In-process metrics collector |
| `cors` | CORS headers |
| `trace` | OpenTelemetry tracing |
| `jwt` | JWT auth (configured separately) |
| `content_security` | RSA body signature verification |
| `cryption` | AES-CFB body decryption |

When `middleware:` is NOT specified, all middlewares apply globally (backwards compatible). When specified, only `recover` + `health` are global; rest are per-path.

### OpenAPI

Enables auto-generated API documentation:

```yaml
server:
  openapi:
    enabled: true
    version: "1.0.0"
    spec_path: /openapi.json
    docs_path: /docs
    theme: moon
    dark_mode: true
```

- `GET /openapi.json` — OpenAPI 3.0.3 spec (auto-generated from entry definitions and registered models)
- `GET /docs` — Scalar UI documentation browser
- Models must be registered via `svc.RegisterModel("Product", (*Product)(nil))` for schema generation

## Environment variable interpolation

All `url` fields and any string in the YAML support `${VAR}` interpolation:

```yaml
url: "${DATABASE_URL}"
url: "nats://${NATS_HOST}:4222"
```

Variables are expanded at load time from the process environment. No file-based `.env` loading.

## Pool auto-sizing

When `pool.max_conns` is `<= 0` or not specified, the pool size is auto-calculated:

```
max(1, (PG_SERVER_MAX_CONNS - RESERVED_CONNS) / REPLICA_COUNT)
```

| Env var | Default |
|---------|---------|
| `PG_SERVER_MAX_CONNS` | `100` |
| `REPLICA_COUNT` | `1` |

`RESERVED_CONNS` comes from `pool.reserved_conns` (default: `10`).
