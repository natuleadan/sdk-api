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
| Upload (POST /files/upload/:key) | 22,473 | S3 + RAM cache L1 |
| Download (GET /files/download/:key) | 121,088 | S3 + RAM cache L1 |


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — NewFromYAML + Storage upload/download handlers |
| `service.yaml` | Service config (api_prefix: /api, storage with L1 RAM + L2 disk cache) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
