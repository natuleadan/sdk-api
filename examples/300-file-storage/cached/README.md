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
| Upload (POST /files/upload/:key) | 69,763 | RustFS + RAM cache L1 |
| Download (GET /files/download/:key) | 128,024 | RustFS + RAM cache L1 |

Measured 2026-08-01 with RustFS 1.0.0-beta.12 (F3). The MinIO RELEASE.2025-09-07 pin (07-28) caused a ~-52% upload regression across all S3 variants; RustFS restores and exceeds the 22,473 baseline by +210%.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — NewFromYAML + Storage upload/download handlers |
| `service.yaml` | Service config (api_prefix: /api, storage with L1 RAM + L2 disk cache) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
