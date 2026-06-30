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

## CSRF

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

<details>
<summary>How it works</summary>

1. GET response sets `csrf_token` cookie (non-HttpOnly, readable by JS)
2. Frontend reads cookie, sends it as `X-CSRF-Token` header on mutating requests
3. Server compares cookie vs header — 403 on mismatch

Per-entry exclusion: `entry[].csrf: false`.
</details>

## Rate Limiting

```yaml
server:
  rate_limit:
    enabled: true
    driver: memory            # memory | redis
    redis_url: "${REDIS_URL}"
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

### Dimensions

| Dimension | Key | Scope |
|-----------|-----|-------|
| Global | — | Whole server |
| Per-IP | Client IP | Single IP address |
| Per-User | JWT `sub` claim | Authenticated user |

### Headers

Rate-limited requests return:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
```

### Per-entry override

```yaml
entry:
  - type: rest
    path: /api/v1/expensive-report
    rate_limit:
      requests_per_second: 1
      burst: 2
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

Internal server errors (500+) return a generic message:

```json
{"code": 500, "message": "internal server error"}
```

The real error is logged server-side via `logx.Errorf`. Client errors (400-499) pass through as-is.

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
    gosec -quiet -exclude=G304,G307 ./...
```

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
