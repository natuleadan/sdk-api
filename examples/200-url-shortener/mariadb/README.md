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
| Expand (GET /expand/:shortCode) | 30,088 | MariaDB via ProxySQL pooler |
| List (GET /links) | 44,082 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 29,292 | Direct read by PK |
| Create (POST /links) | 18,036 | Insert via MariaDB |
| Update (PUT /links/:id) | 138,390 | Update via MariaDB |
| Delete (DELETE /links/:id) | 26,231 | Delete via MariaDB |

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
