# Configuration

sdk-api is **YAML-driven**. Everything is declared in a single `service.yaml` — no code scaffolding for config, no magic. The YAML defines databases, NATS connections, entry endpoints (HTTP), exit workers (NATS consumers), cron jobs, server settings, and security features.

## Full Schema

```yaml
name: my-service              # Service name
port: 8080                    # HTTP port (env override: $PORT)

# ---- Log configuration (optional) ----
log:
  level: info                  # debug | info | error | severe
  encoding: json               # json | plain (use plain for development)

# ---- Deploy target (optional, CLI validation) ----
# deploy:
#   target: auto               # auto | vercel | docker | kube | bare-metal

# ---- Databases (multiple, by driver) ----
databases:
  - name: pg-main             # Reference name used by entry: db:
    driver: postgres          # postgres | turso | mysql
    url: "${DATABASE_URL}"    # Connection string (env interpolation)
    pool:
      max_conns: 10
      min_conns: 2
      max_conn_lifetime: 30m
      max_conn_idle_time: 5m
      health_check_period: 1m
      reserved_conns: 10
    slow_query:
      enabled: true
      threshold: 200ms               # log queries slower than this

# ---- Key-Value stores (Redis / Dragonfly) ----
kv:
  - name: cache-main
    driver: redis
    url: "${REDIS_URL}"

# ---- Stream connections (NATS + Kafka) ----
stream:
  - name: default
    driver: nats
    url: "${NATS_URL}"
    streams:
      - name: orders
        max_age: 24h
        max_bytes: 1073741824

  - name: analytics
    driver: kafka
    brokers: ["localhost:9092"]
    consumer_group: sdk-api

# ---- Entry endpoints (HTTP) ----
entry:
  # CRUD — auto-generates List/Get/Create/Update/Delete
  - type: crud
    model: Product
    db: pg-main
    table: products
    path: /products
    event_stream: default
    event_publish:
      - stream: orders
        subject: order.created

  # REST — single endpoint, no auto-generation
  - type: rest
    method: GET
    path: /products/:id/transform
    handler: onTransformProduct
    auth_modes: [jwt]
    event_publish:
      - stream: orders
        subject: orders.transformed

  # Webhook
  - type: webhook
    path: /webhooks/sendgrid
    handler: onInboundEmail
    event_publish:
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
    max_files: 5
    magic_bytes: true
    storage:
      mode: s3
      bucket: uploads
      endpoint: "http://minio:9000"
      access_key: "${MINIO_ACCESS_KEY}"
      secret_key: "${MINIO_SECRET_KEY}"

  # Async job — 202 Accepted + polling
  - type: async
    path: /jobs/reports
    handler: processReport

  # GraphQL — auto-generated schema + resolvers
  - type: graphql
    path: /graphql

# ---- Exit workers ----
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
    db: pg-main
    reply: false
    consumer_mode: push

# ---- Cron (scheduled jobs) ----
cron:
  - name: daily-report
    schedule: "0 6 * * *"
    mode: nats
    publish:
      stream: cron
      subject: cron.daily-report

# ---- Server config ----
server:
  host: "0.0.0.0"
  prefork: false
  body_limit: 4194304
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
      - "https://app.example.com"   # Never "*" in production
    methods:
      - GET
      - POST
      - PUT
      - PATCH
      - DELETE
    credentials: true
    max_age: 86400

  # Security headers
  security_headers:
    frame_options: DENY
    referrer_policy: strict-origin-when-cross-origin
    permissions_policy: "camera=(), microphone=()"
    hsts: true
    hsts_max_age: 31536000
    csp: "default-src 'self'; script-src 'self'"
    csp_report_path: /csp-violation

  # CSRF protection
  csrf:
    enabled: true
    cookie_name: csrf_token
    header_name: X-CSRF-Token
    same_site: Strict
    exclude_paths:
      - /webhooks/*

  # Rate limiting
  rate_limit:
    enabled: true
    kv: cache-main                # references kv[].name (optional)
    algorithm: sliding_window     # sliding_window | token_bucket
    global:
      requests_per_second: 1000
      burst: 2000
    per_ip:
      requests_per_second: 200
      burst: 300
    per_user:
      requests_per_second: 50
      burst: 100
    per_key:
      requests_per_second: 200
      burst: 500

  # TLS
  tls:
    enabled: true
    manual:
      cert_file: /etc/certs/cert.pem
      key_file: /etc/certs/key.pem
    min_version: "1.2"
    max_version: "1.3"
    redirect_http: true

  # SSRF protection (disabled by default)
  ssrf:
    enabled: false
    block_private: true
    block_loopback: true
    block_metadata: true

  # Authentication & Authorization
  auth:
    enabled: true                         # Enable auth system
    driver: openfga-zitadel                # none | manual | openfga-zitadel | ory
    secret: "${JWT_SECRET}"               # HMAC shared secret
    prev_secret: "${OLD_JWT_SECRET}"      # Previous secret for key rotation (optional)
    algorithm: HS256                       # HS256 | HS384 | HS512 | RS256
    expiry: 900                            # JWT TTL in seconds (default 900 = 15min)
    context_key: claims                    # fiber.Ctx.Locals key
    issuer: "sdk-api"                      # Validate iss claim
    audience: "api.example.com"            # Validate aud claim
    openfga_url: "http://localhost:18080"  # OpenFGA HTTP API
    openfga_store: "default"               # OpenFGA store ID
    zitadel_url: "https://auth.tld"        # Zitadel issuer (OIDC)
    kratos_url: "http://localhost:4433"    # Ory Kratos public URL
    keto_url: "http://localhost:4466"      # Ory Keto URL

Token blacklist: use `svc.WithJWTBlacklist()` at runtime to register a callback. See `docs/runtime.md` for API details.
    cookie:                                # Cookie settings for JWT tokens
      access_token_name: token             # Cookie name for access token (default: token)
      refresh_token_name: refresh_token    # Cookie name for refresh token (default: refresh_token)
      domain: ".example.com"               # Cookie domain (optional)
      path: "/"                            # Cookie path (default: /)
      http_only: true                      # HttpOnly flag (default: true)
      secure: false                        # Secure flag (default: true)
      same_site: Lax                       # SameSite: Strict | Lax | None (default: Strict)
    refresh:                               # Auto-wired token refresh
      enabled: true                        # Enable auto-wired POST /auth/refresh
      ttl: 604800                          # Refresh TTL in seconds (default 604800 = 7 days)
      endpoint: /auth/refresh              # Custom endpoint path (default: /auth/refresh)
      secret: "${REFRESH_SECRET}"          # Separate secret for refresh (falls back to auth.secret)

  # Security extensions (opt-in)
  security:
    content_security:
      enabled: false
      public_key: /etc/secrets/rsa.pub
    cryption:
      enabled: false
      key: "${AES_KEY}"
    encrypt_cookie:
      enabled: false
      key: "${COOKIE_ENCRYPT_KEY}"       # base64-encoded 32-byte key (AES-256-GCM)
      except:                             # cookie names to skip encryption
        - csrf_token

  # Global cookie settings
  cookies:
    same_site: Lax
    secure: true

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
| `databases` | array | `[]` | Database connections |
| `kv` | array | `[]` | Key-value store connections (Redis / Dragonfly) |
| `stream` | array | `[]` | Stream connections (NATS or Kafka) |
| `entry` | array | `[]` | HTTP endpoint definitions |
| `exit` | array | `[]` | Worker definitions |
| `cron` | array | `[]` | Scheduled job definitions |
| `auth` | object | `nil` | Authentication configuration |
| `deploy` | object | `nil` | Deployment target for platform-specific validation |
| `server` | object | (defaults) | HTTP server + security configuration |

## Databases

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 10
      min_conns: 2
  - name: mongo-main
    driver: mongo
    url: "${MONGO_URI}"
    database: mydb
    pool:
      max_conns: 100
  - name: local-turso
    driver: turso
    url: "${DATABASE_URL}"
    pool:
      max_conns: 500
    turso:
      mode: local
      busy_timeout: 30000
```

