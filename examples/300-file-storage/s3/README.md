# 300-file-storage-s3

File storage with S3 direct with presign.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Upload (POST /files/upload/:key) | **60,608** | 36,424 | 31,814 |
| Download (GET /files/download/:key) | 41,143 | 30,016 | 33,675 |
| Sign Only (GET /files/sign/:key) | 100,536 | 74,949 | 74,202 |

Upload measured isolated (`--rps:upload`) with warm RustFS on both platforms.

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; Local = 10-core
ARM macOS with VMM acceleration; RustFS 1.0.0-beta.12). Baseline 2026-08-01.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload, proxy download, presigned redirect, sign-only |
| `service.yaml` | Service config (api_prefix: /api, S3 storage with presign) |
| `run.sh` | Entrypoint: --rps for benchmarks (upload, download, sign-only) |
| `bench_test.go` | Functional tests |
| `upload.lua` / `download.lua` / `sign.lua` | S3 benchmarks |
| `docker-compose.yml` | RustFS S3 + bench |
