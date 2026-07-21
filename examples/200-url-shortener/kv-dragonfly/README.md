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
| Expand (GET /api/expand/:shortCode) | ~120K | Dragonfly/Redis in-memory |
| List (GET /api/links) | ~18K | Pagination with SSCAN |
| GetByID (GET /api/links/:id) | ~127K | Direct read by primary key |
| Create (POST /api/links) | ~38K | Insert via Dragonfly |
| Update (PATCH /api/links/:id) | ~57K | Update via Dragonfly |
| Delete (DELETE /api/links/:id) | ~51K | Delete via Dragonfly |

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
