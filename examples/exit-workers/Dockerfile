FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/exit-workers ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc .

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/exit-workers/service.yaml /app/service.yaml
EXPOSE 18091
CMD ["/app/svc"]
