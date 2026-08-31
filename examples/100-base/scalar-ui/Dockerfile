FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY sdk-api/examples/101-scalar-ui ./
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/101-scalar-ui/service.yaml /app/service.yaml
COPY sdk-api/examples/101-scalar-ui/run-test-logic.sh /app/run-test-logic.sh
COPY sdk-api/examples/101-scalar-ui/run.sh /app/run.sh
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23101
ENTRYPOINT ["/app/run.sh"]