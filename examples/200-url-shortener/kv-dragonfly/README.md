# 200-url-shortener-kv-dragonfly

URL shortener with Dragonfly/Redis in-memory.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /api/expand/:shortCode) | 159,539 | 117,849 | 108,256 |
| List (GET /api/links) | 37,105 | 36,120 | 32,303 |
| GetByID (GET /api/links/:id) | 153,687 | 117,927 | 113,828 |
| Create (POST /api/links) | 46,620 | 34,232 | 34,989 |
| Update (PATCH /api/links/:id) | 62,296 | 45,429 | 44,547 |
| Delete (DELETE /api/links/:id) | 48,659 | 36,592 | 35,865 |

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; wrk inside Docker, clean host before measuring; Dragonfly
`--proactor_threads=2 --maxclients=20000`). Baseline 2026-08-01. The List row
includes the SSCAN fix (cursor parsed as `string`, pipelined GETs).

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
