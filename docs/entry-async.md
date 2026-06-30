# Async Job Entry Type

The `type: async` entry provides a pattern for long-running tasks with `202 Accepted` submission and status polling.

## Configuration

```yaml
entry:
  - type: async
    path: /jobs/reports
    handler: processReport
```

## Endpoints

| Method | Path | Response |
|--------|------|----------|
| POST | `/path` | `202 Accepted` + `job_id` + `status_url` |
| GET | `/path/:job_id` | `200 OK` + `JobState` JSON |

## Handler

```go
svc.WithAsync("processReport", func(body []byte, job *runtime.JobState) error {
    // Process the job (runs in a goroutine)
    result := generateReport(body)
    job.Result = fiber.Map{
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
```

## Use Cases

- Report generation (PDF, CSV, XLSX)
- Data export/import
- Image/video processing
- Batch email sending
- AI inference pipelines
