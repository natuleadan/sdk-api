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
| Upload (POST /files/upload/:key) | 56420 | In-memory filesystem |
| Download (GET /files/download/:key) | 99785 | In-memory filesystem |


## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests |
| `docker-compose.yml` | Services:  |
