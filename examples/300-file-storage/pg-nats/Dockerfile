FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api sdk-api
COPY sdk-api/examples/300-file-storage/pg-nats ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=./sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/300-file-storage/pg-nats/*.lua /app/
COPY sdk-api/examples/300-file-storage/pg-nats/run.sh /app/run.sh
COPY sdk-api/examples/300-file-storage/pg-nats/run-test-logic.sh /app/run-test-logic.sh
COPY sdk-api/examples/300-file-storage/pg-nats/service.yaml /app/service.yaml
RUN chmod +x /app/run.sh /app/run-test-logic.sh /app/svc /app/tester
EXPOSE 23304
ENTRYPOINT ["/app/run.sh"]
