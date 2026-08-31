FROM golang:1.26-alpine AS builder
WORKDIR /build
ARG SDK_VERSION=v0.24.0
COPY sdk-api/examples/300-file-storage/s3 ./
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:3.21
RUN apk add --no-cache wrk ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/300-file-storage/s3/*.lua /app/
COPY sdk-api/examples/300-file-storage/s3/run.sh /app/run.sh
COPY sdk-api/examples/300-file-storage/s3/run-test-logic.sh /app/run-test-logic.sh
COPY sdk-api/examples/300-file-storage/s3/service.yaml /app/service.yaml
RUN chmod +x /app/run.sh /app/run-test-logic.sh /app/svc /app/tester
EXPOSE 23305
ENTRYPOINT ["/app/run.sh"]
