# 300-file-storage-cached

S3-backed file storage with L1 RAM cache and L2 disk cache. Uploads write through to S3 asynchronously. Downloads hit RAM first, then disk, then S3 — hot keys are ~6× faster than proxy-only. Demonstrates SDK `type: file` with `mode: s3`, `pool` tuning, and `cache: l1 + l2` with async write-through.

**Stack:** Fiber + MinIO (S3-compatible) via SDK `type: file` + RAM cache + disk cache (async write-through).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | `10124` | HTTP port |
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
      pool:
        max_idle_conns: 200
        max_idle_conns_per_host: 100
        max_conns_per_host: 250
      cache:
        l1: ram
        l1_ttl: 5m
        l1_size: 10000
        l2: disk
        l2_path: /data/cache
  - type: rest
    method: GET
    path: /files/download/:key
    handler: onDownloadCached
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
| Upload (proxy PUT, async write-through) | 1,917 | 1,821–2,013 | 1,725–2,109 |
| Download (RAM cache hit, L1) | 133,794 | 127,104–140,484 | 120,415–147,173 |

## Limitations

- **Cache warming:** First request after start misses cache (hits S3). Only hot keys benefit.
- **Upload penalty:** Async write-through adds overhead vs no-cache proxy (~1,917 vs ~2,200 RPS).
- **RAM bound:** L1 cache size is 10,000 entries. Beyond that, entries are evicted.
- **Prefork incompatible:** Each prefork process has its own RAM cache — use NATS KV for shared cache.

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | Entry point, registers file + cached download handler |
| `service.yaml` | Config: S3, pool tuning, L1 RAM + L2 disk cache |
| `bench_test.go` | Functional tests (including CacheHitRAM) |
| `upload.lua` | wrk script for upload benchmark |
| `download.lua` | wrk script for cached download benchmark |
| `run.sh` | Entrypoint: functional tests + optional RPS bench |
| `run-test-logic.sh` | Test logic helper |
| `Dockerfile` | Multi-stage build |
| `docker-compose.yml` | App + MinIO + bucket initializer + cache volume |
