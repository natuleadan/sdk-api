# 300-file-storage-ephemeral

File storage with In-memory filesystem.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Upload (POST /files/upload/:key) | 42,266 | 65,550 | 61,191 |
| Download (GET /files/download/:key) | 132,552 | 104,287 | 115,033 |

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; Local = 10-core
ARM macOS with VMM acceleration; wrk inside Docker). Baseline 2026-08-01.
Dedicated upload was measured in a full run — re-measure isolated for clean
reference.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — 4 REST handlers (upload, download, list, delete) |
| `service.yaml` | Service config (api_prefix: /api, 4 REST entries) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + upload/download benchmarks |
| `docker-compose.yml` | Bench container with tmpfs volume |
