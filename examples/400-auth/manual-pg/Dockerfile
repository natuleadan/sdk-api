FROM golang:1.26-alpine AS builder
WORKDIR /build
ARG SDK_VERSION=v0.18.2
COPY sdk-api/examples/400-auth/manual-pg ./
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:3.21
RUN apk add --no-cache wrk ca-certificates curl openssl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/400-auth/manual-pg/run.sh /app/run.sh
COPY sdk-api/examples/400-auth/manual-pg/service.yaml /app/service.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23400
ENTRYPOINT ["/app/run.sh"]
