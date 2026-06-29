FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/mongo/go.mod sdk-api/examples/mongo/go.sum ./
RUN go mod download
COPY sdk-api/examples/mongo/ ./
RUN CGO_ENABLED=0 go build -o /svc .

FROM alpine:latest
RUN apk add --no-cache wrk ca-certificates curl mongodb-tools
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/mongo/get.lua /app/get.lua
COPY sdk-api/examples/mongo/run.sh /app/run.sh
RUN chmod +x /app/run.sh /app/svc
EXPOSE 18087
CMD ["/app/run.sh"]
