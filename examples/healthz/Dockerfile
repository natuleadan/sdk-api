FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/healthz ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/healthz/healthz.lua /app/healthz.lua
COPY sdk-api/examples/healthz/run.sh /app/run.sh
RUN chmod +x /app/run.sh /app/svc
EXPOSE 18081
CMD ["/app/run.sh"]
