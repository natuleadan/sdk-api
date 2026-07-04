FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api /sdk-api

COPY sdk-api/examples/auth-none-microservices/users-service /example-users
WORKDIR /example-users
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy
RUN CGO_ENABLED=0 go build -o /svc-users .

COPY sdk-api/examples/auth-none-microservices/products-service /example-products
WORKDIR /example-products
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy
RUN CGO_ENABLED=0 go build -o /svc-products .

WORKDIR /example-users
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl wrk
WORKDIR /app
RUN mkdir -p /app/users /app/products
COPY --from=builder /svc-users /app/users/svc
COPY --from=builder /svc-products /app/products/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/auth-none-microservices/users-service/service.yaml /app/users/service.yaml
COPY sdk-api/examples/auth-none-microservices/products-service/service.yaml /app/products/service.yaml
COPY sdk-api/examples/auth-none-microservices/users.lua /app/
COPY sdk-api/examples/auth-none-microservices/products.lua /app/
COPY sdk-api/examples/auth-none-microservices/run.sh /app/
RUN chmod +x /app/users/svc /app/products/svc /app/tester /app/run.sh
EXPOSE 13001 13002
CMD ["/app/run.sh"]
