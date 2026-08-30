FROM golang:1.26 AS builder
WORKDIR /build
ARG SDK_VERSION=v0.20.0
COPY . .
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN CGO_ENABLED=1 go build -tags golibsql -o /svc ./cmd/
RUN CGO_ENABLED=1 go test -tags golibsql -c -o /tester . 2>/dev/null; true

FROM debian:trixie-slim
RUN apt-get update -qq && apt-get install -y -qq ca-certificates curl wrk 2>/dev/null && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY *.lua /app/
COPY run.sh /app/run.sh
COPY run-test-logic.sh /app/run-test-logic.sh
COPY service.docker.yaml /app/service.docker.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23211
ENTRYPOINT ["/app/run.sh"]
