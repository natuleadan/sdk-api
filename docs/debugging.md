# Debugging with Delve

## Install

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

## Debug the Service

```bash
# Start the service in debug mode
dlv debug ./cmd/sdk-api -- run --config service.yaml

# Start a specific example
dlv debug ./examples/500-tickets/cmd/
```

## Attach to a Running Process

```bash
# Find PID
pgrep -a sdk-api

# Attach
dlv attach $(pgrep sdk-api)
```

## Debug Integration Tests

```bash
# Run a specific test with debugger
dlv test ./runtime -- -test.run TestRegisterEntries_Async -test.v

# With build tags
dlv test ./examples/500-tickets --tags=integration -- -test.run TestAsync_Callback -test.v
```

## Inspect State

```go
// (dlv) break runtime/async_job.go:85
// (dlv) break runtime/entry.go:310   // registerOneEntry
// (dlv) break runtime/exit.go:153    // process message
// (dlv) break server/middleware/jwt.go:30  // JWT validation
// (dlv) break db/table.go:172        // Table.Create
// (dlv) break db/table.go:235        // Table.Update

// View variables
// (dlv) args
// (dlv) locals
// (dlv) p entry
// (dlv) p entry.Path

// Step through
// (dlv) next
// (dlv) step
// (dlv) continue

// Evaluate expressions
// (dlv) call someFunction()
```

## Debug with Environment Variables

```bash
DATABASE_URL="postgres://..." dlv debug ./cmd/
```

## Common Workflows

| Task | Command |
|------|---------|
| Debug startup | `dlv debug ./cmd/ -- break runtime/config.go:1290` |
| Debug a CRUD handler | `dlv debug ./cmd/ -- break runtime/entry_crud.go:147` |
| Debug NATS messages | `dlv debug ./cmd/ -- break runtime/exit.go:124` |
| Debug middleware chain | `dlv debug ./cmd/ -- break server/middleware/jwt.go:55` |
| Debug async job processing | `dlv test ./runtime -- -test.run TestRegisterEntries_Async` |
