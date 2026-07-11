# Vercel Deployment

Deploy a project built with sdk-api on Vercel using the Go Framework Preset (server mode).

## Prerequisites

- [Vercel CLI](https://vercel.com/docs/cli) installed
- Vercel account connected
- Project with `go.mod` at root

## How it works

Vercel detects a root `go.mod` and an entrypoint (`main.go`, `cmd/api/main.go`, or `cmd/server/main.go`), builds the Go binary, and runs it. The binary must listen on the `PORT` environment variable assigned by Vercel.

sdk-api handles this automatically:
- `runtime.NewFromYAML()` embeds `service.yaml` in the binary via `//go:embed` — no external file needed at runtime
- `applyEnvOverrides` in `LoadConfig` resolves `PORT` — no code changes needed
- `deploy.target: vercel` validates incompatible settings (prefork, TLS) at startup

## Step-by-step

### 1. Create a project

```bash
sdk-api new my-service --model Product --fields "name:string,price:float64"
```

The generated `main.go` uses `//go:embed` to embed `service.yaml` into the binary:

```go
//go:embed service.yaml
var configYAML []byte

func main() {
    svc, err := runtime.NewFromYAML(configYAML)
    // ...
}
```

### 2. Configure `service.yaml`

Set `deploy.target: vercel` to enable validation:

```yaml
name: my-service
port: 8080
deploy:
  target: vercel
server:
  host: "0.0.0.0"
  prefork: false          # required for Vercel (no SO_REUSEPORT)
  # tls.enabled must be false (Vercel terminates TLS at edge)
```

### 3. Generate `vercel.json`

```bash
sdk-api vercel --output vercel.json
```

Or with custom build flags:

```bash
sdk-api vercel --output vercel.json --go-flags "-ldflags '-s -w'"
```

### 4. Deploy

```bash
vercel deploy --prod
```

## Limitations

| Feature | Vercel support | Notes |
|---------|---------------|-------|
| Server mode (main.go) | ✅ | Full Go binary |
| Serverless functions (api/*.go) | ❌ | Incompatible with Fiber — use server mode |
| TLS | ❌ | Handled by Vercel edge network |
| Prefork | ❌ | SO_REUSEPORT not available |
| Local file storage | ⚠️ | Ephemeral filesystem, OK for reads |
| S3 presigned URLs | ✅ | Redirect-based downloads, zero server bandwidth |
| S3 cached (L2 disk) | ⚠️ | L1 RAM works, L2 disk is ephemeral on Vercel |
| PostgreSQL / Turso / MySQL | ✅ | Via external connection strings |
| NATS / Kafka | ✅ | Via external servers |
| WebSocket | ⚠️ | Vercel supports WebSocket upgrades in server mode |
| Cron jobs (runtime-based) | ❌ | Use Vercel Cron Jobs instead |

## Project structure

```
my-project/
├── go.mod
├── go.sum
├── main.go              # or cmd/api/main.go or cmd/server/main.go
├── service.yaml
├── vercel.json
└── ...
```

## Manual `vercel.json` reference

Minimal:

```json
{
  "framework": "go"
}
```

With custom build:

```json
{
  "framework": "go",
  "build": {
    "env": {
      "GO_BUILD_FLAGS": "-ldflags '-s -w'"
    }
  }
}
```

## Validation

When `deploy.target: vercel` is set in `service.yaml`, the runtime enforces:

- `server.prefork` must be `false` — rejected at startup with a clear error
- `server.tls.enabled` must be `false` — rejected at startup with a clear error
- Local storage on writable paths logs a warning (filesystem is ephemeral on Vercel)
