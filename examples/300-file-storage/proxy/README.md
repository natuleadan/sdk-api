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
| Upload (POST /files/upload/:key) | 15,796 | S3 proxy (no cache) |
| Download (GET /files/download/:key) | 18,803 | S3 proxy (no cache) |


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload/download proxy without cache |
| `service.yaml` | Service config (api_prefix: /api, S3 storage) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
