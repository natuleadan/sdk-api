# 300-file-storage-pg-nats

File storage with PostgreSQL + NATS events + S3.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Upload (POST /files/upload) | 1,694 | RustFS + L1 RAM cache + L2 disk cache |
| Download (GET /files/download/:key) | 23,327 | RustFS + L1 RAM cache + L2 disk cache |
| Create (POST /products) | 17,144 | PG insert + NATS event publish |
| List (GET /products?size=20) | 30,096 | Keyset pagination |

Measured 2026-08-01 with PgDog pool 200/50/8 + max_connections 500 (fix applied 2026-07-31) and RustFS 1.0.0-beta.12 (F3). Upload is capped at ~1.7-2.7K by the handler's per-upload presign + L2 disk cache + NATS work, independent of the S3 backend (observed with MinIO and RustFS alike) — pending optimization.


## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + S3 upload + product view + exit workers |
| `models/model.go` | Product, ProductPublic, UploadResponse structs |
| `models/hooks.go` | ProductHooks (AfterCreate, AfterGet, AfterDelete) + TransformToPublic |
| `service.yaml` | Service config (CRUD + file storage + NATS + OpenAPI) |
| `run.sh` | Entrypoint: --rps for benchmarks (upload, download, create, list) |
| `bench_test.go` | Functional tests |
| `upload.lua` / `download.lua` | S3 file transfer benchmarks |
| `create.lua` / `list.lua` | CRUD product benchmarks |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + NATS JetStream + RustFS S3 |
