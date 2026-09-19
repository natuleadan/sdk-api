FROM golang:1.27-alpine AS builder
WORKDIR /build
ARG SDK_VERSION=v0.31.0
COPY sdk-api/examples/400-auth/openfga-zitadel ./
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o /svc ./cmd/
RUN CGO_ENABLED=0 go test -c -o /tester .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/400-auth/openfga-zitadel/run.sh /app/run.sh
COPY sdk-api/examples/400-auth/openfga-zitadel/service.yaml /app/service.yaml
RUN chmod +x /app/run.sh /app/svc /app/tester
EXPOSE 23400
ENTRYPOINT ["/app/run.sh"]