| Field | Description |
|-------|-------------|
| `name` | Reference name. Used by `entry[].db` and `exit[].db` |
| `driver` | `postgres` (or `pg`), `turso`, `mysql`, `mongo` |
| `url` | Connection string. Supports `${VAR}` env interpolation |
| `database` | Database name (required for `mongo` driver) |
| `pool.max_conns` | Max open connections (Postgres: pgx pool, Turso/MySQL: `SetMaxOpenConns`, Mongo: `maxPoolSize` query param) |
| `pool.min_conns` | Min idle connections (**Mongo**: `maxConnecting` query param) |
| `turso.mode` | `local` (apply PRAGMA busy_timeout) or `remote` (skip PRAGMAs for Turso Cloud) |
| `turso.busy_timeout` | Busy timeout in ms (default: 30000). Only used when `mode: local` |

## KV Stores (Redis / Dragonfly)

```yaml
kv:
  - name: cache-main
    driver: redis
    url: "${REDIS_URL}"
```

| Field | Description |
|-------|-------------|
| `name` | Reference name. Used by `server.rate_limit.kv`, `entry[].cache` |
| `driver` | `redis` (default) |
| `url` | Redis/Dragonfly connection URL |

KV connections are created lazily and shared by all references to the same name. Multiple names pointing to the same URL reuse a single connection. Compatible with Dragonfly, Redis, and any Redis-protocol server.

