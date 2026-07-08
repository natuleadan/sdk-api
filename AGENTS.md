# sdk-api — AI Guide

## Overview

General-purpose Go SDK for event-driven microservices and monoliths. YAML-driven, entry/exit architecture, security built-in.

**Stack:** Fiber (fasthttp) + pgxpool (PostgreSQL) + MongoDB + NATS JetStream + Kafka + go-zero infra (45+ packages)

**Security:** Security headers, CSRF, rate limiting, TLS (manual + autocert), SSRF protection, input validation, error sanitization, column whitelist, secrets validation — all YAML-driven.

## Quick reference

| Topic | Location |
|-------|----------|
| Full documentation | `docs/` (18 files) |
| YAML config schema (all entry types, security) | `docs/configuration.md` |
| Security guide (headers, CSRF, rate limit, TLS, SSRF, validation) | `docs/security.md` |
| HTTP server & middlewares | `docs/http-server.md` |
| Runtime API | `docs/runtime.md` |
| NATS + Kafka messaging | `docs/messaging.md` |
| Database drivers & CRUD | `docs/database.md` |
| GraphQL entry type | `docs/entry-graphql.md` |
| Async jobs entry type | `docs/entry-async.md` |
| Secrets management | `docs/secrets.md` |
| API patterns (all entry types) | `docs/api-patterns.md` |
| CLI commands | `docs/cli.md` |
| Best practices & gotchas | `docs/best-practices.md` |
| Live examples (dockerized) | `examples/` |

## Entrypoints

- `cmd/sdk-api/` — CLI generator (new/docker/kube/client)
- `runtime/` — Service orchestrator, entry router, exit workers, cron, hooks
- `server/` — Fiber HTTP + 14 middlewares + storage backends
- `db/` — Table[T] CRUD (pgx, Turso, MySQL, MongoDB) + AutoInit
- `events/` — NATS JetStream: producers, consumers, KV cache, request-reply
- `infra/` — 45+ go-zero packages (conf, logx, trace, breaker, redis, discover)

## Architecture

```
 ┌──────────┬─────────────┬──────────────┬─────────────┬───────────┐
 │   db/    │   server/   │   events/    │  runtime/   │  infra/   │
 │  (pgx)   │  (Fiber)    │  (NATS+      │ (orchestr.) │ (go-zero) │
 │          │             │   Kafka)     │             │           │
 │ Table[T] │ 14+ middle- │  EventBroker │ Service     │ 45+ pkgs  │
 │ CRUD     │ wares       │  Producers   │ YAML cfg    │ conf,logx │
 │ AutoInit │ Security    │  Exit Workers│ Entry routes│ trace,brk │
 │ PG/Turso │ Headers     │  KV Cache    │ 9 entry     │ redis,mon │
 │ MySQL    │ CSRF/RL/TLS │  Request-    │ types       │ discov    │
 │ MongoDB  │ SSRF/Valid  │  Reply       │ Security    │           │
 └──────────┴─────────────┴──────────────┴─────────────┴───────────┘
```

## Patterns (full code in docs/)

- **CRUD + Entry hooks** — define struct with `db:""` tags → register `CRUDProvider` → implement hooks for business logic → `AutoInit()` creates table on startup
- **NATS exit worker** — add `exit:[].subscribe.stream` + `.handler` to YAML → `svc.WithExit()` → return `([]byte, error)` (reply if enabled)
- **Multi-DB** — add `databases:[]` with separate names → reference via `entry[].db` or `exit[].db`
- **OpenAPI** — set `server.openapi.enabled: true` → `svc.RegisterModel("Product", (*Product)(nil))`

## Workflows (for AI assistants)

### Create a microservice
1. Ask for model fields
2. `sdk-api new <name> --model M --fields "a:string,b:int"`
3. Verify `service.yaml` (databases, entry, server)
4. Write hooks in `models/model.go`
5. Wire main.go: `NewFromYAML(configYAML)` with `//go:embed` → `WithCRUDFactory()` → `Run()`
   (or `WithCRUD()` with a pre-built provider when pool is not needed)

### Add a REST endpoint
1. Add `entry: - type: rest method: GET path: ... handler: name`
2. `svc.WithRest("name", func(c *runtime.RestCtx) error { ... })`

### Add an exit worker
1. Add `exit: - name: w subscribe.stream: s handler: onMsg`
2. `svc.WithExit("onMsg", func(ctx, msg []byte) ([]byte, error) { ... })`

### Add a cron job
1. Add `cron: - name: c schedule: "0 6 * * *" mode: handler handler: onTask`
2. `svc.WithCron("onTask", func(ctx) error { ... })`

