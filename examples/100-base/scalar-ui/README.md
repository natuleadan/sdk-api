# 101-scalar-ui — CORS matrix across all entry types

Single example that proves CORS works correctly on **every entry type** the SDK
supports without external containers: `rest`, `webhook`, `websocket`, `sse`,
`file`, `async`, `crud` (libsql local file), and `graphql`. Each endpoint has
its own CORS policy via named groups — and none poison each other.

## Endpoints × CORS variants

| Endpoint | Type | CORS group | Origin allowed |
|---|---|---|---|
| `/docs` | Scalar UI | `docs` | `*` (wildcard) |
| `/api/ping` | rest | `app` | `https://app.example.com` |
| `/api/echo` | webhook | `webhooks` | `https://hooks.example.com` |
| `/ws/chat` | websocket | `ws` | `https://app.example.com` (+ private network) |
| `/sse/events` | sse | `ws` | `https://app.example.com` |
| `/api/upload` | file | `app` | `https://app.example.com` |
| `/api/jobs` | async | `app` | `https://app.example.com` |
| `/api/products` | crud (postgres) | `app` | `https://app.example.com` |
| `/api/graphql` | graphql | `app` | `https://app.example.com` |
| `/api/internal/status` | rest | *(none)* | same-origin only |

## CORS groups model

```yaml
server:
  cors_groups:
    - name: docs            # public Scalar UI - open
      origins: ["*"]
      methods: [GET]
      credentials: false
    - name: app             # client API - locked + credentials
      origins: ["https://app.example.com"]
      methods: [GET, POST, PATCH, DELETE]
      headers: [Content-Type, Authorization]
      credentials: true
      expose_headers: [X-Request-ID]
    - name: ws              # realtime - private network
      origins: ["https://app.example.com"]
      methods: [GET]
      credentials: false
      allow_private_network: true
    - name: webhooks        # server-to-server - narrow POST
      origins: ["https://hooks.example.com"]
      methods: [POST]
      headers: [Content-Type]
      max_age: 600

  middleware:
    - path: "/docs"
      apply: ["cors:docs"]

entry:
  - type: rest
    path: /ping
    handler: ping
    cors: app                # per-entry reference
```

## CSP per-route (csp_groups)

The global `csp_config` stays **strict** (`default-src 'self'`) for every route.
Only `/docs` (Scalar) gets an amplified CSP via `csp_groups`, so external
domains are never allowed on your API endpoints.

```yaml
security_headers:
  csp_config:                  # strict global - all routes except groups
    default_src: ["'self'"]
    script_src: ["'self'"]
    style_src: ["'self'", "'unsafe-inline'"]
    img_src: ["'self'", "data:"]
    connect_src: ["'self'"]
    font_src: ["'self'"]

csp_groups:                    # amplified only for matching routes
  - name: docs                 # Scalar: jsdelivr + Google Fonts + fonts.scalar.com
    csp_config:
      script_src: ["'self'", "'unsafe-inline'", "https://cdn.jsdelivr.net"]
      style_src: ["'self'", "'unsafe-inline'", "https://cdn.jsdelivr.net"]
      img_src: ["'self'", "data:", "https:"]
      connect_src: ["'self'", "https://cdn.jsdelivr.net"]   # sourcemap
      font_src:
        - "'self'"
        - "https://fonts.googleapis.com"
        - "https://fonts.gstatic.com"
        - "https://cdn.jsdelivr.net"
        - "https://fonts.scalar.com"

middleware:
  - path: "/docs"
    apply: ["cors:docs", "csp:docs"]   # both CORS and CSP scoped to /docs
```

## crud with postgres (docker-compose)

`crud` requires a `db` in YAML; this example satisfies it with a PostgreSQL
instance from `docker-compose.yml` (`dev:devpass@postgres:5432`). The CRUD
provider is an in-memory Go provider (`WithCRUD` + `RegisterModel`) — the pool
exists only to satisfy config validation and OpenAPI schema generation. No
pgdog, no external infra beyond the compose postgres.

## Why not grpc?

gRPC (HTTP/2 binary) does not use CORS — browsers cannot call it directly.
gRPC-web (via a gateway) does need CORS but requires proto + gateway config;
that is documented here rather than included to keep this example focused on CORS.

## OpenAPI + Scalar theming (v0.25.0)

The `openapi` block is fully YAML-driven. This example enables:

```yaml
openapi:
  enabled: true
  title: Scalar UI Showcase              # overrides spec/docs title
  description: CORS/CSP matrix ...       # sets info.description
  theme: moon                            # scalar theme (moon, default, mars, ...)
  layout: modern                         # modern | classic
  hide_download: true                    # remove "Download spec" button
  spec_cache_ttl: 30m                    # Cache-Control max-age for /openapi.json
  custom_css: |                          # brand CSS (--scalar-* variables)
    :root {
      --scalar-color-accent: #b32323;
    }
```

The root URL redirects to the docs (no Go code):

```yaml
redirects:
  - from: /
    to: /docs
    status: 302
```

For options outside YAML there are two hooks: `svc.WithOpenAPIMutator(...)`
(mutate the generated spec) and `svc.WithScalarOptions(...)` (raw scalar-go
options). Full block reference: `docs/configuration.md` — Scalar UI.

## Favicon

The SDK serves a default inline SVG (magnifying glass) at `/favicon.ico` when
`openapi.enabled: true`. Three modes:

```yaml
openapi:
  enabled: true
  # Mode 1 (default): empty -> inline magnifying glass
  # Mode 2 (local): file on disk, read once at startup
  favicon_url: "static/logo.svg"
  # Mode 3 (remote): downloaded server-side with TTL cache
  # favicon_url: "https://cdn.example.com/logo.svg"
  # favicon_refresh: "24h"   # optional TTL (default 24h)
```

Supported extensions: `.svg`, `.png`, `.ico`, `.webp`. In all modes the bytes
are cached in memory with an `ETag` (repeat requests get `304 Not Modified`).

## Running the logic tests

```bash
# local (builds and runs the service, runs CORS tests)
./run-test-logic.sh

# docker (build + functional tests in container)
docker compose run --rm bench
```

The test matrix (`bench_test.go`) verifies per-endpoint CORS headers:
- `TestCORS_Docs_Wildcard` — `/docs` allows any origin
- `TestCORS_App_AllowedOrigin` / `_DeniedOrigin` — locked origin + credentials
- `TestCORS_WS_PrivateNetwork` — `Allow-Private-Network` header
- `TestCORS_Webhooks_Narrow` — POST-only, hooks origin
- `TestCORS_Internal_NoCORS` — same-origin default
- `TestCORS_NoPoisoning` — origins from one group fail on another
- `TestCORS_CRUD_App` / `TestCORS_GraphQL_App` — crud/graphql share app group
- `TestOpenAPI_HasEndpoints` / `TestFavicon_Inline` / `TestHealthz`

Since v0.25.0 the matrix also covers the OpenAPI/Scalar options:
- `TestRootRedirectsToDocs` — `/` lands on `/docs` (302)
- `TestOpenAPI_TitleAndSummary` — `openapi.title` + entry summary in the spec
- `TestDocs_ThemingAndLayout` — theme/layout/hide_download/custom_css in HTML
- `TestOpenAPI_SpecCacheHeader` — `spec_cache_ttl` drives Cache-Control

## Quick start

```bash
cd examples/101-scalar-ui
go run ./cmd/
open http://localhost:23101/docs
```

## Docker

```bash
docker build -t scalar-ui .
docker run -p 23101:23101 scalar-ui
```
