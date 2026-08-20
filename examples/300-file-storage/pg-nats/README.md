# 300-file-storage-pg-nats

File storage with PostgreSQL + NATS events + S3.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Upload (POST /files/upload) | **2,714** | 2,807 | 2,276 |
| Upload Async (POST /files/upload-async) | 10,729 | 7,484 | 8,345 |
| Download (GET /files/download/:key) | 27,510 | 21,580 | 22,154 |
| Create (POST /products) | **26,705** | 17,700 | 22,331 |
| List (GET /products?size=20) | 38,989 | 28,331 | 29,077 |

Upload and Create measured isolated on a clean dedicated box with warm infra.
The previous dedicated values (75 / 1,895) were anomalous — measured during
NATS/PG startup contention. Async remains ~3.7x faster than sync (spool → 202,
S3 in background via the NATS exit worker).

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; PgDog pool
200/50/8 + RustFS 1.0.0-beta.12 + spooled uploads). Baseline 2026-08-02.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + S3 upload + product view + exit workers |
| `models/model.go` | Product, ProductPublic, UploadResponse structs |
| `models/hooks.go` | ProductHooks (AfterCreate, AfterGet, AfterDelete) + TransformToPublic |
| `service.yaml` | Service config (CRUD + file storage + NATS + OpenAPI) |
| `run.sh` | Entrypoint: --rps for benchmarks (upload, upload-async, download, create, list) |
| `bench_test.go` | Functional tests |
| `upload.lua` / `upload-async.lua` | Sync and async spooled upload benchmarks |
| `download.lua` | S3 file download benchmark |
| `create.lua` / `list.lua` | CRUD product benchmarks |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + NATS JetStream + RustFS S3 |
