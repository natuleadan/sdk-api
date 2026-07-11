# 300-file-storage-proxy

S3-backed file storage with proxy upload/download — the server acts as a middleman, streaming data through itself between clients and S3 (MinIO). Demonstrates SDK `type: file` with `mode: s3` and no pool/cache optimization. No presign — all traffic goes through the server.

**Stack:** Fiber + MinIO (S3-compatible) via SDK `type: file`.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | `10122` | HTTP port |
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
  - type: rest
    method: GET
    path: /files/download/:key
    handler: onDownloadProxy
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
| Upload (proxy PUT to S3) | 2,106 | 2,001–2,211 | 1,895–2,317 |
| Download (proxy GET from S3) | 20,801 | 19,761–21,841 | 18,721–22,881 |

## Limitations

- **Proxy bandwidth:** Every byte goes through the server — doubles network cost.
- **No connection pool tuning:** Uses Go default `MaxIdleConnsPerHost=2`.
- **No caching:** Every download hits S3, even for hot keys.
- **No presign:** Cannot offload traffic to clients.

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | Entry point, registers file + REST handlers |
| `service.yaml` | Config: S3 storage, no pool, no cache, no presign |
| `bench_test.go` | Functional tests |
| `upload.lua` | wrk script for upload benchmark |
| `download.lua` | wrk script for download benchmark |
| `run.sh` | Entrypoint: functional tests + optional RPS bench |
| `run-test-logic.sh` | Test logic helper |
| `Dockerfile` | Multi-stage build |
| `docker-compose.yml` | App + MinIO + bucket initializer |