## Stream Connections (NATS + Kafka)

```yaml
stream:
  - name: default
    driver: nats
    url: "${NATS_URL}"
    streams:
      - name: orders

  - name: analytics
    driver: kafka
    brokers: ["localhost:9092"]
    consumer_group: sdk-api
```

| Field | Description |
|-------|-------------|
| `name` | Reference name. Used by `entry[].event_stream` |
| `driver` | `nats` or `kafka` |
| `url` | NATS URL (required for `driver: nats`) |
| `brokers` | Kafka broker list (required for `driver: kafka`) |
| `consumer_group` | Kafka consumer group (defaults to `{name}-group`) |

Default subjects for a NATS stream named `orders`: `[orders, orders.>]`.

## Log

```yaml
log:
  level: info                   # debug | info | error | severe
  encoding: json                # json | plain
  mode: console                 # console | file | volume
  path: /var/log/my-service     # log directory (mode=file only)
  keep_days: 30                 # delete logs older than N days
  max_size: 100                 # MB, rotation by size
  rotation: daily               # daily | size
  compress: false               # gzip rotated files
```

| Field | Default | Description |
|-------|---------|-------------|
| `level` | `info` | Minimum log level: `debug`, `info`, `error`, `severe` |
| `encoding` | `json` | Output format: `json` or `plain` (use `plain` for local development) |
| `mode` | `console` | Output destination: `console`, `file`, `volume` |
| `path` | `logs` | Directory for log files (only when `mode: file`) |
| `keep_days` | `0` | Delete log files older than N days (`0` = keep all) |
| `max_size` | `0` | Max MB per log file before rotation (`0` = unlimited) |
| `rotation` | `daily` | Rotation strategy: `daily` or `size` |
| `compress` | `false` | Gzip rotated log files |

All settings are optional. Defaults are shown above and match sensible values for most deployments. Setting `level: error` in production silences `info` and `debug` messages, reducing noise.

## Entry (HTTP endpoints)

9 types of entry endpoints:

### `type: crud`
Auto-generates 5 endpoints for a model.

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/resource` | List (paginated, filterable, sortable) |
| GET | `/resource/:id` | Get by primary key |
| POST | `/resource` | Create (with entry hooks) |
| PATCH | `/resource/:id` | Partial update |
| DELETE | `/resource/:id` | Delete |

```yaml
- type: crud
  model: Product
  db: pg-main
  table: products
  event_stream: default                # Optional: broker for event_publish
  event_publish:
    - stream: orders
      subject: order.created
  overrides:
    list: ~
    get: "onCustomGet"
    create: "-"
```

| Field | Description |
|-------|-------------|
| `model` | Model name (registered via CRUDProvider) |
| `db` | Database reference |
| `table` | Table name. Defaults to snake_case of model |
| `event_stream` | Broker name for event publishing |
| `overrides` | CRUD override controls |
| `pagination` | Pagination mode: `"offset"` (default) or `"keyset"`. Offset uses `LIMIT/OFFSET` + `COUNT(*)`. Keyset uses `WHERE pk > $1 LIMIT N` — O(log N), no `COUNT(*)`, returns `nextCursor` |
| `page_size` | Default page size and range minimum. Default `10` |
| `max_page_size` | Maximum allowed page size. Range maximum. Default `100` |
| `sortable` | Allowed sort columns. Empty = all columns allowed. E.g. `[id, name, price]` |

### `type: rest`
Single endpoint with any HTTP method.

```yaml
- type: rest
  method: GET
  path: /products/:id/transform
  handler: onTransformProduct
  auth_modes: [jwt]
  event_publish:
    - stream: orders
      subject: orders.transformed
