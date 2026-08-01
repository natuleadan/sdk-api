# 200-url-shortener-postgres

URL shortener with PostgreSQL and PgDog pooler. No cache — every request hits the database. Two CRUD entries on the same `link` table: one with `id` as PK, another with `short_code` as PK for the expand endpoint. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + pgx + PgDog pooler.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres` | PostgreSQL via PgDog |
| `CONFIG_PATH` | `service.docker.yaml` | Prefork on, pool 500 |
| PgDog pool | `20` | Transaction pooler, no `[admin]` |

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 48,500 | PostgreSQL via PgDog |
| List (GET /links) | 25,208 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 37,017 | Direct PG read by PK |
| Create (POST /links) | 18,013 | PG INSERT with PgDog |
| Update (PATCH /links/:id) | 15,864 | PG UPDATE via PgDog |
| Delete (DELETE /links/:id) | 37,968 | PG DELETE via PgDog |

Measured 2026-08-01 with PgDog pool 200/50/8 + max_connections 500. The previous Update row (187,776) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database. The same applies to all pre-2026-08-01 Update rows above ~20K across every variant. Update benchmarks now use PATCH with a 1-2000 id range (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap via `runtime.MustRegister` |
| `models/link.go` | Link model + `BeforeCreate` auto-generates short codes |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `bench_test.go` | Functional tests + expand benchmark |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `docker-compose.yml` | PostgreSQL 18 + PgDog |
