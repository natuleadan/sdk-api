# 300-file-storage-s3

S3-backed file storage with proxy upload, presigned download redirect, and sign-only JSON endpoint. Demonstrates SDK `type: file` with `mode: s3`, `presign: true`, and S3 HTTP connection pool tuning.

Three download modes:
- **Proxy** — server reads from S3 and streams to client
- **Presign redirect (302)** — server returns a signed S3 URL, client downloads directly
- **Sign-only JSON** — server returns signed URL as JSON without redirecting

**Stack:** Fiber + MinIO (S3-compatible) via SDK `type: file` + `Presigner` interface.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | `10123` | HTTP port |
| `S3_ENDPOINT` | `http://minio:9000` | MinIO endpoint |
| `S3_BUCKET` | `dummy-bucket` | S3 bucket name |
| `S3_ACCESS_KEY` | `minioadmin` | MinIO access key |
| `S3_SECRET_KEY` | `minioadmin` | MinIO secret key |

YAML:
```yaml
entry:
  - type: file
    method: POST
    path: /files/upload/:key
    handler: onUpload
    storage:
      mode: s3
      endpoint: "${S3_ENDPOINT}"
      bucket: "${S3_BUCKET}"
      region: us-east-1
      access_key: "${S3_ACCESS_KEY}"
      secret_key: "${S3_SECRET_KEY}"
      presign: true
      presign_ttl: 5m
  - type: rest
    method: GET
    path: /files/download/:key
    handler: onDownloadProxy
  - type: rest
    method: GET
    path: /files/presign/:key
    handler: onDownloadPresign
  - type: rest
    method: GET
    path: /files/sign/:key
    handler: onSignOnly
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
| Upload (proxy PUT to S3) | 2,208 | 2,098–2,318 | 1,987–2,429 |
| Download (proxy GET from S3) | 21,759 | 20,671–22,847 | 19,583–23,935 |
| Sign-only (JSON signed URL) | 64,352 | 61,134–67,570 | 57,917–70,787 |

## Limitations

- **Proxy upload still blocking:** Upload bytes flow through server to S3.
- **Presign redirect skips server bandwidth** but requires client S3 access.
- **Sign-only is fastest** — no data transfer, just URL signing (~64k RPS).
- **No caching:** Every proxy download hits S3.

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | Entry point, registers file + 3 REST handlers |
| `service.yaml` | Config: S3, pool tuning, presign enabled |
| `bench_test.go` | Functional tests (including PresignRedirect + SignOnlyJSON) |
| `upload.lua` | wrk script for upload benchmark |
| `download.lua` | wrk script for proxy download benchmark |
| `sign.lua` | wrk script for sign-only JSON benchmark |
| `run.sh` | Entrypoint: functional tests + optional RPS bench (3 endpoints) |
| `run-test-logic.sh` | Test logic helper |
| `Dockerfile` | Multi-stage build |
| `docker-compose.yml` | App + MinIO + bucket initializer |
