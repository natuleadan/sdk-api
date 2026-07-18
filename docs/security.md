# Security

sdk-api provides multiple layers of security: headers, CSRF, rate limiting, TLS, SSRF protection, input validation, secrets management, and error sanitization. All configured via YAML.

## Quick Reference

| Feature | Config section | Default | Status |
|---------|---------------|---------|--------|
| Security headers | `server.security_headers` | Off | Always: `X-Content-Type-Options: nosniff` |
| CSRF | `server.csrf` | Off | Token in cookie + header required |
| Rate limiting | `server.rate_limit` | Off | Token bucket, memory or Redis |
| TLS | `server.tls` | Off | Manual or autocert (Let's Encrypt) |
| SSRF protection | `server.ssrf` | Off | Blocks private/loopback/metadata IPs |
| Error sanitization | — | On by default | 500+ errors return generic message |
| CRLF protection | — | On by default | Rejects `\r`/`\n` in headers |
| Secrets check | — | On by default | Warns on plaintext secrets in YAML |
| Secrets env defaults | — | On by default | `${VAR:default}` syntax supported |
| Column whitelist | — | On by default | Validates column names in SQL queries |
| Struct validation | `entry[].validate` | Off | Requires `go-playground/validator/v10` |

## Security Headers

```yaml
server:
  security_headers:
    frame_options: DENY
    referrer_policy: strict-origin-when-cross-origin
    permissions_policy: "camera=(), microphone=()"
    hsts: true
    hsts_max_age: 31536000
    csp: "default-src 'self'"
    csp_report_path: /csp-violation
    coop: same-origin
    coep: require-corp
    corp: same-origin
    cache_control: "no-store"
```

Always on: `X-Content-Type-Options: nosniff`.

### CSP Builder

The SDK also provides a programmatic CSP builder:

```go
import "github.com/natuleadan/sdk-api/server/middleware"

// Pre-built levels
strictCSP := middleware.BuildCSP(middleware.CSPConfig{Level: middleware.CSPLevelStrict})

// Custom
customCSP := middleware.BuildCSP(middleware.CSPConfig{
    DefaultSrc: []string{"'none'"},
    ScriptSrc:  []string{"'self'", "https://cdn.example.com"},
})

// Generate nonces for inline scripts
nonce := middleware.GenerateNonce()
```

**YAML config:**

```yaml
server:
  security_headers:
    csp_config:
      level: strict
      default_src: ["'self'"]
      script_src: ["'self'", "'strict-dynamic'"]
      upgrade_insecure_requests: true
```

The `csp_config` section uses the `BuildCSP()` function to generate the policy. When both `csp` (raw string) and `csp_config` are set, `csp_config` takes precedence.

## Authentication & Authorization

sdk-api provides four authentication modes and three strategies for role/permission validation. The JWT middleware validates tokens per-entry, supports algorithm pinning and claim validation, and extracts an `AuthContext` available in handlers and hooks.

**Three ways to validate roles:**

| # | Strategy | Mode | Source of truth | entry.roles links to... |
|---|----------|------|-----------------|------------------------|
| **A** | JWT claims | `none` / `manual` | `roles` claim inside the JWT | `AuthContext.Roles[]` — validated locally |
| **B** | Custom hook | `manual` | Your own DB, Redis, NATS KV, or API | Passed to `WithAuthValidator` callback |
| **C** | External ReBAC | `openfga-zitadel` / `ory` | OpenFGA or Ory Keto (tuples + Check API) | Verified via gRPC Check to OpenFGA/Keto |

The YAML defines **what roles are required** per endpoint. The driver defines **where and how those roles are verified**.

### Auth modes

| Mode | Identity | Authorization | Role validation | Dependencies | Use case |
|------|----------|--------------|----------------|--------------|----------|
| `none` | JWT (shared secret) | None (or JWT claims) | JWT claims (strategy A) | 0 | Service-to-service, internal APIs |
| `manual` | JWT (shared secret) | Custom validator | JWT claims (A) or custom hook (B) | 0 | Custom auth logic (DB, Redis) |
| `openfga-zitadel` | Zitadel OIDC (JWKS) | OpenFGA ReBAC | OpenFGA Check (C) | Zitadel + OpenFGA + 2x PG | Multi-tenant SaaS, fine-grained permissions |
| `ory` | Ory Kratos OIDC | Ory Keto ReBAC | Keto Check (C) | Kratos + Keto + PG | Enterprise IAM with Ory ecosystem |

### YAML configuration

```yaml
server:
  auth:
    enabled: true
    driver: openfga-zitadel       # none | manual | openfga-zitadel | ory
    secret: "${JWT_SECRET}"       # Shared secret for HS256 (mode: none/manual)
    algorithm: HS256              # HS256 | HS384 | HS512 | RS256
    context_key: claims           # Key used in fiber.Ctx.Locals()
    issuer: "sdk-api"             # Validate iss claim
    audience: "api.example.com"   # Validate aud claim
    openfga_url: "http://localhost:18080"
    openfga_store: "default"
    zitadel_url: "https://auth.tu-dominio.com"
    kratos_url: "http://localhost:4433"
    keto_url: "http://localhost:4466"

  # RSA body signing / AES body decryption (opt-in)
  security:
    content_security:
      enabled: false
      public_key: "/etc/secrets/rsa.pub"
    cryption:
      enabled: false
      key: "${AES_KEY}"
    encrypt_cookie:
      enabled: false
      key: "${COOKIE_ENCRYPT_KEY}"       # base64-encoded 32-byte key (AES-256-GCM)
      except:
        - csrf_token
```

### Cookie Encryption

The `encrypt_cookie` middleware encrypts cookie values using AES-256-GCM. Every `Set-Cookie` header is intercepted and the value is encrypted on the wire. Cookies are transparently decrypted when read via `c.Cookies()`. This prevents sensitive cookie data (like JWT tokens) from being readable in plaintext even if intercepted.

```yaml
server:
  security:
    encrypt_cookie:
      enabled: true
      key: "${COOKIE_ENCRYPT_KEY}"
      except:                               # cookies that must remain readable by JS
        - csrf_token
```

**Important:** The key must be base64-encoded. Generate one with:
```bash
openssl rand -base64 32
```

When `encrypt_cookie` is enabled and login sets a `token` cookie, the value is:
- Encrypted on the wire (`Set-Cookie: token=<ciphertext>`)
- Decrypted transparently by the middleware when JWT reads `c.Cookies("token")`
- Never visible to JavaScript (when `HttpOnly` flag is set)

```yaml
# JWT cookie encryption + HttpOnly example
server:
  security:
    encrypt_cookie:
      enabled: true
      key: "diPHoCg5vhBrTHCSJhlud1RRMRFpRo+4N/d32S+48t8="
```

On the login handler, the cookie is set with security flags:
```go
c.SetCookie(&fiber.Cookie{
    Name:     "token",
    Value:    signed,
    Path:     "/",
    HTTPOnly: true,
    Secure:   true,
    SameSite: "Strict",
    MaxAge:   900,
})
```

### JWT Token Lifetime

JWT tokens expire after a configurable TTL. The default is **900 seconds (15 minutes)**:

```yaml
auth:
  expiry: 900            # JWT TTL in seconds (default: 900 = 15min)
```

Shorter TTLs reduce the window for token theft. For long-lived sessions, use the refresh token flow.

| `prev_secret` | string | — | Previous JWT secret for key rotation. When set, tokens signed with either secret are accepted. |

### Per-entry auth

Each entry can enable or disable authentication independently:

```yaml
entry:
  # Public endpoint — no auth
  - type: rest
    path: /health
    handler: healthCheck

  # Authenticated endpoint — any valid JWT
  - type: rest
    path: /whoami
    handler: whoami
    auth_modes: [jwt]

  # Authenticated + role-gated
  - type: crud
    resource: products
    auth_modes: [jwt]
    roles: ["products:editor"]
    permissions: ["products:write"]
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auth_modes` | []string | `[]` | Authentication modes (`jwt`, `apikey`, or both) |
| `roles` | []string | `[]` | Required roles (validated by driver) |
| `permissions` | []string | `[]` | Required permissions (validated by driver) |
| `jwt_from` | string | `"header:Authorization"` | JWT source: `"header:Authorization"`, `"cookie:token"`, `"query:token"` |
| `api_key_prefix` | string | `""` | Optional prefix to validate (e.g. `"sk-"`, only when `auth_modes` includes `apikey`) |

**Entry auth combinations:**

| `auth_modes` | `roles` / `permissions` | What the middleware does |
|--------------|------------------------|-------------------------|
| (empty) | — | No auth, public endpoint |
| `[jwt]` | empty | Validates JWT signature + claims (identity only) |
| `[jwt]` | `["editor"]` | Validates JWT + verifies roles/permissions via driver |
| `[apikey]` | — | API key validation via driver |
| `[jwt, apikey]` | — | Both (router detects format) |

### AuthContext

After JWT validation, the middleware injects an `AuthContext` into both `fiber.Ctx.Locals()` and the request `context.Context`. Hooks automatically receive it via their `ctx` parameter.

```go
import "github.com/natuleadan/sdk-api/server/middleware"

// From fiber handler
auth := middleware.GetAuth(c)
fmt.Println(auth.UserID, auth.OrgID, auth.Roles)

// From hook (context.Context)
func (h *Hooks) BeforeCreate(ctx context.Context, req Product) (Product, error) {
    auth := middleware.AuthFromContext(ctx)
    if !slices.Contains(auth.Roles, "products:editor") {
        return req, fmt.Errorf("forbidden")
    }
    return req, nil
}
```

Fields:

| Field | Type | Source | Description |
|-------|------|--------|-------------|
| `UserID` | string | JWT `sub` claim | Authenticated user ID |
| `OrgID` | string | JWT `org_id` claim | Organization / tenant |
| `Roles` | []string | JWT `roles` claim | Roles assigned to user |
| `Permissions` | []string | JWT `permissions` claim | Permissions assigned to user |
| `RawToken` | string | Authorization header | Raw JWT string |
| `Claims` | jwt.MapClaims | Parsed JWT | Complete claims map |

### How roles are validated

The YAML fields `entry.roles` and `entry.permissions` define **contracts**: "this endpoint requires these roles/perms". How they are verified depends on the auth driver:

| Strategy | Driver(s) | Who resolves the role→user mapping | How `entry.roles` is checked |
|----------|-----------|-----------------------------------|------------------------------|
| **A: JWT claims** | `none`, `manual` | The IDP that issued the JWT | Middleware extracts `claims.roles[]` and compares with `entry.roles[]` (set intersection). The hook receives the result in `AuthContext.Roles`. |
| **B: Custom hook** | `manual` | Your code via `WithAuthValidator` | SDK calls your validator callback with `(ctx, auth, entry.roles, entry.permissions)`. You query DB, Redis, or any source and return error if denied. |
| **C: OpenFGA / Keto** | `openfga-zitadel`, `ory` | OpenFGA or Ory Keto (tuples) | Middleware calls `fga.Check(user, "role:admin", "role-assignment")` or `keto.Check(namespace, object, relation, subject)` for each role. |

In all cases, the hook receives the final `AuthContext` with `Roles` and `Permissions` populated, and can perform additional fine-grained checks.

### Mode: none — JWT claims

When `driver: none`, the JWT is validated using the shared secret. Roles and permissions are read from the JWT claims and compared against `entry.roles` / `entry.permissions` from YAML:

```yaml
server:
  auth:
    driver: none
    secret: "${JWT_SECRET}"
    algorithm: HS256

entry:
  - type: rest
    path: /admin/stats
    handler: getStats
    auth_modes: [jwt]
    roles: ["admin"]
```

```go
// The hook receives AuthContext with roles from JWT claims
func (h *StatsHooks) AfterRead(ctx context.Context, id string, s *Stats) error {
    auth := middleware.AuthFromContext(ctx)
    // auth.Roles = ["admin", "editor"]  ← from JWT claims
    // entry required "admin" → middleware already verified it
    // Fine-grained check: does this admin own this stat?
    if !slices.Contains(auth.Roles, "admin") {
        return fmt.Errorf("forbidden")
    }
    return nil
}
```

Roles are validated locally (no network call). The JWT acts as a signed cache — changes take effect when the user obtains a new token.

### Mode: manual — custom validator

When `driver: manual`, the JWT is validated as usual but role/permission checks are delegated to a user-registered callback:

```go
svc.WithAuthValidator(func(ctx context.Context, auth *middleware.AuthContext, roles, permissions []string) error {
    // Query your own DB, Redis, or NATS KV
    // Both roles and permissions from the YAML entry are passed.
    allowed, err := db.Query("SELECT 1 FROM user_roles WHERE user=$1 AND role=ANY($2)", auth.UserID, roles)
    if err != nil || !allowed {
        return fmt.Errorf("forbidden: insufficient roles")
    }
    return nil
})
```

### Mode: openfga-zitadel

Uses **Zitadel** as OpenID Connect provider (login, MFA, user management) and **OpenFGA** for fine-grained relationship-based authorization (ReBAC).

**Zitadel** validates JWTs via JWKS:

```go
fgaClient.Check(ctx, openfga.CheckRequest{
    User:     "user:org123:user456",
    Relation: "can_write",
    Object:   "products:create",
})
```

The middleware automatically:
1. Validates the JWT against Zitadel's JWKS endpoint
2. Calls OpenFGA gRPC Check API for configured roles/permissions
3. Caches results in NATS KV or Redis (configurable)

Start OpenFGA + Zitadel via Docker:

```bash
docker compose -f docker-compose.test.yml up -d openfga zitadel
```

### Mode: ory

Uses **Ory Kratos** for identity and **Ory Keto** for authorization. The middleware validates JWT tokens using Kratos's public JWKS endpoint (RS256) and checks permissions via Keto `/relation-tuples/check`:

```go
oryClient.KetoCheck(ctx, ory.KetoCheckRequest{
    Namespace: "products",
    Object:    "document:42",
    Relation:  "can_edit",
    SubjectID: "user:456",
})
```

### API key authentication

API keys are wired per-entry via YAML. When `auth_modes: [apikey]`, the middleware reads the API key from the `Authorization` header (replacing JWT). The API key is validated against OpenFGA if configured, otherwise only presence + prefix is checked.

```yaml
entry:
  - type: webhook
    path: /webhooks/stripe
    auth_modes: [apikey]             # Accept API keys (replaces JWT)

  - type: webhook
    path: /webhooks/github
    auth_modes: [apikey]
    api_key_prefix: "gh_"            # Optional prefix validation
```

The automatic wiring registers the middleware with:
- `Object`: `{entry.type}:{entry.path}` (e.g. `webhook:/webhooks/stripe`)
- `Relation`: `"can_access"`
- `Client`: the configured OpenFGA client (if `auth.driver: openfga-zitadel`)

When a FGA client is available, the API key is treated as a subject (`apikey:<key_id>`) and checked via OpenFGA Check. Without FGA, any valid key passes (presence-only validation).

The `Authorization` header is read and the `Bearer ` prefix is automatically stripped. If the key has a configured prefix (`api_key_prefix`), it must match.

### Choosing an auth mode

```
Do you have an external identity provider?
├── No → I just need JWT between services
│   └── Use `driver: none`
│       └── Roles come from JWT claims (strategy A)
│
├── No → I want to define my own auth logic
│   └── Use `driver: manual`
│       └── Register `WithAuthValidator` (strategy B)
│       └── Or keep roles in JWT claims (strategy A)
│
└── Yes → I have Zitadel, Keycloak, or Ory
    ├── Do I need fine-grained, multi-tenant permissions?
    │   ├── Yes → Use `driver: openfga-zitadel`
    │   │   └── OpenFGA handles all permissions (strategy C)
    │   └── Yes, with Ory stack → Use `driver: ory`
    │       └── Keto handles permissions (strategy C)
    └── No, just identity → `driver: none` + JWT from your IDP
        └── JWT claims carry roles (strategy A)
```

### Token refresh

The SDK provides a configurable token refresh handler that delegates to the identity provider.

**YAML-driven auto-wire (recommended):**

```yaml
auth:
  refresh:
    enabled: true
    ttl: 604800
    endpoint: /auth/refresh
  cookie:
    access_token_name: token
    path: /
    http_only: true
    secure: true
    same_site: Strict
```

When `auth.refresh.enabled: true`, the runtime auto-registers `POST /auth/refresh`, sets the cookie according to `auth.cookie.*`, and returns `{"access_token", "token_type", "expires_in"}`.

**Manual handler (alternative):**

```go
import "github.com/natuleadan/sdk-api/server/middleware"

app.Post("/auth/refresh", middleware.TokenRefreshHandler(middleware.TokenRefreshConfig{
    JWTSecret:       "shared-secret",   // Manual mode
    ZitadelTokenURL: "https://auth.tld/oauth/v2/token",  // Zitadel mode
    ZitadelClientID: "sdk-api",
    KratosRefreshURL: "http://kratos:4433/sessions/refresh",  // Ory mode
}))
```

### Cryptography helpers

`middleware.SignToken(secret, algorithm string, claims map[string]any) (string, error)` — creates and signs a JWT

`middleware.DefaultClaims(sub, orgID string, roles, permissions []string, ttlSeconds int) map[string]any` — builds standard JWT claims

`auth.HashPassword(password string) (string, error)` — bcrypt hash a password

`auth.VerifyPassword(hash, password string) bool` — verify password against bcrypt hash

## CSRF

```yaml
server:
  csrf:
    enabled: true
    cookie_name: csrf_token
    header_name: X-CSRF-Token
    same_site: Strict
    secure: true
    json_check: true              # Skip CSRF for JSON requests (safe via SOP)
    exclude_paths:
      - /webhooks/*
```

<details>
<summary>How it works</summary>

1. GET response sets `csrf_token` cookie (non-HttpOnly, readable by JS)
2. Frontend reads cookie, sends it as `X-CSRF-Token` header on mutating requests
3. Server compares cookie vs header — 403 on mismatch

When `json_check: true`, requests with `Content-Type: application/json` skip CSRF validation entirely (browser Same-Origin Policy protects JSON). Recommended for API-only backends.

Per-entry exclusion: `entry[].csrf: false`.
</details>

## Rate Limiting

```yaml
server:
  rate_limit:
    enabled: true
    kv: cache-main   # references kv[].name
    algorithm: token_bucket         # token_bucket | sliding_window (default)
    global:
      requests_per_second: 1000
      burst: 2000
    per_ip:
      requests_per_second: 200
      burst: 300
    per_user:
      requests_per_second: 100
      burst: 150
    per_key:
      requests_per_second: 500
      burst: 1000
    skip_failed_requests: false      # Don't consume rate limit on error responses
    skip_successful_requests: false  # Don't consume rate limit on success responses
```

### Dimensions

| Dimension | Key | Scope |
|-----------|-----|-------|
| Global | — | Whole server |
| Per-IP | Client IP | Single IP address |
| Per-User | JWT `sub` claim | Authenticated user |
| Per-Key | API key value | Single API key |

### Skip flags

When `skip_failed_requests: true`, rate limit tokens are not consumed for HTTP responses >= 400.
When `skip_successful_requests: true`, tokens are not consumed for HTTP responses < 400.
Useful for pricing or abuse detection where you only want to count successful operations.

### Dynamic override (WithRateLimitMaxFunc)

At runtime, register a callback that can override per-entry rate limits dynamically:

```go
svc.WithRateLimitMaxFunc(func(c fiber.Ctx) int {
    if c.Get("X-Debug") == "true" {
        return 5 // override both RequestsPerSecond and Burst
    }
    return 0 // use static YAML config
})
```

When the function returns > 0, both `requests_per_second` and `burst` are set to the returned value for that request. See `examples/400-auth/manual-pg`.

### Post-auth rate limits (per-entry)

In addition to server-level limits, entries can define per-user, per-key, and per-role rate limits that run after authentication:

```yaml
entry:
  - type: rest
    path: /api/v1/expensive-report
    rate_limit:                       # Pre-auth: checked before authentication
      requests_per_second: 1
      burst: 2
    rate_limit_per_user:              # Post-auth: per authenticated user
      requests_per_second: 5
      burst: 10
    rate_limit_per_key:               # Post-auth: per API key
      requests_per_second: 10
      burst: 20
    rate_limit_per_role:              # Post-auth: per role
      admin:
        requests_per_second: 10
        burst: 20
      viewer:
        requests_per_second: 1
        burst: 2
```

### Headers

Rate-limited requests return:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
```

## TLS

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
    curve_preferences: [X25519, P-256]
    cipher_suites:
      - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    redirect_http: true
    redirect_port: 80
```

| Mode | When to use |
|------|-------------|
| **Off** | Behind Caddy/Nginx reverse proxy |
| **Manual** | Custom certificates or internal CA |
| **Autocert** | Direct internet exposure, Let's Encrypt |

When `redirect_http: true`, a goroutine listens on port 80 and issues 308 redirect to HTTPS.

## SSRF Protection

Disabled by default. When enabled, provides a protected HTTP client:

```yaml
server:
  ssrf:
    enabled: true
    block_private: true       # 10.x, 172.16-31.x, 192.168.x
    block_loopback: true      # 127.0.0.1, ::1
    block_metadata: true      # 169.254.169.254
    allowed_hosts:
      - api.stripe.com
```

```go
client := svc.SafeHTTPClient()
resp, err := client.Do(req)   // Validates host before connecting
```

The implementation uses a sub-package to break gosec cross-package taint analysis for G704 (SSRF).

## Input Validation

### Struct Validation (opt-in)

```yaml
entry:
  - type: crud
    resource: products
    model: Product
    validate: "CreateProductInput"
```

```go
type CreateProductInput struct {
    Name  string  `json:"name" validate:"required,min=3,max=100"`
    Price float64 `json:"price" validate:"required,gt=0"`
}

svc.RegisterValidation("CreateProductInput", CreateProductInput{})
```

Returns `422` with field-level errors on failure.

### Column Whitelist

All database column names in `FindBy`, `Update`, `Exists`, `Increment`, `QueryPaginated`, `Count`, `Upsert`, and `QueryWhere` are validated against the model's fields. Invalid column names return an error before any SQL is executed.

### File Upload Validation

```yaml
entry:
  - type: file
    path: /files/upload
    handler: onUpload
    allowed_types: [image/jpeg, image/png]
    max_size: 10MB
    max_files: 5
    magic_bytes: true
```

| Rule | Description |
|------|-------------|
| `allowed_types` | Content-Type whitelist |
| `max_size` | Maximum body size |
| `max_files` | Maximum files per multipart request |
| `magic_bytes` | Verify file content matches declared MIME type |

### Filename Sanitization

The `SanitizeFilename()` function is available for user handlers:

```go
safeName := runtime.SanitizeFilename(originalName)
```

- Removes path separators (`/`, `\`)
- Removes null bytes
- Limits length to 255 characters
- Only allows safe characters: `[a-zA-Z0-9._-]`
- Preserves file extension

## Error Sanitization

All errors pass through a sanitizer that redacts sensitive infrastructure details:

- **IP addresses** — `dial tcp 10.0.0.5:5432: timeout` → `dial tcp [redacted]:5432: timeout`
- **Connection strings** — `postgres://admin:pass@host/db` → `postgres://[redacted]@host/db`
- **File paths** — `/etc/sdk-api/service.yaml` → `[redacted]`

Internal server errors (500+) return a generic message after redaction:

```json
{"code": 500, "message": "internal server error"}
```

Client errors (400-499) preserve the error message but with sensitive data redacted:

```json
{"code": 400, "message": "dial tcp [redacted]:5432: timeout"}
```

The real error with full context is logged server-side via `logx.Errorf`. CRLF injection prevention is applied before the sanitizer.

## CRLF Protection

Global middleware (always on). Rejects requests containing `\r` or `\n` bytes in any header value. Prevents HTTP response splitting and header injection attacks.

## Secrets Management

### Best Practices

1. Use `${VAR}` for all secrets in YAML
2. Use `${VAR:default}` when a default value exists
3. Never hardcode passwords, keys, or tokens in YAML files
4. Use environment variables or a vault solution in production

### Env Var Expansion

```yaml
databases:
  - name: primary
    url: "${DATABASE_URL}"              # Required, error logged if missing
    pool:
      max_conns: "${DB_POOL_SIZE:10}"   # Default 10 if env var not set
```

The SDK logs a warning at startup if it detects values that look like plaintext secrets (containing `password`, `secret`, `key`, `token`, or `auth`).

### SOPS/age

For encrypting YAML files at rest in git repositories, use SOPS with age keys:

```bash
sdk-ops secrets init          # Generate age key
sdk-ops secrets encrypt service.yaml > service.enc.yaml
sdk-ops secrets decrypt service.enc.yaml > service.yaml
```

## Global Cookie Settings

```yaml
server:
  cookies:
    same_site: Lax
    secure: true
```

Applied as defaults to CSRF tokens and configurable for other cookies.

## Security Scanning (Makefile)

```bash
make security-deps        # govulncheck — dependency vulnerabilities
make security-sast        # gosec — static analysis security testing
make security-sbom        # syft — software bill of materials
make security-audit       # All of the above
```

### CI Integration

Add to `.github/workflows/ci.yml`:

```yaml
- name: Security audit
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...
    go install github.com/securego/gosec/v2/cmd/gosec@latest
    gosec -quiet ./...
```

## Tenant Scoping

For multi-tenant applications, CRUD entries support automatic data isolation via `tenant_scope` + `tenant_field`:

```yaml
entry:
  - type: crud
    model: Product
    resource: products
    auth_modes: [jwt]
    tenant_scope: org_id        # JWT claim containing the tenant ID
    tenant_field: tenant_id     # DB column to filter on
```

When `tenant_field` is set, the SDK injects a middleware that extracts the tenant ID from the JWT claim specified in `tenant_scope` (defaults to `org_id`). All CRUD operations automatically include a `WHERE tenant_field = <tenant_id>` clause:

| Operation | SQL |
|-----------|-----|
| List | `SELECT * FROM products WHERE tenant_id = $1 ORDER BY id` |
| Get | `SELECT * FROM products WHERE id = $1 AND tenant_id = $2` |
| Create | Injects `tenant_id` from JWT into the row (client-side `tenant_id` values are ignored) |
| Update | `UPDATE products SET ... WHERE id = $1 AND tenant_id = $2` |
| Delete | `DELETE FROM products WHERE id = $1 AND tenant_id = $2` |

Tenant scoping is supported for PostgreSQL, MySQL, Turso, and MongoDB providers. See `examples/400-auth/manual-pg` for a complete demo with cross-tenant isolation tests.

## MFA / TOTP

Multi-factor authentication via Time-based One-Time Passwords (TOTP) is supported through the `runtime/auth` package (stdlib only, no external dependencies):

| Endpoint | Description |
|----------|-------------|
| `POST /auth/mfa/enable` | Generates a TOTP secret and returns `otpauth://` URI for authenticator apps |
| `POST /auth/mfa/verify` | Validates a 6-digit code, returns a new JWT with `mfa: true` claim |

Configure per-entry: `entry[].requires_mfa: true` rejects requests that lack `mfa: true` in JWT claims.

The JWT middleware checks the `mfa` claim via `MFARequired()` middleware. After successful MFA verification, the new JWT contains `mfa: true`, granting access to MFA-protected endpoints. See `examples/400-auth/manual-pg` for a complete MFA enable/verify/required flow.

## Token Revocation / Blacklist

JWT tokens can be revoked via `svc.WithJWTBlacklist()`. The callback receives the raw token string and returns `true` if the token should be rejected:

```go
svc.WithJWTBlacklist(func(rawToken string) bool {
    pool := svc.PoolPGTyped("primary")
    hash := sha256Hex(rawToken)
    var exists bool
    pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE token_hash=$1)`, hash).Scan(&exists)
    return exists
})
```

The blacklist check runs after JWT validation in all auth drivers (manual, ory, openfga-zitadel). Token hash is stored via a revoke endpoint (`POST /auth/revoke`). See `examples/400-auth/manual-pg` for the complete implementation with `withBlacklist` wrapper.

## Account Lockout

After 5 consecutive failed login attempts, the account is locked for 15 minutes. Lockout is tracked in the `failed_logins` table and reset after a successful login. See `examples/400-auth/manual-pg` for the implementation.

## Password Strength

Signup validates passwords with:
- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one digit

See `examples/400-auth/manual-pg` `checkPasswordStrength()`.

## Email Verification

New users receive a verification token logged to console (mock — no actual email sent). Verify via `GET /auth/verify-email?token=...`. See `examples/400-auth/manual-pg`.

## Password Reset

Three-step flow:
1. `POST /auth/forgot-password` — generates a reset token, logs URL to console
2. `POST /auth/reset-password` — consumes the token, updates password hash
3. Tokens expire after 1 hour

See `examples/400-auth/manual-pg`.

## Summary of default security posture

| Attack vector | Protected? | Since |
|--------------|-----------|-------|
| Clickjacking | ✅ (X-Frame-Options) | v0.3+ |
| MIME sniffing | ✅ (X-Content-Type-Options) | v0.3+ |
| XSS via CSP | ✅ (Content-Security-Policy) | v0.3+ |
| CSRF | ✅ (Double-submit cookie) | v0.3+ |
| Brute force | ✅ (Rate limiting) | v0.3+ |
| MITM | ✅ (TLS + HSTS) | v0.3+ |
| SSRF | ✅ (SafeHTTPClient) | v0.3+ |
| CRLF injection | ✅ (Header sanitize) | v0.3+ |
| Error information leak | ✅ (Error sanitization) | v0.3+ |
| SQL column injection | ✅ (Column whitelist) | v0.3+ |
| Input validation | ✅ (struct validator) | v0.3+ |
| Secrets in git | ⚠️ (Plaintext warning) | v0.3+ |
| **JWT forgery** | ✅ **(Algorithm pinning)** | v0.5+ |
| **Role escalation** | ✅ **(OpenFGA / Ory Keto / manual validator)** | v0.5+ |
| **API key leakage** | ✅ **(Scoped API keys + OpenFGA)** | v0.5+ |
| **Cross-tenant access** | ✅ **(Tenant scoping)** | v0.6+ |
| **Token revocation** | ✅ **(Blacklist callback)** | v0.6+ |
| **MFA / TOTP** | ✅ **(TOTP middleware)** | v0.6+ |
| **Account lockout** | ✅ **(Failed login tracking)** | v0.6+ |
| **Password strength** | ✅ **(Signup validation)** | v0.6+ |
| **Email verification** | ✅ **(Token-based mock)** | v0.6+ |
| **Password reset** | ✅ **(Token with expiry)** | v0.6+ |
