# Manual Auth Features Guide

The CLI generates **skeleton handlers** for manual auth features. Each skeleton returns `501` and references the exact files to copy from the reference implementation in `examples/400-auth/manual-pg/`. Implement each feature by copying the reference logic into the generated handlers and adapting the configuration.

## Generate

```bash
sdk-api new my-svc --model User --fields "name:string,email:string" \
    --auth manual \
    --features mfa,magic-link,sms,social,webauthn,oauth-server
```

This generates:

```
my-svc/
├── cmd/main.go                        # WithAuthValidator wiring
├── service.yaml                       # auth block + one entry per endpoint
└── internal/handler/
    ├── auth_mfa.go                    # MFAEnable, MFAVerify (501 stubs)
    ├── auth_magic-link.go             # MagicLinkSend, MagicLinkVerify
    ├── auth_sms.go                    # SMSSend, SMSVerify
    ├── auth_social.go                 # SocialLogin, SocialCallback, LinkedAccounts, LinkAccount, UnlinkAccount
    ├── auth_webauthn.go               # 8 passkey handlers
    └── auth_oauth-server.go           # 10 OAuth 2.1 + OIDC handlers
```

Delete the feature files you do not need (and their entries in `service.yaml`).

## Feature Reference Map

Each generated file header lists the source files to copy. The reference example uses a `handle*` prefix on function names — rename to the generated names (they match the `service.yaml` handler entries).

| Feature | Copy from `examples/400-auth/manual-pg/` | Models | Extra deps | Env vars |
|---------|------------------------------------------|--------|-----------|----------|
| **mfa** | `internal/handler/auth_mfa_enable.go`, `auth_mfa_verify.go` | `MFASecret` | none (SDK `runtime/auth` TOTP) | — |
| **magic-link** | `internal/handler/auth_magic_link.go` | (users table) | none (SDK `middleware.SignToken`/`ParseToken`) | — |
| **sms** | `internal/handler/auth_sms.go`, `internal/svc/sms_provider.go` | `AuthCode` | `go get github.com/twilio/twilio-go` | `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM_NUMBER` |
| **social** | `internal/handler/auth_social.go`, `internal/handler/social_providers.go` | `LinkedAccount` | `go get golang.org/x/oauth2` | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` |
| **webauthn** | `internal/handler/auth_webauthn.go` | `WebAuthnUser`, `WebAuthnCredential`, `WebAuthnSession` | `go get github.com/go-webauthn/webauthn` | `WEBAUTHN_RP_NAME`, `WEBAUTHN_RP_ID`, `WEBAUTHN_ORIGINS` |
| **oauth-server** | `internal/handler/auth_oauth.go`, `internal/svc/oauth_provider.go`, `internal/svc/oauth_store.go` | `OAuthClient`, `OAuthSession`, `OAuthJTIS` | `go get github.com/ory/fosite github.com/go-jose/go-jose/v3` | `OIDC_PRIVATE_KEY_B64` |

## oauth-server: extra wiring

The OAuth feature needs the fosite provider initialized before routes are registered. Copy the following from the reference example:

1. **Provider** — `internal/svc/oauth_provider.go` (fosite `ComposeAllEnabled` + RSA JWKS). Adapt `IDTokenIssuer` to your host.
2. **Storage** — `internal/svc/oauth_store.go` (30 methods / 10 fosite interfaces over 3 tables).
3. **Models** — `OAuthClient`, `OAuthSession`, `OAuthJTIS` structs with `db` tags.
4. **Init + seed** — from `cmd/main.go:156-168`: `svcCtx.InitOAuth(pool)` and seed at least one client:
   ```go
   svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
       pool := s.PoolPGTyped("pg-main")
       svcCtx.InitOAuth(pool)
       // seed oauth_clients row (hashed secret, redirect_uris, grant_types...)
       return nil
   })
   ```
5. **RSA key** — generate and base64-encode a PKCS8 RSA key:
   ```bash
   openssl genrsa -out oidc_private_key.pem 2048
   base64 -i oidc_private_key.pem        # macOS
   base64 -w0 oidc_private_key.pem       # linux
   ```

## Validating JWT RS256 from the OAuth server

Once the OAuth server issues ID tokens, validate them in any SDK service without touching fosite:

```yaml
auth:
  enabled: true
  driver: manual
  jwks_url: "https://oauth.internal/.well-known/jwks.json"
```

The middleware fetches the JWKS, resolves keys by `kid`, and validates RS256 tokens with rotation. See `docs/security.md`.

## Order of implementation

1. `mfa` — zero deps, quick win to learn the pattern
2. `magic-link` — zero deps, uses SDK JWT helpers
3. `sms` — mock provider first (works without Twilio), then add credentials
4. `social` — mock mode returns `mock_code` without credentials
5. `webauthn` — needs origin/domain config
6. `oauth-server` — heaviest (fosite adapter); last
