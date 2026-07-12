FROM golang:1.26 AS builder
WORKDIR /build
COPY sdk-api sdk-api
COPY sdk-api/examples/200-url-shortener/sqlite ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=./sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=1 go build -o /svc .
RUN CGO_ENABLED=1 go test -c -o /tester . 2>/dev/null; true
RUN go run -tags tursoinit turso-init.go "file:///tmp/cache.db" && rm -f /tmp/cache.db && \
    ls /root/.cache/turso-go/*/libturso_sync_sdk_kit.so && echo "CACHE OK"

FROM debian:bookworm-slim
RUN apt-get update -qq && apt-get install -y -qq ca-certificates curl wrk 2>/dev/null && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY --from=builder /root/.cache/turso-go /root/.cache/turso-go
COPY sdk-api/examples/200-url-shortener/sqlite/*.lua /app/
COPY sdk-api/examples/200-url-shortener/sqlite/run.sh /app/run.sh
COPY sdk-api/examples/200-url-shortener/sqlite/run-test-logic.sh /app/run-test-logic.sh
COPY sdk-api/examples/200-url-shortener/sqlite/service.docker.yaml /app/service.docker.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23208
ENTRYPOINT ["/app/run.sh"]
