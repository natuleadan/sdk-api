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
| Upload (POST /files/upload) | 2,741 | S3 + L1 RAM cache + L2 disk cache |
| Download (GET /files/download/:key) | 25,775 | S3 + L1 RAM cache + L2 disk cache |
| Create (POST /products) | 15,739 | PG insert + NATS event publish |
| List (GET /products?size=20) | 29,739 | Keyset pagination |


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
| `docker-compose.yml` | PostgreSQL 18 + PgDog + NATS JetStream + MinIO S3 |
