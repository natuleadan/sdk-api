# Async Job Entry Type

The `type: async` entry provides a pattern for long-running tasks with `202 Accepted` submission and status polling.

## Configuration

```yaml
entry:
  - type: async
    path: /jobs/reports
    handler: processReport
```

### Persistent Store Configuration

By default, jobs are stored in memory and lost on process restart. Use `async_store` to persist jobs across restarts and share state between instances:

```yaml
entry:
  - type: async
    path: /jobs/reports
    handler: processReport
    async_store:
      driver: postgres
      db: pg-main
      table: async_jobs
      result_ttl: 24h
      reassign:
        enabled: true
        processing_timeout: 5m
        reap_interval: 30s
        max_retries: 3
      callback:
        url: "${CALLBACK_URL}"
        secret: "${CALLBACK_SECRET}"
        retry: 3
        retry_delay: 5s
    max_concurrent: 10
```

| Field | Description |
|-------|-------------|
| `driver` | Backend: `memory` (default), `postgres`, `redis`, `nats_kv` |
| `db` | Database reference (required for `driver: postgres`) |
| `kv` | KV store reference (required for `driver: redis`) |
| `stream` | Stream reference (required for `driver: nats_kv`) |
| `bucket` | NATS KV bucket name (default `async-jobs`, `driver: nats_kv`) |
| `table` | PostgreSQL table name (default `async_jobs`, `driver: postgres`) |
| `result_ttl` | Auto-cleanup completed/failed jobs after this duration (e.g. `24h`, `0` = keep forever) |
| `reassign.enabled` | Enable the reaper to recover stuck jobs |
| `reassign.processing_timeout` | Max time a job can stay in `processing` before reassign (default `5m`) |
| `reassign.reap_interval` | How often the reaper checks for stale jobs (default `30s`) |
| `reassign.max_retries` | Max retries before moving to `failed` (default `3`) |
| `callback.url` | Webhook URL called on job completion or failure |
| `callback.secret` | HMAC-SHA256 key for signing the callback payload |
| `callback.retry` | Number of retry attempts if the callback fails |
| `callback.retry_delay` | Delay between retry attempts |
| `max_concurrent` | Limit simultaneous job processing goroutines (`0` = unlimited) |
| `event_publish` | Publish to NATS/Kafka on job submission (see `docs/configuration.md`) |

### Drivers

| Driver | Backend | Persistence | Multi-instance |
|--------|---------|-------------|----------------|
| `memory` | In-process RAM | Lost on restart | No |
| `postgres` | PostgreSQL table | Survives restart | Yes (shared DB) |
| `redis` | Redis/Dragonfly key-value | Survives restart | Yes (shared Redis) |
| `nats_kv` | NATS KV bucket | Survives restart | Yes (shared NATS) |

### Reaper (Automatic Job Recovery)

When `reassign.enabled: true`, a background goroutine periodically scans for jobs stuck in `processing` status (e.g., after a process crash). Jobs with an expired `processing_deadline` are reset to `pending` with incremented `retry_count`. Jobs that exceed `max_retries` are moved to `failed`.

The reaper also runs `Cleanup` when `result_ttl` is set, removing completed and failed jobs older than the TTL. This prevents unbounded storage growth in production.

### Callback Webhook

When a job completes or fails, the SDK can POST the job state to a configured URL:

```bash
POST ${callback.url}  {"id": "j_abc123", "status": "completed", "result": {...}}
X-Job-Signature: <HMAC-SHA256 hex encoded>
```

The receiver can verify the signature using `runtime.VerifyCallbackSignature(payload, secret, signature)`.

Per-request callback URL override: include `_callback_url` in the POST body:

```bash
curl -X POST /jobs/reports \
  -d '{"type": "monthly", "_callback_url": "https://myapp.com/cb"}'
```

This overrides the static YAML callback URL for that specific job.

## Endpoints

| Method | Path | Response |
|--------|------|----------|
| POST | `/path` | `202 Accepted` + `job_id` + `status_url` |
| GET | `/path/:job_id` | `200 OK` + `JobState` JSON |
| DELETE | `/path/:job_id` | `204 No Content` (or `409 Conflict` if processing) |
| GET | `/path/:job_id/status` | `200 OK` + SSE stream of status changes |
| GET | `/path` | `200 OK` + list of recent jobs |

DELETE semantics:
- `204` — job was pending/completed/failed and has been removed
- `409` — job is still processing (cannot cancel in-flight work)
- `404` — job ID not found

The SSE endpoint streams `data: <json>\n\n` events. The first event is the current state, followed by updates as the job progresses.

## Handler

```go
svc.WithAsync("processReport", func(body []byte, job *runtime.JobState) error {
    result := generateReport(body)
    job.Result = map[string]any{
        "report_url": "https://example.com/report.pdf",
        "input":      string(body),
    }
    return nil
})
```

## Job States

| State | Description |
|-------|-------------|
| `pending` | Job created, not yet processing |
| `processing` | Handler is running |
| `completed` | Handler returned successfully |
| `failed` | Handler returned an error |

## Submission Flow

```
POST /jobs/reports  {"type": "monthly"}
  → 202 {"job_id": "abc123", "status": "pending", "status_url": "/jobs/reports/abc123"}

GET /jobs/reports/abc123
  → 200 {"id": "abc123", "status": "completed", "result": {"report_url": "..."}, "error": ""}

DELETE /jobs/reports/abc123
  → 204 (cancel if not processing)

GET /jobs/reports/abc123/status
  → data: {"id":"abc123","status":"processing",...}
     data: {"id":"abc123","status":"completed",...}

GET /jobs/reports
  → 200 {"jobs": [...], "total": 5}
```

## JobState Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique job ID (`j_<24-hex>`) |
| `status` | string | `pending`, `processing`, `completed`, `failed` |
| `result` | any | User-defined result (set `job.Result`) |
| `error` | string | Error message if failed |
| `created_at` | timestamp | Job creation time |
| `updated_at` | timestamp | Last status change |
| `retry_count` | int | Number of reassigns (when reaper enabled) |
| `max_retries` | int | Max retries before failure |
| `processing_deadline` | timestamp | Deadline for current processing attempt |
| `callback_url` | string | Per-request callback URL override (set via `_callback_url`) |

## Use Cases

- Report generation (PDF, CSV, XLSX)
- Data export/import
- Image/video processing
- Batch email sending
- AI inference pipelines