```

### `type: webhook`
Defaults to POST if omitted. No JWT validation by default.

```yaml
- type: webhook
  path: /webhooks/slack
  handler: onSlackCommand
  event_publish:
    - stream: events
      subject: events.slack
```

### `type: websocket`
Upgrades HTTP to WebSocket.

```yaml
- type: websocket
  path: /ws/chat
  handler: onChat
```

### `type: sse`
Server-Sent Events streaming endpoint.

```yaml
- type: sse
  path: /events/stream
  handler: onStream
```

### `type: file`
File upload/download with middleware validation.

```yaml
- type: file
  method: POST
  path: /files/upload
  handler: onFileUpload
  allowed_types:
    - image/png
    - image/jpeg
  max_size: 10MB
  max_files: 5
  magic_bytes: true
  storage:
    mode: local
    path: /data/uploads

S3 with presigned URLs, HTTP pool, and L1+L2 cache:

```yaml
- type: file
  method: POST
  path: /files/upload
  handler: onFileUpload
  storage:
    mode: s3
    bucket: uploads
    endpoint: http://minio:9000
    access_key: "${ACCESS_KEY}"
    secret_key: "${SECRET_KEY}"
    presign: true
    presign_ttl: 5m
    pool:
      max_idle_conns: 200
      max_idle_conns_per_host: 100
      max_conns_per_host: 250
      idle_timeout: 90s
    cache:
      l1: ram
      l1_ttl: 5m
      l1_size: 10000
      l2: disk
      l2_path: /data/cache
```

| Field | Description |
|-------|-------------|
| `allowed_types` | Content-Type whitelist. Returns 415 on mismatch |
| `max_size` | Max body size. Supports `KB`, `MB`, `GB` suffixes |
| `max_files` | Max files per multipart request |
| `magic_bytes` | Verify file content matches declared type (body > 512 bytes) |
| `storage.mode` | Storage driver: `s3` or `local` |
| `storage.bucket` | S3 bucket name |
| `storage.endpoint` | S3 endpoint URL (e.g. `http://minio:9000` or `https://s3.amazonaws.com`) |
| `storage.region` | S3 region. Default `us-east-1` |
| `storage.access_key` | S3 access key |
| `storage.secret_key` | S3 secret key |
| `storage.path` | Local filesystem path (when `mode: local`) |
| `storage.presign` | Enable presigned URL generation via `Presigner` interface. Default `false` |
| `storage.presign_ttl` | Presigned URL TTL duration. Default `5m` |
| `storage.pool.max_idle_conns` | Max idle HTTP connections for S3 client. Default `200` |
| `storage.pool.max_idle_conns_per_host` | Max idle connections per S3 host. Default `100` |
| `storage.pool.max_conns_per_host` | Max total connections per S3 host. Default `250` |
| `storage.pool.idle_timeout` | Idle connection timeout. Default `90s` |
| `storage.cache.l1` | L1 cache type: `ram` or `none`. L1 and L2 are independent |
| `storage.cache.l1_ttl` | L1 cache TTL. Default `5m` |
| `storage.cache.l1_size` | L1 cache max entries. Default `10000` |
| `storage.cache.l2` | L2 cache type: `disk` or `none` |
| `storage.cache.l2_path` | L2 disk cache directory (required when `l2: disk`) |

### `type: async`
Async job with 202 Accepted + status polling.

```yaml
- type: async
  path: /jobs/reports
  handler: processReport
```

| Method | Path | Behavior |
|--------|------|----------|
| POST | `/path` | Submit job → 202 + `job_id` + `status_url` |
| GET | `/path/:job_id` | Poll job status → JSON |

Handler signature: `func(body []byte, job *JobState) error`. Set `job.Result` for status response.

### `type: graphql`
Auto-generated GraphQL schema from registered models.

```yaml
- type: graphql
  path: /graphql
```

Queries and mutations are auto-generated from `CRUDProvider` registrations. Models must be registered via `svc.RegisterModel()`.

### Common entry fields

