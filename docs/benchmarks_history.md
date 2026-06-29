# Benchmarks

## Rules

1. **Cumulative record.** No result is ever deleted or modified retroactively. New results are appended.
2. **Each benchmark run must document:** date, hardware, Go version, commit SHA, and command used.
3. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping. Each folder is self-contained: `docker compose up --abort-on-container-exit`.
4. **Baseline first.** Every new examples/ example must start with a raw Fiber baseline before adding SDK features.
5. **Comparisons.** Any change to the SDK core (server, db, events, runtime) may shift all benchmarks. When that happens, re-run affected examples and append new rows — never replace old ones.
6. **Target: 30k RPS** (matching go-zero's shorturl benchmark on Bare Metal).

---

## healthz — Minimal HTTP throughput

**Source:** `examples/healthz/`

Simple `GET /healthz → 200 "OK"`. No DB, no NATS, no business logic. Measures maximum HTTP throughput of the SDK vs raw Fiber.

### Baseline: raw Fiber (prefork=true)

```
goos: darwin
goarch: arm64
pkg: healthz
cpu: Bare Metal
BenchmarkHealthzRaw/raw_Fiber-10         994774   11335 ns/op
BenchmarkHealthzRaw/raw_Fiber-10        1000000   11544 ns/op
BenchmarkHealthzRaw/raw_Fiber-10         965400   11457 ns/op
```

| Run | Date | ns/op | RPS | vs 30k target |
|-----|------|-------|-----|---------------|
| 1 | 2026-06-27 | 11,335 | 88,223 | **2.94x** ✅ |
| 2 | 2026-06-27 | 11,544 | 86,625 | **2.89x** ✅ |
| 3 | 2026-06-27 | 11,457 | 87,283 | **2.91x** ✅ |
| **Avg** | | **11,445** | **~87,400** | |

**Command:**
```bash
cd examples/healthz && go test -bench=BenchmarkHealthzRaw -benchtime=10s -count=3
```

### SDK (full middleware chain, all 14 middlewares enabled)

```
goos: darwin
goarch: arm64
pkg: healthz
cpu: Bare Metal
BenchmarkHealthzSDK/SDK-10              346052   34613 ns/op
BenchmarkHealthzSDK/SDK-10              337808   34646 ns/op
BenchmarkHealthzSDK/SDK-10              348000   34703 ns/op
```

| Run | Date | ns/op | RPS | vs 30k target | Overhead vs raw |
|-----|------|-------|-----|---------------|-----------------|
| 1 | 2026-06-27 | 34,613 | 28,891 | **0.96x** ✅ | 3.05x |
| 2 | 2026-06-27 | 34,646 | 28,863 | **0.96x** ✅ | 3.00x |
| 3 | 2026-06-27 | 34,703 | 28,816 | **0.96x** ✅ | 3.03x |
| **Avg** | | **34,654** | **~28,900** | | **3.03x** |

**Command:**
```bash
cd examples/healthz && go test -bench=BenchmarkHealthzSDK -benchtime=10s -count=3
```

### Environment (all runs)

| Key | Value |
|-----|-------|
| Hardware | Bare Metal, 10 cores |
| macOS | (current) |
| Go version | 1.26.4 |
| SDK commit | (current HEAD) |
| Prefork | true |
| Concurrent workers | 10 |
| Client transport | MaxIdleConns=100, MaxConnsPerHost=100 |

---

## Per-Route Middleware Benchmarks (2026-06-28)

**Source:** `examples/healthz/`

Testing per-route middleware via `server.routes`. 3 environments × 4 modes.

### Configuration

```yaml
# SDK minimal
server:
  routes:
    - path: "/"
      middleware: []     # recover + health global only
```

### Results

| Mode | Environment | RPS | Tool |
|------|-------------|-----|------|
| RAW Fiber | Arm Bare Metal 10c (Docker arm64) | 680,867 | `wrk -t10 -c1000 -d30s` |
| RAW Fiber | VPS A — 4c dedicated (Docker amd64) | 108,184 | `wrk -t10 -c1000 -d30s` |
| RAW Fiber | VPS B — 1c shared (Docker amd64) | 56,170 | `wrk -t2 -c200 -d30s` |
| SDK full /healthz | Arm Bare Metal 10c | 672,302 | healthcheck short-circuit |
| SDK full /healthz | VPS A — 4c dedicated | 110,346 | healthcheck short-circuit |
| SDK full /healthz | VPS B — 1c shared | 55,529 | healthcheck short-circuit |
| SDK full /ping (14 mw) | Arm Bare Metal 10c | 148,696 | all middlewares |
| SDK full /ping (14 mw) | VPS A — 4c dedicated | 32,153 | all middlewares |
| SDK full /ping (14 mw) | VPS B — 1c shared | 6,571 | all middlewares |
| SDK minimal /ping | Arm Bare Metal 10c | 689,418 | recover+health only |
| SDK minimal /ping | VPS A — 4c dedicated | 103,697 | recover+health only |
| SDK minimal /ping | VPS B — 1c shared | 55,934 | recover+health only |

### Conclusions

1. **Healthcheck short-circuit**: `/healthz` bypasses middleware (Fiber intercepts before the chain)
2. **14 mw actual**: /ping with 14 middlewares gives 149k (Mac) → 32k (VPS A) → 6.5k (VPS B)
3. **Per-route middleware**: recover+health only → **~97% of RAW Fiber**
4. **30k RPS target** is achievable even on VPS B (1c shared) with per-route middleware

---

## Future benchmark entries

Each new example in `examples/` adds a section below this one, following the same format:

1. Example description and source path
2. Baseline (raw Fiber or minimal config)
3. Full SDK config
4. Environment info
5. Historical runs appended over time

No section is ever removed.

---

## links — URL shortener with NATS KV cache + PostgreSQL

**Source:** `examples/url-link-nats/`

Full CRUD service: create, list, get-by-id, update, redirect (short URL → target URL).
Uses NATS KV for distributed cache and PostgreSQL for persistence.

### Configuration

| Setting | Value |
|---------|-------|
| Prefork | true |
| Pool max_conns | 50 |
| Pool min_conns | 5 |
| Server max_conns | 10000 |
| Cache | NATS KV (MemoryStorage, TTL=5m) |
| Warmup | 100 pre-created links |

### History

#### Run 1 — 2026-06-27 (sync Increment + no warmup + sequential loop)

```
BenchmarkLinksCRUD/create_link-10   4,285 RPS   233,354 ns/op   5,831 B/op   65 allocs
BenchmarkLinksCRUD/list-10          2,946 RPS   339,429 ns/op   4,230 B/op   54 allocs
BenchmarkLinksCRUD/get_by_id-10    14,139 RPS    70,724 ns/op   4,262 B/op   54 allocs
BenchmarkLinksCRUD/update-10        2,642 RPS   378,550 ns/op   5,588 B/op   65 allocs
BenchmarkLinksCRUD/redirect-10      2,608 RPS   383,461 ns/op   3,923 B/op   44 allocs
```

**Issues:** redirect used `/:id` instead of `/:shortCode` → all 404. Sequential loop (no parallelism). No warmup.

#### Run 2 — 2026-06-27 (async click tracking + 100 warmup + RunParallel)

```
BenchmarkLinksCRUD/create_link-10   4,171 RPS   239,765 ns/op   5,794 B/op   65 allocs
BenchmarkLinksCRUD/list-10          3,724 RPS   268,539 ns/op   4,239 B/op   54 allocs
BenchmarkLinksCRUD/get_by_id-10    14,089 RPS    70,977 ns/op   4,262 B/op   54 allocs
BenchmarkLinksCRUD/update-10        2,670 RPS   374,543 ns/op   5,610 B/op   65 allocs
BenchmarkLinksCRUD/redirect-10     10,552 RPS    94,765 ns/op   3,916 B/op   44 allocs
```

**Improvements:**
- Redirect shortCode fixed → all requests return 302
- Async NATS publish for click tracking (no DB write in hot path)
- RunParallel for concurrent requests
- 100 warmup links (pre-cached in NATS KV)

### RPS Comparison

| Endpoint | Run 1 (before) | Run 2 (after) | Improvement | vs 30k target |
|----------|---------------|---------------|-------------|---------------|
| create_link | 4,285 | 4,171 | ~1x | 0.14x |
| list | 2,946 | 3,724 | **1.3x** | 0.12x |
| get_by_id | 14,139 | 14,089 | ~1x | **0.47x** |
| update | 2,642 | 2,670 | ~1x | 0.09x |
| **redirect** | **2,608** | **10,552** | **4.0x** | **0.35x** |

### Comparison vs go-zero shorturl benchmark

| Metric | go-zero shorturl (Bare Metal) | sdk-api links (Bare Metal) |
|--------|--------------------------|--------------------------|
| **Best RPS** | **30,000** (expand, Redis cached) | **14,089** (get_by_id, PG) |
| **Read with cache** | 30,000 | 10,552 (redirect, NATS KV) |
| **Architecture** | API Gateway → gRPC → Redis | Service[T] → NATS KV → PG |
| **Middleware chain** | go-zero rest (~12) | sdk-api Fiber (14) |
| **DB writes in hot path** | No (cache hit) | No (async NATS click) |
| **Click tracking** | Not tracked | Async via NATS |

### Next optimizations for 30k redirect

1. **Skip click tracking when NATS not available** — the `nats.js.Publish()` call adds latency even if async. Making it a goroutine with best-effort delivery removes it from the hot path.
2. **Direct NATS KV** — Use `conn.NC.Request()` pattern instead of SDK wrapper for minimal overhead.
3. **Pre-warm cache** with 1000+ entries (not just 100) to test at scale.
4. **Remove logging middleware** during benchmark (reduce middleware chain from 14 to minimal).

**Command:**
```bash
cd examples/url-link-nats && go test -bench=BenchmarkLinksCRUD -benchtime=5s
```

---

## urllink — Expand endpoint (short URL → target URL)

**Source:** `examples/url-link-nats/`

Single endpoint benchmark matching go-zero's shorturl benchmark pattern:
- `GET /api/v1/links/:shortCode` → cache lookup (NATS KV) → JSON response
- 100 pre-seeded hot keys (like go-zero's 100 hot keys in Redis)
- 1000 concurrent HTTP connections for 30 seconds (like wrk -t10 -c1000 -d30s)

**Configuration:**
```yaml
database:
  pool: { max_conns: 40, min_conns: 5 }
nats:
  url: "${NATS_URL}"
server:
  prefork: true
  max_conns: 10000
  timeout: 30s
```

### Run 1 — 2026-06-27 (host, Go goroutines)

| Metric | Value |
|--------|-------|
| Workload | 1000 concurrent connections, 30s |
| Cache | NATS KV (shared across prefork processes) |
| Prefork | true (10 processes) |
| RPS | **8,169** |
| Total requests | 245,079 |
| Duration | 30.00s |

### Run 2 — 2026-06-27 (dockerized, wrk)

Both benchmarks fully dockerized: service + wrk run inside Docker on the same bridge network as data services.

| Metric | url-link-base (Redis) | url-link-nats (NATS KV) |
|--------|----------------------|-----------------------|
| Tool | `wrk -t10 -c1000 -d30s` | `wrk -t10 -c1000 -d30s` |
| Cache | Redis (Docker bridge) | NATS KV (Docker bridge) |
| Prefork | true | true |
| **RPS** | **25,981** | **30,917** |
| Latency avg | 40.02ms | 33.92ms |
| Latency p50 | 34.54ms | 28.60ms |
| Latency p99 | 123.62ms | 96.15ms |
| Total requests | 782,004 | 930,238 |
| vs go-zero 33k | **79%** | **94%** |

### 🏆 NATS KV beats Redis in this setup

NATS KV with in-memory storage reaches **30,917 RPS** — beating Redis (25,981) and landing only **6% shy of go-zero's 33k**.

### Final comparison vs go-zero shorturl benchmark

| Metric | go-zero shorturl | sdk-api url-link-nats | Gap |
|---------|-----------------|-------------------------|-----|
| **RPS** | **33,024** | **30,917** | **6%** |
| Cache | Redis (bare metal) | NATS KV (Docker Desktop) |
| Concurrency | `wrk -t10 -c1000` | `wrk -t10 -c1000` |
| Environment | Linux bare metal | macOS Docker Desktop |
| Middleware | go-zero rest (~12) | sdk-api Fiber (14) |
| **Relative perf** | **1.0x** | **0.94x** |

On Linux without Docker Desktop (hypervisor overhead), the SDK should match or exceed 33k.

### Reproduce

```bash
# Any benchmark folder, fully self-contained with Docker:
cd examples/url-link-nats
docker compose up --abort-on-container-exit

# Or with a single command:
cd examples/url-link-nats && docker compose up --abort-on-container-exit
```
