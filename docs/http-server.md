# HTTP Server

The HTTP server wraps **Fiber v2** (fasthttp) with 14 built-in middlewares, per-route middleware configuration, and auto-generated OpenAPI documentation.

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

## Entry Types

The server auto-registers routes from `entry:` in YAML. Six types:

| Type | HTTP Methods | What you write | Description |
|------|-------------|----------------|-------------|
| `crud` | GET, POST, PATCH, DELETE | Nothing | Auto-generated CRUD |
| `rest` | Any | Single handler | Custom handler, any method |
| `webhook` | Any (default POST) | Single handler | Webhook receiver, no JWT by default |
| `websocket` | GET (upgrade) | WS handler | WebSocket upgrade |
| `sse` | GET | SSE handler | SSE streaming |
| `file` | GET, POST, PUT, PATCH, DELETE | Upload handler | File upload/download/delete |

## Built-in Middlewares (14)

| Name | File | Description |
|------|------|-------------|
| `Recovery()` | `recover.go` | Panic recovery → 500 JSON |
| `Logger()` | `logger.go` | Request logging (method, path, status, duration) |
| `Shedding()` | `shedding.go` | Adaptive load shedding (go-zero) |
| `Breaker()` | `breaker.go` | Circuit breaker per route |
| `CORS()` | `cors.go` | CORS headers |
| `JWT()` | `jwt.go` | JWT with secret rotation |
| `Prometheus()` | `prometheus.go` | In-process metrics collector |
| `Trace()` | `trace.go` | OpenTelemetry tracing |
| `Timeout()` | `timeout.go` | Per-request deadline |
| `MaxConns()` | `maxconns.go` | Concurrency limiter (semaphore) |
| `MaxBytes()` | `maxbytes.go` | Body size limiter |
| `Gunzip()` | `gunzip.go` | Auto-decompress gzip |
| `ContentSecurity()` | `content_security.go` | RSA body signature verification |
| `Cryption()` | `cryption.go` | AES-CFB body decryption |

## Per-route Middleware

When `server.middleware` is specified, only `Recovery` + `Health` are global. All others apply per-path:

```yaml
server:
  middleware:
    - path: "/healthz"
      apply: []                       # Fast path: recover+health only
    - path: "/api/v1/*"
      apply: [logger, breaker, cors]
    - path: "/api/admin/*"
      apply: [logger, breaker, cors, jwt, maxconns]
```

Without `middleware:`, all 14 middlewares apply globally (backwards compatible).

## JWT

Configured via `auth` in config. When `auth.secret` is set, JWT middleware is added globally:

```yaml
auth:
  secret: my-secret
  prev_secret: old-secret        # Key rotation support
  token_lookup: header:Authorization
  context_key: claims
```

## Error Handler

All Fiber errors return JSON:

```json
{"code": 500, "message": "internal server error"}
```

## OpenAPI & Scalar UI

Auto-generated API documentation (when `server.openapi.enabled` is `true`):

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

Serves files from `./public` at `/static/*`.

## Health Endpoint

Every service exposes a liveness probe at `health_path` (default: `/health`):

```json
{"status": "ok"}
```

Returns `200 OK` as long as the process is alive. Compatible with Kubernetes, AWS ALB, GCP LB, Traefik, Caddy, Nginx.

## Metrics

Prometheus metrics at `metrics_path` (default: `/metrics`):

```
go_requests_total{...,method="GET",path="/api/v1/products",status="200"}
go_request_duration_ms{...,method="GET",path="/api/v1/products"}
go_requests_active{...,method="GET",path="/api/v1/products"}
```

## Graceful Shutdown

The server shuts down in order:
1. Close HTTP listener (no new connections)
2. Wait for in-flight requests up to `shutdown_timeout`
3. After server, runtime drains NATS, closes DB pools
