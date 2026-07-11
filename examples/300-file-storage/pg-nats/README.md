# 300-file-storage-pg-nats

Full-stack microservice: PostgreSQL (products CRUD) + NATS JetStream (event publishing) + S3 file upload (media per product). Demonstrates SDK `type: crud` + `type: file` in the same service, exit workers, OpenAPI, and S3 cache + presign.

Three entry types:
- **CRUD** — Product model backed by PostgreSQL, publishes `media.created`/`media.deleted` events to NATS
- **File upload** — S3-backed with RAM cache + presign, stores media reference in PG
- **REST view** — `GET /products/:id/view` JOINs product + media with a NATS exit worker

**Stack:** Fiber + pgxpool (PostgreSQL) + NATS JetStream + MinIO (S3). SDK `type: crud` + `type: file` + exit workers.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | `18088` | HTTP port |
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres?sslmode=disable` | PostgreSQL via PgDog pooler |
| `NATS_URL` | `nats://nats:4222` | NATS JetStream |
| `S3_ENDPOINT` | `http://minio:9000` | MinIO endpoint |
| `S3_BUCKET` | `dummy-bucket` | S3 bucket |
| `S3_ACCESS_KEY` | `minioadmin` | MinIO access key |
| `S3_SECRET_KEY` | `minioadmin` | MinIO secret key |

YAML:
```yaml
databases:
  - driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 500
      min_conns: 10

event_streams:
  - driver: nats
    streams:
      - name: media

entry:
  - type: crud
    model: Product
    table: products
    path: /products
    pagination: keyset
    page_size: 20
    event_publish:
      - stream: media
        subject: media.created
        on: create
      - stream: media
        subject: media.deleted
        on: delete

  - type: file
    path: /files/upload
    handler: onFileUpload
    storage:
      mode: s3
      presign: true
      cache: { l1: ram, ... }

  - type: rest
    path: /products/:id/view
    handler: onGetProductWithMedia

exit:
  - name: onMediaUploaded
    subscribe:
      stream: media
      subject: media.created
    handler: onMediaUploaded
```

## Quick Start

```bash
docker compose up --build -d
docker compose logs app -f
docker compose down -v
```

Benchmark (wrk inside container):
```bash
RPS_BENCH=1 docker compose up --build -d
docker compose logs app -f
# wait for "Benchmark complete", then:
docker compose down -v
```

## Benchmark (wrk -t10 -c1000 -d30s, 1 measure run)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| List (GET /api/v1/products?size=20, PG keyset cursor, 50 seeded rows) | 37,483 | 35,609–39,357 | 33,735–41,231 |
| Create (POST /api/v1/products, PG insert + NATS event) | 17,895 | 17,000–18,790 | 16,106–19,685 |

## Limitations

- **List uses keyset pagination** (`pagination: keyset`, `page_size: 20`). Keyset uses `WHERE id > $1 LIMIT N` — O(log N), no `SELECT COUNT(*)`. The response includes `nextCursor` instead of `page`/`total`. Perfect for large tables but does not support random page access.
- **List benchmark** (24,844 RPS) with 50 seeded rows. Without keyset, `SELECT COUNT(*)` on 1M rows would take ~60ms.
- **Create** (17,895 RPS) — PG INSERT via PgDog (pool=500), plus NATS JetStream publish. Comparable to 200 NATS create (~16–19k RPS).
- **Benchmark order:** list runs BEFORE create to keep the table small for comparable results with 200 pg (23,071 RPS on 200 rows).
- **Prefork off:** Single process only — NATS consumers would need dedup across forks.

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | Entry point, wraps with CRUD + file + exit workers |
| `models/model.go` | Product struct with `db:""` + `json:""` tags |
| `models/hooks.go` | BeforeCreate/BeforeDelete hooks for media management |
| `service.yaml` | Config: PG + NATS + S3 + cache + presign + OpenAPI |
| `bench_test.go` | Functional tests: CRUD flow, upload, product+media |
| `create.lua` | wrk script for create benchmark |
| `list.lua` | wrk script for list benchmark |
| `run.sh` | Entrypoint: functional tests + optional RPS bench |
| `run-test-logic.sh` | Test logic helper |
| `Dockerfile` | Multi-stage build |
| `pgdog.toml` | PgDog pooler config (transaction mode, pool=20) |
| `users.toml` | PgDog user credentials |
| `docker-compose.yml` | App + PgDog + PostgreSQL + NATS + MinIO + bucket init |
