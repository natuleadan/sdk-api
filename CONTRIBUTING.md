# Contributing

Thanks for your interest in contributing to Natuleadan's SDK API!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/natuleadan/sdk-api.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run tests: `go test ./...`
6. Push and open a Pull Request

## Conventional Commits

Every commit must follow `type(scope): description` with a **required scope**.

```
feat(api): add pagination to list endpoint
fix(db): correct null pointer in product query
```

See [docs/conventional-commits.md](docs/conventional-commits.md) for the full reference.

## Development Setup

```bash
# Run unit tests (no Docker needed)
go test -short ./...

# Run integration tests (requires Docker + NATS)
docker compose -f docker-compose.test.yml up -d
go test ./...

# All tests
go test -short ./...
```

## Code Style

- Run `go fmt ./...` before committing
- Run `go vet ./...` — no warnings allowed
- Keep tests passing: `go test ./...`
- Use `goccy/go-json` instead of `encoding/json` in new code

## Pull Request Checklist

- [ ] Code compiles: `go build ./...`
- [ ] Lint passes: `go vet ./...`
- [ ] Tests pass: `go test ./...`
- [ ] New code has tests
- [ ] New features have YAML configuration