### Deploy to Vercel
1. Add `deploy.target: vercel` to `service.yaml`
2. `sdk-api vercel --output vercel.json` (validates prefork=false, tls=false)
3. `vercel deploy --prod`

### Change runtime mode
1. By default, the generated project uses `runtime.NewFromYAML` with embedded config
2. For file-based config (not recommended for production), use `runtime.New("service.yaml")`

## Testing

```bash
go test -short ./...          # Unit tests (no external services)
go test ./...                 # All tests (requires Docker: PG + NATS)
```

## Gotchas

- **`db` vs `json` tags** — independent. DB tags control columns, JSON tags control API.
- **NATS KV keys** — must match `[-/_=.[:alnum:]]`. No colons or spaces.
- **Pool sizing** — `max(1, (PG_MAX_CONNS - RESERVED) / REPLICAS)`. RESERVED defaults to 10.
- **Prefork + cache** — each process has its own memory. Use NATS KV (shared) or disable prefork.
- **Cron** — 5-field expressions only (`min hour dom month dow`). No seconds support.
- **OpenAPI** — requires `RegisterModel()` for schema generation. Without it, paths exist but schemas are empty.
- **Auth: JWT middleware is wired per-entry** — Use `entry.auth: true` to enable, configure via `auth:` block. See `examples/auth-*` for 4 driver modes (none/manual/openfga-zitadel/ory).
- **Pool access before Run** — `svc.Pool()` returns nil before `svc.Run()`. Use `WithCRUDFactory()` (preferred, no manual sync.Once) or `sync.Once` + lazy init for CRUD tables.
- **WARNING: `go build` skips `*_test.go` files, `go test` includes them.** When `bench_test.go` uses `exec.Command("go", "build", ...)`, the test binary and the service binary are different. The test binary compiles both `main.go` and `bench_test.go`. The service binary (`go build`) compiles only `main.go`.
- **SSRF is disabled by default** — enable via `server.ssrf.enabled: true` to activate `SafeHTTPClient()`.
- **Rate limit `driver: redis`** — requires `redis_url` configured and Redis running.
- **CSRF excludes webhooks** — use `entry[].csrf: false` or `server.csrf.exclude_paths`.
- **Secrets warning** — the SDK logs when values look like plaintext secrets. Always use `${VAR}`.
- **`svc.WithRest` uses `*RestCtx`, not `*fiber.Ctx`** — the SDK wraps Fiber's context so handlers don't need to import `github.com/gofiber/fiber/v3`. Use `c.Params()`, `c.JSON()`, `c.Status()` etc. on the `*RestCtx` parameter.
- **Turso `_busy_timeout`** — set via DSN: `"mydb.db?_busy_timeout=30000"` or via YAML `turso: {mode: local, busy_timeout: 30000}`. `mode: remote` skips PRAGMAs (for Turso Cloud).
- **RPS benchmarks** — `docker compose up --abort-on-container-exit` runs only functional tests by default. Add `RPS_BENCH=1` to run wrk benchmarks with 6 endpoints.
- **Mongo pool config** — set via YAML `pool.max_conns` (appends `?maxPoolSize=N&maxConnecting=10` to URI). Does not improve RPS; MongoDB's bottleneck is BSON overhead.

## Linting Rules

The project enforces strict lint rules. All must pass before commit:

| Rule | Enforced by | Policy |
|------|-------------|--------|
| 0 `//nolint` comments | Project policy | Every issue fixed, not silenced |
| 0 `_ =` error ignores | `errcheck` | Errors logged via `logx.Errorf` or propagated |
| 0 unused params | `unparam` | Dead parameters removed |
| Complexity < 15 | `gocyclo` | Functions testable and maintainable |
| No deprecated APIs | `staticcheck` SA1019 | Prevent build breaks on dep upgrades |
| Custom context keys | `staticcheck` SA1029 | `ctxKey` type, not bare `string` |
| Go version pinned | CI config | `go-version: "1.26.x"` in all workflows |

## Crypto

| Component | Algorithm | Notes |
|-----------|-----------|-------|
| Config defaults | SHA-256 (via `hash.Md5`/`hash.Md5Hex`) | Functions named Md5 for backward compat, use SHA-256 internally |
| Request body en/decryption | AES-256-GCM | `server.middleware.cryption.go` — 12-byte nonce |
| TLS minimum | 1.2 | `MinVersion: tls.VersionTLS12` |
| CSP nonce | 32 random bytes | `crypto/rand` via `middleware.GenerateNonce()` |
