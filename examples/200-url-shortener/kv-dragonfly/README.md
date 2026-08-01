# 200-url-shortener-kv-dragonfly

URL shortener with Dragonfly/Redis in-memory.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /api/expand/:shortCode) | 108,256 | Dragonfly/Redis in-memory |
| List (GET /api/links) | 32,303 | Pagination with SSCAN + pipelined GETs |
| GetByID (GET /api/links/:id) | 113,828 | Direct read by primary key |
| Create (POST /api/links) | 34,989 | Insert via Dragonfly |
| Update (PATCH /api/links/:id) | 44,547 | Update via Dragonfly |
| Delete (DELETE /api/links/:id) | 35,865 | Delete via Dragonfly |

Measured 2026-08-01 with Dragonfly `--proactor_threads=2 --maxclients=20000` and SDK-fixed List (pagination defaults page=1/size=10 like the CRUD entries, SSCAN cursor parsed as `string`, GETs batched via redis pipeline). The previous List row (9,406, and the ~18K baseline) were invalid: the SSCAN cursor was cast to `[]byte` while go-redis v9 returns `string`, so every list request panicked into a 500 response (bug since the 07-12 SSCAN refactor, undetected because no functional test covers List). Real list throughput is 32,303. The previous Update row (378,057) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap with `runtime.New()` + `handler.RegisterRoutes()` |
| `internal/handler/routes.go` | Route registration |
| `internal/handler/*.go` | Per-endpoint handlers (create, list, get, update, delete, expand) |
| `internal/logic/links.go` | Business logic (Redis CRUD) |
| `internal/svc/servicecontext.go` | DI container with `*redis.Redis` |
| `service.yaml` | YAML config with `api_prefix: /api` |
| `bench_test.go` | Functional tests + benchmark |
| `run.sh` | Entrypoint: `--rps` for benchmarks |
| `docker-compose.yml` | Dragonfly + bench container |
