# HTTP Server

The HTTP server wraps **Fiber v3** (fasthttp) with built-in middlewares, per-route middleware configuration, security headers, CSRF, rate limiting, TLS support, and auto-generated OpenAPI documentation.

## Configuration

```yaml
server:
  host: "0.0.0.0"
  prefork: false
  body_limit: 4194304       # 4 MB
  timeout: 30s
  max_conns: 1000
  max_bytes: 4194304
  api_prefix: /api/v1
  health_path: /health
  metrics_path: /metrics
  shutdown_timeout: 10s
  recover_stack: true       # Show stack traces on panic
```

| Field | Default | Description |
|-------|---------|-------------|
| `host` | `0.0.0.0` | Bind address |
| `prefork` | `false` | Fiber prefork (SO_REUSEPORT, Linux only) |
| `body_limit` | `4194304` | Max body size (Fiber level) |
| `timeout` | `30s` | Read/write/idle timeout |
| `max_conns` | `1000` | Concurrency limit via middleware |
| `max_bytes` | `4194304` | Per-request max bytes |
| `api_prefix` | `/api/v1` | Prefix prepended to all entry paths |
| `health_path` | `/health` | Kubernetes liveness probe endpoint |
| `metrics_path` | `/metrics` | Prometheus metrics endpoint |
| `shutdown_timeout` | `10s` | Graceful shutdown wait time |
| `recover_stack` | `true` | When true, stack traces appear in 500 responses (dev). Error messages are sanitized for internal errors. |

## Entry Types

The server auto-registers routes from `entry:` in YAML. Nine types:

| Type | HTTP Methods | What you write | Description |
|------|-------------|----------------|-------------|
| `crud` | GET, POST, PATCH, DELETE | Nothing | Auto-generated CRUD |
| `rest` | Any | Single handler | Custom handler, any method |
| `webhook` | Any (default POST) | Single handler | Webhook receiver |
| `websocket` | GET (upgrade) | WS handler | WebSocket upgrade |
| `sse` | GET | SSE handler | SSE streaming |
| `file` | GET, POST, PUT, PATCH, DELETE | Upload handler | File upload/download/delete |
| `async` | POST, GET | Async handler | 202 Accepted + status polling |
| `graphql` | POST | Nothing | Auto-generated GraphQL schema |

## Security Headers

Global middleware that injects security headers into every response. Configured via `server.security_headers`:

```yaml
server:
  security_headers:
    frame_options: DENY
    referrer_policy: strict-origin-when-cross-origin
    permissions_policy: "camera=(), microphone=()"
    hsts: true
    hsts_max_age: 31536000
    hsts_include_subdomains: true
    csp: "default-src 'self'"
    csp_report_path: /csp-violation
    coop: same-origin
    coep: require-corp
    corp: same-origin
    cache_control: "no-store"
```

| Header | Config field | Always set? |
|--------|-------------|-------------|
| `X-Content-Type-Options: nosniff` | — | ✅ Always |
| `X-Frame-Options` | `frame_options` | Only if configured |
| `Referrer-Policy` | `referrer_policy` | Only if configured |
| `Permissions-Policy` | `permissions_policy` | Only if configured |
| `Strict-Transport-Security` | `hsts` | Only if configured |
| `Content-Security-Policy` | `csp` | Only if configured |
| `Cross-Origin-Opener-Policy` | `coop` | Only if configured |
| `Cross-Origin-Embedder-Policy` | `coep` | Only if configured |
| `Cross-Origin-Resource-Policy` | `corp` | Only if configured |
| `Cache-Control` | `cache_control` | Only if configured |

**CSP Report Endpoint:** When `csp_report_path` is set (e.g. `/csp-violation`), a `POST` endpoint is auto-registered that logs violation reports via `logx.Errorf`.

## CSRF Protection

Double-submit cookie pattern. Configured via `server.csrf`:

```yaml
server:
  csrf:
    enabled: true
    cookie_name: csrf_token
    header_name: X-CSRF-Token
    same_site: Strict
    secure: true
    exclude_paths:
      - /webhooks/*
```

- Token generated on `GET`/`HEAD`/`OPTIONS` responses, set as non-HttpOnly cookie
- Validated on `POST`/`PUT`/`PATCH`/`DELETE` by comparing cookie vs header
- Returns `403` on mismatch
- Per-entry override: `entry[].csrf: false`

## Rate Limiting

Configured via `server.rate_limit`:

```yaml
server:
  rate_limit:
    enabled: true
    kv: cache-main            # references kv[].name
    global:
      requests_per_second: 1000
      burst: 2000
    per_ip:
      requests_per_second: 200
      burst: 300
    per_user:
      requests_per_second: 100
      burst: 150
```

- Uses token bucket algorithm
- `kv` references a named KV store from `kv[]` config
- Returns `429 Too Many Requests` + `Retry-After` header
- Returns `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers
- Per-entry override: `entry[].rate_limit`

## TLS

Configured via `server.tls`:

```yaml
server:
  tls:
    enabled: true
    manual:
      cert_file: /etc/certs/cert.pem
      key_file: /etc/certs/key.pem
    autocert:
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
- **Manual** — Loads cert/key files
- **Autocert** — Let's Encrypt automatic certificates via `golang.org/x/crypto/acme/autocert`

When `redirect_http: true`, a goroutine listens on `redirect_port` (default 80) and issues `308 Permanent Redirect` to HTTPS.

