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
| Upload (POST /files/upload/:key) | 19,696 | S3 direct with presign |
| Download (GET /files/download/:key) | 13,376 | S3 proxy download |
| Sign Only (GET /files/sign/:key) | 62,835 | S3 presign URL generation |

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload, proxy download, presigned redirect, sign-only |
| `service.yaml` | Service config (api_prefix: /api, S3 storage with presign) |
| `run.sh` | Entrypoint: --rps for benchmarks (upload, download, sign-only) |
| `bench_test.go` | Functional tests |
| `upload.lua` / `download.lua` / `sign.lua` | S3 benchmarks |
| `docker-compose.yml` | MinIO S3 + bucket init + bench |
