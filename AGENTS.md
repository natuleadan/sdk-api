# sdk-api — AI Guide

## Overview

General-purpose Go SDK for event-driven microservices and monoliths. YAML-driven, entry/exit architecture.

**Stack:** Fiber (fasthttp) + pgxpool (PostgreSQL) + NATS JetStream + go-zero infra (45+ packages)

## Quick reference

| Topic | Location |
|-------|----------|
| Full documentation | `docs/` (14 files) |
| API patterns (6 entry types, exit, cron, hooks) | `docs/api-patterns.md` |
| YAML config schema | `docs/configuration.md` |
| Runtime API (Service, CRUDProvider, RegisterModel) | `docs/runtime.md` |
| HTTP server & per-route middleware | `docs/http-server.md` |
| Database drivers & CRUD | `docs/database.md` |
| NATS messaging & exit workers | `docs/messaging.md` |
| CLI (new/docker/kube/client) | `docs/cli.md` |
| Best practices & gotchas | `docs/best-practices.md` |
| Live examples (dockerized) | `examples/` |

## Entrypoints

- `cmd/sdk-api/` — CLI generator (new/docker/kube/client)
- `runtime/` — Service orchestrator, entry router, exit workers, cron, hooks
- `server/` — Fiber HTTP + 14 middlewares + storage backends
- `db/` — Table[T] CRUD (pgx, Turso, MySQL) + AutoInit
- `events/` — NATS JetStream: producers, consumers, KV cache, request-reply
- `infra/` — 45+ go-zero packages (conf, logx, trace, breaker, redis, discover)

## Architecture

```
 ┌──────────┬─────────────┬──────────────┬─────────────┬───────────┐
 │   db/    │   server/   │   events/    │  runtime/   │  infra/   │
 │  (pgx)   │  (Fiber)    │  (NATS JS)   │ (orchestr.) │ (go-zero) │
 │          │             │              │             │           │
 │ Table[T] │ 14 middle-  │  Producers   │ Service     │ 45+ pkgs  │
 │ CRUD     │ wares       │  Exit Workers│ YAML cfg    │ conf,logx │
 │ AutoInit │ JWT/CORS    │  KV Cache    │ Entry routes│ trace,brk │
 │ PG/Turso │ SSE/WS      │  PublishAnd  │ Exit workers│ redis,mon │
 │ MySQL    │ OpenAPI      │  Wait        │ Cron        │ discov    │
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
5. Wire main.go: `New()` → `Pool()` → `NewCRUDProvider()` → `WithCRUD()` → `Run()`

### Add a REST endpoint
1. Add `entry: - type: rest method: GET path: ... handler: name`
2. `svc.WithRest("name", func(c *fiber.Ctx) error { ... })`

### Add an exit worker
1. Add `exit: - name: w subscribe.stream: s handler: onMsg`
2. `svc.WithExit("onMsg", func(ctx, msg []byte) ([]byte, error) { ... })`

### Add a cron job
1. Add `cron: - name: c schedule: "0 6 * * *" mode: handler handler: onTask`
2. `svc.WithCron("onTask", func(ctx) error { ... })`

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
