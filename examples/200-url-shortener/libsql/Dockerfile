FROM golang:1.26 AS builder
WORKDIR /build
ARG SDK_VERSION=v0.24.0
COPY sdk-api/examples/200-url-shortener/libsql ./
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester . 2>/dev/null; true

FROM debian:bookworm-slim
RUN apt-get update -qq && apt-get install -y -qq ca-certificates curl wrk 2>/dev/null && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/200-url-shortener/libsql/*.lua /app/
COPY sdk-api/examples/200-url-shortener/libsql/run.sh /app/run.sh
COPY sdk-api/examples/200-url-shortener/libsql/run-test-logic.sh /app/run-test-logic.sh
COPY sdk-api/examples/200-url-shortener/libsql/service.docker.yaml /app/service.docker.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23210
ENTRYPOINT ["/app/run.sh"]
