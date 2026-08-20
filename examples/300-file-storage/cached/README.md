# 300-file-storage-cached

File storage with S3 + RAM cache L1.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Upload (POST /files/upload/:key) | **75,079** | 49,761 | 69,763 |
| Download (GET /files/download/:key) | 167,252 | 114,062 | 128,024 |

Upload measured isolated (`--rps:upload`) with warm RustFS on both platforms.

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; Local = 10-core
ARM macOS with VMM acceleration; RustFS 1.0.0-beta.12). Baseline 2026-08-01.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — NewFromYAML + Storage upload/download handlers |
| `service.yaml` | Service config (api_prefix: /api, storage with L1 RAM + L2 disk cache) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | RustFS S3 + bench |
