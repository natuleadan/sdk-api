# 300-file-storage-pg-nats

File storage with PostgreSQL + NATS events + S3.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload/:key) | 558163 | PostgreSQL + NATS events + S3 |
| Download (GET /files/download/:key) | 524839 | PostgreSQL + NATS events + S3 |


## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests |
| `docker-compose.yml` | Services:  |