| Field | Applies to | Description |
|-------|-----------|-------------|
| `auth_modes` | crud, rest, webhook, file | Authentication modes (`jwt`, `apikey`, or both) |
| `roles` | crud, rest, webhook | Required roles (validated by auth driver) |
| `permissions` | crud, rest, webhook | Required permissions (validated by auth driver) |
| `jwt_from` | crud, rest, webhook, file | JWT source: `"header:Authorization"`, `"cookie:token"`, `"query:token"` (default `header:Authorization`) |
| `api_key_prefix` | crud, rest, webhook, file | API key prefix (e.g. `"sk-"`, only when `auth_modes` includes `apikey`) |
| `csrf` | crud, rest, webhook, file | CSRF protection override (`true`/`false`) |
| `rate_limit` | crud, rest, webhook | Per-entry pre-auth rate limit |
| `rate_limit_per_user` | crud, rest, webhook | Post-auth per-user rate limit |
| `rate_limit_per_key` | crud, rest, webhook | Post-auth per-key rate limit |
| `rate_limit_per_role` | crud, rest, webhook | Post-auth per-role rate limit (maps role → limit) |
| `cache` | crud | KV store name for CRUD cache layer (references `kv[]`) |
| `event_stream` | crud, rest, webhook, file | Stream broker name for publishes |
| `event_publish` | crud, rest, webhook, file | Publish targets for event streams |
| `timeout` | crud, rest, webhook, file | Per-entry request deadline (e.g. `30s`) |
| `tenant_scope` | crud | JWT claim for tenant ID (e.g. `org_id`). Defaults to `org_id` |
| `tenant_field` | crud | DB column for tenant filter (e.g. `tenant_id`). Enables automatic multi-tenant scoping |
| `requires_mfa` | crud, rest | Require `mfa: true` in JWT claims. Rejects with 401 if missing or false |
| `csrf` | crud, rest, webhook | Per-entry override: `false` skips CSRF for this entry |
| `validate` | crud, rest, webhook | Validation model name |

**Entry auth combinations:**

| `auth_modes` | `roles` / `permissions` | What the middleware does |
|--------------|------------------------|-------------------------|
| (empty) | — | No auth, public endpoint |
| `[jwt]` | empty | Validates JWT signature + claims (identity only) |
| `[jwt]` | defined | Validates JWT + verifies roles/permissions via driver |
| `[apikey]` | — | API key validation via driver |
| `[jwt, apikey]` | — | Both (router detects format) |

### event_publish targets

```yaml
event_publish:
  - stream: orders                # Stream/topic name
    subject: order.created        # Subject (defaults to stream)
    event_stream: default         # Optional: broker name override
```

When `event_stream` is specified per-target, the message is published only to that broker.
When omitted, the message is published to all available brokers.

## Exit (workers)

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
    db: pg-main
    event_stream: default
```

| Field | Description |
|-------|-------------|
| `event_stream` | Broker name to consume from (nats or kafka) |

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

| Mode | Behavior |
|------|----------|
| `nats` | Publishes to event stream on schedule |
| `handler` | Calls Go function directly |
| `internal` | System tick (logs only) |

## Server

### Databases

Each entry in `databases:` supports the following fields:

| Field | Default | Description |
|-------|---------|-------------|
| `name` | — | Unique reference name used by `entry[].db` |
| `driver` | `postgres` | One of: `postgres`, `mysql`, `turso` |
| `url` | — | Connection string |
| `pool.max_conns` | `10` | Maximum connections in the pool |
| `pool.min_conns` | `2` | Minimum idle connections |
| `pool.max_conn_lifetime` | `30m` | Maximum connection age |
| `slow_query.enabled` | `false` | Enable slow query logging |
| `slow_query.threshold` | `100ms` | Queries slower than this are logged |

Slow query logging writes a structured log entry for any database query that exceeds the configured threshold. Threshold is a duration string like `"200ms"` or `"1s"`.

### Server options

| Field | Default | Description |
|-------|---------|-------------|
| `host` | `0.0.0.0` | Bind address |
| `prefork` | `false` | Fiber prefork (SO_REUSEPORT) |
| `body_limit` | `4194304` | Max body size (Fiber level) |
| `timeout` | `30s` | Read/write/idle timeout |
| `api_prefix` | `/api/v1` | Prefix prepended to all entry paths |
| `recover_stack` | `true` | Show stack traces on panic |
| `logger` | `true` | Enable request logging middleware |
| `load_shedding` | `true` | Enable adaptive load shedding |
| `breaker` | `true` | Enable circuit breaker per route |

### KV Store connections

Declarative Redis/Dragonfly connections are defined in the top-level `kv:` section:

```yaml
kv:
  - name: cache-main
    driver: redis
    url: "${REDIS_URL}"