## Error Handler

All Fiber errors return JSON. Internal errors (500+) are sanitized:

```json
{"code": 500, "message": "internal server error"}
```

Client errors (400-499) pass through as-is. Real error details are logged server-side.

## CORS

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

When `cors` is omitted, CORS defaults to same-origin only (secure default).

## Built-in Middlewares

| Name | File | YAML gate | Description |
|------|------|-----------|-------------|
| `Logger()` | `logger.go` | `server.logger` (default `true`) | Request logging (method, path, status, duration) |
| `Shedding()` | `shedding.go` | `server.load_shedding` (default `true`) | Adaptive load shedding (go-zero) |
| `Breaker()` | `breaker.go` | `server.breaker` (default `true`) | Circuit breaker per route |
| `CORS()` | `cors.go` | `server.cors` (block) | CORS headers |
| `JWT()` | `jwt.go` | `entry[].auth_modes` includes `jwt` | JWT validation with algorithm pinning, claim validation (iss, aud, exp), secret rotation, AuthContext injection |
| `JWTWithZitadel()` | `jwt_zitadel.go` | `auth.driver: openfga-zitadel` | JWT validation via Zitadel JWKS (RS256) |
| `OpenFGA()` | `openfga.go` | `entry[].roles` / `entry[].permissions` | OpenFGA ReBAC authorization (roles, permissions, relation checks) |
| `Ory()` | `ory.go` | `auth.driver: ory` | Ory Keto authorization (roles, permissions) |
| `APIKey()` | `apikey.go` | `entry[].auth_modes` includes `apikey` | API key detection + OpenFGA validation (replaces JWT) |
| `Recover()` | Fiber built-in | Always-on | Panic recovery → 500 JSON |
| `Prometheus()` | `prometheus.go` | Always-on | In-process metrics collector |
| `Trace()` | `trace.go` | `telemetry.enabled: true` | OpenTelemetry tracing |
| `Timeout()` | `timeout.go` | `entry[].timeout` (per-entry) | Per-request deadline |
| `MaxConns()` | `maxconns.go` | Always-on | Concurrency limiter (semaphore) |
| `MaxBytes()` | `maxbytes.go` | Always-on | Body size limiter |
| `Gunzip()` | `gunzip.go` | Always-on | Auto-decompress gzip |
| `HeaderSanitize()` | `header_sanitize.go` | Always-on | CRLF protection |
| `ContentSecurity()` | `content_security.go` | `server.security.content_security.enabled` | RSA body signature verification |
| `Cryption()` | `cryption.go` | `server.security.cryption.enabled` | AES-GCM body decryption |
| `EncryptCookie()` | Fiber built-in | `server.security.encrypt_cookie.enabled` | AES-256-GCM cookie value encryption |
| `SSE()` | `server/middleware/sse.go` | — | Sets SSE headers (Content-Type, Cache-Control, Connection) |

## Server-level Gates

```yaml
server:
  logger: false         # disable request logging (default true)
  load_shedding: false  # disable adaptive load shedding (default true)
  breaker: false        # disable circuit breaker (default true)
```

## Per-entry Timeout

```yaml
entry:
  - type: rest
    method: GET
    path: /slow
    handler: onSlow
    timeout: 30s        # per-request deadline
```

## Per-route Middleware

When `server.middleware` is specified, only `Recover` + `Health` + `HeaderSanitize` are global. All others apply per-path:

```yaml
server:
  middleware:
    - path: "/healthz"
      apply: []                       # Fast path (health+headers only)
    - path: "/api/v1/*"
      apply: [logger, breaker, cors]
    - path: "/api/admin/*"
      apply: [logger, breaker, cors, maxconns]
```

Without `middleware:`, all standard middlewares apply globally.

## CRLF Header Protection

Built-in global middleware (always on, no config). Rejects requests containing `\r` or `\n` bytes in header values, preventing HTTP response splitting attacks.

## SSRF Protection

Disabled by default. When enabled via `server.ssrf`, provides a protected HTTP client:

```go
client := svc.SafeHTTPClient()
resp, err := client.Do(req)
```

Blocks requests to private IPs (`10.x`, `172.16-31.x`, `192.168.x`), loopback (`127.0.0.1`, `::1`), and cloud metadata (`169.254.169.254`).

## OpenAPI & Scalar UI

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

- `GET /openapi.json` — OpenAPI 3.0.3 spec
- `GET /docs` — Scalar UI documentation browser

Models must be registered via `svc.RegisterModel("Product", (*Product)(nil))` for schema generation.

## Static Files

```yaml
server:
  static:
    - prefix: /static
      dir: ./public
```

Serves files from `./public` at `/static/*`. Implemented via Fiber v3 `app.Get()` wildcard routing with `SendFile`.

## Health Endpoint

Every service exposes a liveness probe at `health_path` (default: `/health`):

```json
{"status": "ok"}
```

Returns `200 OK` as long as the process is alive.

## Metrics

Prometheus metrics at `metrics_path` (default: `/metrics`):

```
go_requests_total{...,method="GET",path="/api/v1/products",status="200"}
go_request_duration_ms{...,method="GET",path="/api/v1/products"}
go_requests_active{...,method="GET",path="/api/v1/products"}
```

## Graceful Shutdown

The server shuts down in order:
1. Stop cron scheduler
2. Drain exit workers (waits for in-flight handlers)
3. Drain NATS connections
4. Close DB pools
5. Stop HTTP server
