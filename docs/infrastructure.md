# Infrastructure Packages

45+ production-tested packages from go-zero, available under `infra/`.

## Configuration

| Package | Path | Purpose |
|---------|------|---------|
| conf | `infra/conf` | YAML/JSON/TOML loading with UseEnv() |
| mapping | `infra/mapping` | Reflective struct-to-map mapping |

## Logging

| Package | Path | Purpose |
|---------|------|---------|
| logx | `infra/logx` | Structured logging (levels, context) |
| logc | `infra/logc` | Log configuration |

## Observability

| Package | Path | Purpose |
|---------|------|---------|
| trace | `infra/trace` | OpenTelemetry tracing agent |
| metric | `infra/metric` | Prometheus counters, gauges, histograms |
| prometheus | `infra/prometheus` | Prometheus agent config |
| stat | `infra/stat` | System metrics, CPU, alerting |

## Resilience

| Package | Path | Purpose |
|---------|------|---------|
| breaker | `infra/breaker` | Circuit breaker (Google SRE-style) |
| load | `infra/load` | Adaptive CPU-based load shedding |
| limit | `infra/limit` | Token bucket rate limiter |

## Process Lifecycle

| Package | Path | Purpose |
|---------|------|---------|
| proc | `infra/proc` | Signal handling, graceful shutdown |
| service | `infra/service` | ServiceGroup lifecycle |
| rescue | `infra/rescue` | Panic recovery |

## Data Structures

| Package | Path | Purpose |
|---------|------|---------|
| collection | `infra/collection` | TimingWheel, Bloom filter, Cache, FIFO, Ring, RollingWindow |
| bloom | `infra/bloom` | Bloom filter |
| syncx | `infra/syncx` | AtomicBool, SpinLock, Once, Pool, DoneChan |
| threading | `infra/threading` | RoutineGroup, WorkerGroup, StableRunner |
| timex | `infra/timex` | Ticker, Repr, RelativeTime |

## Functional Programming

| Package | Path | Purpose |
|---------|------|---------|
| fx | `infra/fx` | Map/Reduce/Filter |
| mr | `infra/mr` | MapReduce pattern |
| errorx | `infra/errorx` | BatchError, code-based errors |

## Cryptography

| Package | Path | Purpose |
|---------|------|---------|
| codec | `infra/codec` | RSA, AES, DH encryption |

## Service Discovery

| Package | Path | Purpose |
|---------|------|---------|
| discov | `infra/discov` | etcd service discovery (publisher/subscriber) |
| configcenter | `infra/configcenter` | etcd-backed dynamic config hot-reload |

## Storage Clients

| Package | Path | Purpose |
|---------|------|---------|
| stores/redis | `infra/stores/redis` | Full Redis client (cluster, node, sentinel, locks, metrics) |
| stores/kv | `infra/stores/kv` | KV store over Redis with consistent hash |
| stores/cache | `infra/stores/cache` | Cache abstraction (Redis cluster/node) |
| stores/etcd | `infra/stores/etcd` | etcd KV store (Get/Put/Delete/Keys) |
| stores/mon | `infra/stores/mon` | MongoDB model with CRUD |
| stores/monc | `infra/stores/monc` | Cached MongoDB (Mongo + Redis) |
| stores/builder | `infra/stores/builder` | SQL query builder |
| stores/dbtest | `infra/stores/dbtest` | Database test helpers |

## Utilities

| Package | Path | Purpose |
|---------|------|---------|
| stringx | `infra/stringx` | String utilities |
| mathx | `infra/mathx` | Math utilities, entropy |
| jsonx | `infra/jsonx` | JSON utilities |
| iox | `infra/iox` | BufferPool, TeeReader, Pipe |
| fs | `infra/fs` | Filesystem utilities |
| filex | `infra/filex` | File operations |
| netx | `infra/netx` | Network utilities |
| hash | `infra/hash` | Consistent hashing |
| search | `infra/search` | Trie-based search tree |
| lang | `infra/lang` | Language utilities (repr) |
| naming | `infra/naming` | Naming conventions |
| color | `infra/color` | Terminal coloring |
| sysx | `infra/sysx` | Host info, automaxprocs |
| contextx | `infra/contextx` | Context utilities |
| utils | `infra/utils` | General utilities |
| validation | `infra/validation` | Validator interface |
| prof | `infra/prof` | Profiling |
| executors | `infra/executors` | Periodic/bulk/chunk/delay executors |
| cmdline | `infra/cmdline` | Command-line input |
