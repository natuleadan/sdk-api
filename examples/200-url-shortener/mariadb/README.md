# 200-url-shortener-mariadb

URL shortener with MariaDB via PgDog proxy.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 50,501 | MariaDB via PgDog proxy |
| List (GET /links) | 71,890 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 52,710 | Direct read by PK |
| Create (POST /links) | 15,697 | Insert via MariaDB |
| Update (PUT /links/:id) | 195,204 | Update via MariaDB |
| Delete (DELETE /links/:id) | 45,783 | Delete via MariaDB |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `hooks.go` | BeforeCreate auto-generates short codes |
| `models/link.go` | Link model (PK: id) |
| `models/link_expand.go` | LinkExpand model (PK: short_code) |
| `service.docker.yaml` | Docker config |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services definition |
