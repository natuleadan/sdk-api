# 400-auth/manual-pg — Full-Featured Manual Auth

Complete authentication and authorization example with JWT, API keys, MFA, social login, passkeys, OAuth 2.1, and more — all implemented manually against PostgreSQL.

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Features

| Feature | Endpoints | Status |
|---------|-----------|--------|
| JWT auth (HS256) | POST /login, POST /auth/refresh | ✅ |
| API keys | API key validation with roles | ✅ |
| MFA (TOTP) | POST /auth/mfa/enable, /auth/mfa/verify | ✅ |
| Password management | POST /auth/change-password, /auth/forgot-password, /auth/reset-password | ✅ |
| Account lockout | 5 failed attempts → 15min lockout | ✅ |
| Rate limiting | Per-user, per-key, per-role, max-func | ✅ |
| CSRF + CORS + Security Headers | Global middleware | ✅ |
| Tenant scoping | Multi-tenant CRUD with automatic tenant_id filter | ✅ |
| Magic Link | POST /auth/magic-link, GET /auth/magic-link/verify | ✅ |
| Access Code (6-digit) | POST /auth/access-code, POST /auth/access-code/verify | ✅ |
| SMS OTP | POST /auth/sms/send, POST /auth/sms/verify | ✅ |
| Social Login (Google, GitHub) | GET /auth/{provider}/login, GET /auth/{provider}/callback | ✅ |
| Linked Accounts | GET /auth/linked-accounts, POST /auth/link, DELETE /auth/linked-accounts/:id | ✅ |
| WebAuthn / Passkeys | POST /auth/webauthn/register/begin, /register/finish | ✅ |
| WebAuthn Login | POST /auth/webauthn/login/begin, /login/finish (passkey) | ✅ |
| WebAuthn Manual Login | POST /auth/webauthn/login/manual/begin, /login/manual/finish (security key) | ✅ |
| OAuth 2.1 Server | GET /oauth/authorize, POST /oauth/token | ✅ |
| OAuth Token Introspection | POST /oauth/introspect (RFC 7662) | ✅ |
| OAuth Token Revocation | POST /oauth/revoke (RFC 7009) | ✅ |
| OAuth Client CRUD | GET/POST/DELETE /oauth/clients | ✅ |

## OAuth 2.1 Server

Supported grants:
- **Authorization Code + PKCE** (S256, enforced)
- **Client Credentials**
- **Refresh Token** (with rotation)

Clients are stored in PostgreSQL. The seed client `test-client` with secret `test-client-secret` is pre-configured.

## SMS Provider

| Mode | Env vars | Behavior |
|------|----------|----------|
| Mock (dev) | No Twilio vars set | Logs code to console |
| Production | `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM_NUMBER` | Sends real SMS via Twilio |

## Social Login

| Mode | Env vars | Behavior |
|------|----------|----------|
| Mock (dev) | No Google/GitHub vars | Returns state + mock_code for testing |
| Production | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` / `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | Real OAuth2 redirect |

## Files

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — pool, seed data, validators, DDL |
| `internal/handler/helpers.go` | Shared: ValidateJWT (uses SDK auth.RoleHierarchy) + getAuth |
| `internal/handler/rest_routes.go` | Route registration for all endpoints |
| `internal/handler/auth_*.go` | Auth handlers (login, signup, MFA, magic link, SMS, social, etc.) |
| `internal/handler/products_*.go` | Product CRUD handlers |
| `internal/handler/admin_*.go` | Admin handlers (users, roles) |
| `internal/handler/auth_oauth.go` | OAuth 2.1 handlers (authorize, token, introspect, revoke, clients) |
| `internal/handler/auth_webauthn.go` | WebAuthn/Passkeys handlers |
| `internal/handler/social_providers.go` | OAuth2 provider configs (Google, GitHub) |
| `internal/svc/servicecontext.go` | DI container + ProductStore + SMS provider |
| `internal/svc/oauth_store.go` | fosite Storage implementation (PG) |
| `internal/svc/oauth_provider.go` | OAuth 2.1 provider setup (ory/fosite) |
| `internal/svc/sms_provider.go` | SMS provider interface (mock + Twilio) |
| `service.yaml` | Full auth config (327 lines) |
| `bench_test.go` | 160+ integration tests |
| `run.sh` | Entrypoint: `--test:Name` for specific tests |

## Dependencies

| Library | Purpose |
|---------|---------|
| `github.com/golang-jwt/jwt/v5` | JWT signing and validation |
| `github.com/ory/fosite` | OAuth 2.1 server engine |
| `github.com/go-webauthn/webauthn` | WebAuthn/FIDO2 passkey support |
| `golang.org/x/oauth2` | OAuth2 client (social login) |
| `github.com/twilio/twilio-go` | SMS delivery (optional, mock by default) |
