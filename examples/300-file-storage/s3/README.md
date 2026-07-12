# 300-file-storage-s3

File storage with S3 direct with presign.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload/:key) | 16864 | S3 direct with presign |
| Download (GET /files/download/:key) | 17763 | S3 direct with presign |
| Sign Only (GET /files/sign/:key) | 69603 | Presigned URL generation |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests |
| `docker-compose.yml` | Services:  |
