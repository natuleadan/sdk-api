FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/400-auth/manual-pg ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl openssl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/400-auth/manual-pg/run.sh /app/run.sh
COPY sdk-api/examples/400-auth/manual-pg/service.yaml /app/service.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23400
ENTRYPOINT ["/app/run.sh"]