```

The `kv:` section supports any Redis-protocol server including Dragonfly and Valkey. Multiple `kv` names with the same URL share a single connection. KV stores auto-deduplicate by URL to avoid unnecessary connections.

For programmatic access, use `svc.KV(name)` which returns a `*redis.Redis` client. The connection is created lazily on first use, making it safe for prefork child processes.

```go
rdb := svc.KV("cache-main")
```

KV connections use the URL format. The SDK wraps go-redis internally — Redis node, cluster, and sentinel modes are detected from the URL scheme and path:

```yaml
kv:
  - name: cache-local
    driver: redis
    url: "redis://localhost:6379/0"

  - name: cache-cluster
    driver: redis
    url: "redis://user:pass@cluster-host:6379/0"

  - name: cache-sentinel
    driver: redis
    url: "redis://sentinel1:26379/0"
```

### CORS

```yaml
server:
  cors:
    origins:
      - "https://app.example.com"
    methods:
      - GET
      - POST
    credentials: true
    max_age: 86400
```

When `cors` section is omitted, CORS defaults to same-origin only (secure default).
Never use `"*"` in production when `credentials: true`.

### Security Headers

Global middleware that injects security headers into every response.

```yaml
server:
  security_headers:
    frame_options: DENY                        # X-Frame-Options
    referrer_policy: strict-origin-when-cross-origin
    permissions_policy: "camera=(), microphone=()"
    hsts: true                                 # HTTP Strict Transport Security
    hsts_max_age: 31536000
    hsts_include_subdomains: true
    csp: "default-src 'self'"                  # Content-Security-Policy
    csp_report_path: /csp-violation            # Auto-registers POST endpoint
    coop: same-origin                          # Cross-Origin-Opener-Policy
    coep: require-corp                         # Cross-Origin-Embedder-Policy
    corp: same-origin                          # Cross-Origin-Resource-Policy
    cache_control: "no-store, no-cache"
