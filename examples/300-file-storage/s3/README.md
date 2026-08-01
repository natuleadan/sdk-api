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
| Upload (POST /files/upload/:key) | 31,814 | RustFS direct with presign |
| Download (GET /files/download/:key) | 33,675 | RustFS proxy download |
| Sign Only (GET /files/sign/:key) | 74,202 | RustFS presign URL generation |

Measured 2026-08-01 with RustFS 1.0.0-beta.12 (F3). The MinIO RELEASE.2025-09-07 pin (07-28) caused a ~-52% upload regression across all S3 variants; RustFS restores and exceeds the 19,696 upload baseline (+62%) and the 13,376 download baseline (+152%).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload, proxy download, presigned redirect, sign-only |
| `service.yaml` | Service config (api_prefix: /api, S3 storage with presign) |
| `run.sh` | Entrypoint: --rps for benchmarks (upload, download, sign-only) |
| `bench_test.go` | Functional tests |
| `upload.lua` / `download.lua` / `sign.lua` | S3 benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
