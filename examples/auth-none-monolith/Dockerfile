FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/auth-none-monolith /example
WORKDIR /example
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy
RUN CGO_ENABLED=0 go build -o /svc .
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl wrk
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/auth-none-monolith/service.yaml /app/
COPY sdk-api/examples/auth-none-monolith/get.lua /app/
COPY sdk-api/examples/auth-none-monolith/post.lua /app/
COPY sdk-api/examples/auth-none-monolith/run.sh /app/
RUN chmod +x /app/svc /app/tester /app/run.sh
EXPOSE 13000
CMD ["/app/run.sh"]
