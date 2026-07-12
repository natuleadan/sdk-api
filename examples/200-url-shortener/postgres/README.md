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
| Expand (GET /expand/:shortCode) | 38,753 | PostgreSQL via PgDog |
| List (GET /links) | 20,901 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 41,838 | Direct PG read by PK |
| Create (POST /links) | 17,058 | PG INSERT with PgDog |
| Update (PUT /links/:id) | 196,073 | PG UPDATE via PgDog |
| Delete (DELETE /links/:id) | 36,657 | PG DELETE via PgDog |

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | Bootstrap via `runtime.MustRegister` |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `bench_test.go` | Functional tests + expand benchmark |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `docker-compose.yml` | PostgreSQL 18 + PgDog |
