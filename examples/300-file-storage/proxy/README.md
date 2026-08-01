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
| Upload (POST /files/upload/:key) | 47,315 | RustFS proxy (no cache) |
| Download (GET /files/download/:key) | 39,835 | RustFS proxy (no cache) |

Measured 2026-08-01 with RustFS 1.0.0-beta.12 (F3). The MinIO RELEASE.2025-09-07 pin (07-28) caused a ~-52% upload regression across all S3 variants; RustFS restores and exceeds the 15,796 baseline by +200%.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload/download proxy without cache |
| `service.yaml` | Service config (api_prefix: /api, S3 storage) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
