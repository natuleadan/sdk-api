# 300-file-storage-ephemeral

In-memory file storage via Go `map[string][]byte` — no disk, no external service. Files exist only while the process runs. Demonstrates raw Fiber throughput without I/O bottlenecks. Uses SDK `type: rest` — manual handler registration.

**Stack:** Fiber + Go map (in-memory).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `PORT` | `10121` | HTTP port |

YAML:
```yaml
entry:
  - type: rest
    method: POST
    path: /files/upload/:key
    handler: onUpload
  - type: rest
    method: GET
    path: /files/download/:key
    handler: onDownload
  - type: rest
    method: GET
    path: /files
    handler: onList
  - type: rest
    method: DELETE
    path: /files/:key
    handler: onDelete
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
| Upload (POST /api/v1/files/upload/:key) | 38,922 | 36,976–40,868 | 35,030–42,814 |
| Download (GET /api/v1/files/download/:key) | 81,577 | 77,498–85,656 | 73,419–89,735 |

## Limitations

- **Ephemeral:** All data is lost on restart. No persistence.
- **Single process:** Shared mutable map — not suitable for multi-process or prefork.
- **No size limit:** Large uploads consume RAM without eviction.
- **Memory pressure:** `map` grows unbounded; no TTL or LRU.

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | Entry point, registers 4 REST handlers |
| `service.yaml` | Config: no DB, no storage backend |
| `bench_test.go` | Functional tests |
| `upload.lua` | wrk script for upload benchmark |
| `download.lua` | wrk script for download benchmark |
| `run.sh` | Entrypoint: functional tests + optional RPS bench |
| `run-test-logic.sh` | Test logic helper |
| `Dockerfile` | Multi-stage build |
| `docker-compose.yml` | Single service, no dependencies |
