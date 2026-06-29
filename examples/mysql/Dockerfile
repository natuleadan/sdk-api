FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/mysql/go.mod sdk-api/examples/mysql/go.sum ./
RUN go mod download
COPY sdk-api/examples/mysql/ ./
RUN CGO_ENABLED=0 go build -o /svc .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl mariadb-client
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/mysql/get.lua /app/get.lua
COPY sdk-api/examples/mysql/run.sh /app/run.sh
COPY sdk-api/examples/mysql/service.docker.yaml /app/service.docker.yaml
RUN chmod +x /app/run.sh /app/svc
EXPOSE 18085
CMD ["/app/run.sh"]
