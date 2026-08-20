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

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /expand/:shortCode) | 29,929 | 22,050 | 48,500 |
| List (GET /links) | 30,094 | 23,593 | 25,208 |
| GetByID (GET /links/:id) | 62,926 | 42,836 | 37,017 |
| Create (POST /links) | 64,190 | 38,909 | 18,013 |
| Update (PATCH /links/:id) | 30,468 | 22,167 | 15,864 |
| Delete (DELETE /links/:id) | 54,465 | 29,695 | 34,239 |

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; same code, same tag, wrk inside Docker). Baseline 2026-08-01
with PgDog pool 200/50/8 + max_connections 500. The baseline create (18K) was
measured under host memory pressure; the dedicated column reflects a clean box.
Pre-2026-08-01 Update rows above ~20K were invalid (PUT against a PATCH-only
route — see docs/benchmarks.md rules 10-11).

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
