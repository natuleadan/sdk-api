# 200-url-shortener-mariadb

URL shortener with MariaDB via PgDog proxy.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker via ProxySQL)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 34,014 | MariaDB via ProxySQL pooler |
| List (GET /links) | 23,336 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 37,857 | Direct read by PK |
| Create (POST /links) | 9,878-17,000 | Insert via MariaDB (host memory-pressure dependent) |
| Update (PATCH /links/:id) | 34,382 | Update via MariaDB |
| Delete (DELETE /links/:id) | 35,066 | Delete via MariaDB |

Measured 2026-08-01. The previous Update row (138,390) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database (see docs/benchmarks.md rules 10-11). Create swings 9.8K-17K between runs depending on host memory pressure (swap on macOS).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MySQLMustRegister × 2 |
| `models/link.go` | Link model + BeforeCreate hook (auto short code) |
| `models/link_expand.go` | LinkExpand model (read-only, PK: short_code) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config override |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services: mariadb + proxysql (pooler) + bench |
| `proxysql.cnf` | ProxySQL configuration for connection pooling |
