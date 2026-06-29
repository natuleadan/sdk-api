FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/url-link-nats ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/url-link-nats/expand.lua /app/expand.lua
COPY sdk-api/examples/url-link-nats/run.sh /app/run.sh
COPY sdk-api/examples/url-link-nats/service.docker.yaml /app/service.docker.yaml
RUN chmod +x /app/run.sh /app/svc
EXPOSE 18084
CMD ["/app/run.sh"]
