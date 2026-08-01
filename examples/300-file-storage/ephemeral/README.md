# 300-file-storage-ephemeral

File storage with In-memory filesystem.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload/:key) | 61,191 | In-memory filesystem |
| Download (GET /files/download/:key) | 115,033 | In-memory filesystem |

Measured 2026-08-01.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — 4 REST handlers (upload, download, list, delete) |
| `service.yaml` | Service config (api_prefix: /api, 4 REST entries) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | Bench container with tmpfs volume |
