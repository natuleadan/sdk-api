# 300-file-storage-proxy

File storage with S3 proxy (no cache).

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload/:key) | 18320 | S3 proxy (no cache) |
| Download (GET /files/download/:key) | 19194 | S3 proxy (no cache) |


## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests |
| `docker-compose.yml` | Services:  |
