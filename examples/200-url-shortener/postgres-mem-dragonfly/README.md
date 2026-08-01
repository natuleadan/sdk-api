# 200-url-shortener-postgres-mem-dragonfly

URL shortener with PostgreSQL + RAM L1 + Dragonfly L2.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 259,606 | PostgreSQL + RAM L1 (sync.Map) + Dragonfly L2 |
| List (GET /links) | 20,790 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 42,025 | Direct read by PK |
| Create (POST /links) | 17,633 | Insert via PostgreSQL |
| Update (PATCH /links/:id) | 16,854 | Update via PostgreSQL |
| Delete (DELETE /links/:id) | 34,239 | Delete via PostgreSQL |

Measured 2026-08-01 with PgDog pool 200/50/8 + max_connections 500 (fix applied 2026-07-31) and Dragonfly `--proactor_threads=2 --maxclients=20000` (2026-08-01). The Dragonfly tuning lifted update 14,827→16,854 (+14%), expand 249,149→259,606 (+4%) and create 16,561→17,633 (+6%) vs the plain `--cluster_mode=emulated` config; list/getbyid/delete within ±10% run noise. The previous Update row (202,813) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + CachedCRUD with L1+L2 |
| `models/link.go` | Link model + BeforeCreate hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code, L1+L2 cached) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + Dragonfly |
