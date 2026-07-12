# 300-file-storage-cached

File storage with S3 + RAM cache L1.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload/:key) | 23568 | S3 + RAM cache L1 |
| Download (GET /files/download/:key) | 126469 | S3 + RAM cache L1 |


## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests |
| `docker-compose.yml` | Services:  |
