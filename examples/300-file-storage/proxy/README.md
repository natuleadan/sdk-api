# 300-file-storage-proxy

File storage with S3 proxy (no cache).

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Upload (POST /files/upload/:key) | **58,350** | 59,671 | 47,315 |
| Download (GET /files/download/:key) | 42,080 | 29,785 | 39,835 |

Upload measured isolated (`--rps:upload`) with warm RustFS on both platforms.

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; Local = 10-core
ARM macOS with VMM acceleration; RustFS 1.0.0-beta.12). Baseline 2026-08-01.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — S3 upload/download proxy without cache |
| `service.yaml` | Service config (api_prefix: /api, S3 storage) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | RustFS S3 + bench |