```

`X-Content-Type-Options: nosniff` is always set.
When `csp_report_path` is configured, a `POST /{path}` endpoint is auto-registered to log CSP violation reports.

### CSRF

```yaml
server:
  csrf:
    enabled: true
    cookie_name: csrf_token
    header_name: X-CSRF-Token
    same_site: Strict
    secure: true
    json_check: true              # Skip CSRF for JSON Content-Type (safe via SOP)
    exclude_paths:
      - /webhooks/*
```

Double-submit cookie pattern. Token generated on GET responses, validated on POST/PUT/PATCH/DELETE.
When `json_check: true`, requests with `Content-Type: application/json` skip CSRF validation (browser Same-Origin Policy protects JSON).
Per-entry override: `entry[].csrf: false`.

### Rate Limit

```yaml
server:
  rate_limit:
    enabled: true
    kv: cache-main                    # references kv[].name (optional, for shared counters)
    algorithm: sliding_window         # sliding_window | token_bucket
    global:
      requests_per_second: 1000
      burst: 2000
    per_ip:
      requests_per_second: 200
      burst: 300
    per_user:
      requests_per_second: 50        # post-auth, per authenticated user
      burst: 100
    per_key:
      requests_per_second: 200       # post-auth, per API key
      burst: 500
    skip_failed_requests: false       # don't count 4xx/5xx responses
    skip_successful_requests: false   # don't count 2xx/3xx responses
```

Two algorithms are available: `sliding_window` (default) and `token_bucket`. Sliding window provides smooth rate enforcement without the boundary reset of fixed windows. Token bucket supports bursts up to the configured `burst` value.

When `kv:` is set to a named KV store, rate limit counters are shared across all processes (prefork children, multiple instances). Without `kv:`, each process has independent counters (in-memory).

Post-auth limits (`per_user`, `per_key`) are enforced after JWT or API key validation. They require the `entry` to declare `auth_modes: [jwt]` or `auth_modes: [apikey]`.

Rate-limited requests receive `429 Too Many Requests` with `Retry-After` and `X-RateLimit-*` headers.

Per-entry overrides: `entry[].rate_limit` (pre-auth), `entry[].rate_limit_per_user` (post-auth per-user), `entry[].rate_limit_per_key` (post-auth per-key), `entry[].rate_limit_per_role` (post-auth per-role).

### TLS

```yaml
server:
  tls:
    enabled: true
    manual:
      cert_file: /etc/certs/cert.pem
      key_file: /etc/certs/key.pem
    autocert:                           # Alternative to manual
      domains:
        - api.example.com
      email: admin@example.com
    min_version: "1.2"
    max_version: "1.3"
    curve_preferences:
      - X25519
      - P-256
    cipher_suites:
      - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    redirect_http: true
    redirect_port: 80
```

Three modes:
- **Off** (no `tls:` section) — HTTP only
- **Manual** — `cert_file` + `key_file`
- **Autocert** — Let's Encrypt automatic certificates

When `redirect_http: true`, a separate goroutine listens on `redirect_port` (default 80) and issues 308 redirects to HTTPS.

### Telemetry

```yaml
server:
  telemetry:
    enabled: true                          # Enable OpenTelemetry tracing
    name: my-service                        # Service name for spans
    endpoint: localhost:4317                # OTLP receiver address
    sampler: 0.1                            # Sampling rate (0.0 - 1.0)
    batcher: otlpgrpc                       # Exporter: otlpgrpc | otlphttp | zipkin | file
    otlp_headers:
      x-api-key: "${OTLP_API_KEY}"          # Headers for OTLP export
    trace_response_header: X-Trace-Id       # Response header with trace ID
    skip_paths: [/health, /metrics]         # Paths excluded from tracing
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable OpenTelemetry tracing |
| `name` | — | Service name (appears as `service.name` in spans) |
| `endpoint` | — | OTLP receiver address (e.g. `localhost:4317` for gRPC, `localhost:4318` for HTTP) |
| `sampler` | `1.0` | Sampling rate: `1.0` = all traces, `0.1` = 10% |
| `batcher` | `otlpgrpc` | Exporter type: `otlpgrpc`, `otlphttp`, `zipkin`, `file` |
| `otlp_headers` | — | Additional headers sent with OTLP export requests |
| `otlp_http_path` | — | URL path for OTLP HTTP transport (e.g. `/v1/traces`) |
| `otlp_http_secure` | `false` | Enable TLS for OTLP HTTP transport |
| `trace_response_header` | — | Response header exposing trace ID (e.g. `X-Trace-Id`) |
| `skip_paths` | — | Paths excluded from tracing (health checks, metrics) |

Disabled by default. When enabled, each HTTP request creates an OpenTelemetry span with trace context propagated via W3C `traceparent` headers. If a `trace_response_header` is configured, the trace ID is also exposed in a dedicated response header. Spans are sent to the configured OTLP receiver.

Logs from `logx.WithContext(ctx)` automatically include `trace` and `span` fields when tracing is active.

### SSRF Protection

```yaml
server:
  ssrf:
    enabled: true                      # Disabled by default
    block_private: true                # 10.x, 172.16-31.x, 192.168.x
    block_loopback: true               # 127.0.0.1, ::1
    block_metadata: true               # 169.254.169.254 (cloud metadata)
    allowed_hosts:
      - api.stripe.com
```

Disabled by default to avoid breaking external HTTP calls. When enabled, access `svc.SafeHTTPClient()` to make protected HTTP requests.

### Cookie Settings

```yaml
server:
  cookies:
    same_site: Lax
    secure: true
```

Global defaults for SameSite and Secure flags. Applied to CSRF tokens and can be applied to other cookies.

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

### OpenAPI

```yaml
server:
  openapi:
    enabled: true
    version: "1.0.0"
    spec_path: /openapi.json
    docs_path: /docs
    theme: moon
```

## Environment variable interpolation

### Basic `${VAR}`

```yaml
url: "${DATABASE_URL}"
```

### With default value

```yaml
url: "${DATABASE_URL:postgres://localhost:5432/mydb}"
max_conns: "${DB_MAX_CONNS:10}"
```

If the environment variable is not set, the default value after the colon is used.
If no default is provided and the variable is not set, a warning is logged.

## CRUD Override values

| YAML value | Behavior |
|------------|----------|
| `""` or `~` | Use default auto-generated handler |
| `"-"` | Do not register this endpoint |
| `"handlerName"` | Use custom handler from Rest map |

## Pool auto-sizing

```
max(1, (PG_SERVER_MAX_CONNS - RESERVED_CONNS) / REPLICA_COUNT)
```

| Env var | Default |
|---------|---------|
| `PG_SERVER_MAX_CONNS` | `100` |
| `REPLICA_COUNT` | `1` |
