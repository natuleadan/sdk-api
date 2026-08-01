# 200-url-shortener-postgres-dragonfly

URL shortener with PostgreSQL + Dragonfly cache.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 123,869 | PostgreSQL + Dragonfly L2 cache |
| List (GET /links) | 21,334 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 43,123 | Direct read by PK |
| Create (POST /links) | 19,553 | Insert via PostgreSQL |
| Update (PATCH /links/:id) | 17,192 | Update via PostgreSQL |
| Delete (DELETE /links/:id) | 35,239 | Delete via PostgreSQL |

Measured 2026-08-01 with PgDog pool 200/50/8 + max_connections 500 (fix applied 2026-07-31) and Dragonfly `--proactor_threads=2 --maxclients=20000` (2026-08-01). The Dragonfly tuning lifted every endpoint vs the plain `--cluster_mode=emulated` config: expand 86,279→123,869 (+44%), create 17,003→19,553 (+15%), delete 29,215→35,239 (+21%). The previous Update row (253,469) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + CachedCRUD (write-through) |
| `models/link.go` | Link model + BeforeCreate hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code, cached) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + Dragonfly |
