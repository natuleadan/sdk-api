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
| Upload (POST /files/upload) | 2,276 | Spooled sync: ingest → S3 → 200 |
| Upload Async (POST /files/upload-async) | 8,345 | Spooled async: ingest → 202, S3 in background |
| Download (GET /files/download/:key) | 22,154 | RustFS + L1 RAM cache + L2 disk cache |
| Create (POST /products) | 22,331 | PG insert + NATS event publish |
| List (GET /products?size=20) | 29,077 | Keyset pagination |

Measured 2026-08-02 with PgDog pool 200/50/8 + max_connections 500, RustFS 1.0.0-beta.12 and spooled uploads (storage.spool). Both upload modes stream the body to memory (4MB) then to /tmp/spool before touching S3 — the ingest bound is local disk, not S3 or RAM. **Async is the default mode**: it returns 202 right after spooling (upload handled by the NATS exit worker) and is 3.7x faster than sync (8,345 vs 2,276). Multipart part size and concurrency are YAML-driven (`multipart_part_size`, `multipart_concurrency`).


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
