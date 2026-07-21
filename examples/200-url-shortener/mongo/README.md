# 200-url-shortener-mongo

URL shortener with MongoDB direct.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 23,733 | MongoDB direct |
| List (GET /links) | 17,966 | Pagination with skip/limit |
| GetByID (GET /links/:id) | 29,062 | Direct read by PK |
| Create (POST /links) | 32,687 | Insert via MongoDB |
| Update (PUT /links/:id) | 139,859 | Update via MongoDB |
| Delete (DELETE /links/:id) | 18,261 | Delete via MongoDB |

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MongoMustRegister × 2 |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config override |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services definition |
