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
| Expand (GET /expand/:shortCode) | 118,331 | Dragonfly/Redis in-memory |
| List (GET /links) | 15,656 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 114,503 | Direct read by PK |
| Create (POST /links) | 51,703 | Insert via Dragonfly |
| Update (PUT /links/:id) | 64,884 | Update via Dragonfly |
| Delete (DELETE /links/:id) | 55,797 | Delete via Dragonfly |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | 6 REST handlers (create/list/get/update/delete/expand) via `svc.WithRest` |
| `service.docker.yaml` | 6 `type: rest` entries, `apply: []` middleware, no databases |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Dragonfly + bench container |
