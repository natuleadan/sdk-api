FROM golang:1.26-alpine AS builder
RUN apk add --no-cache protobuf protoc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
WORKDIR /build
ARG SDK_VERSION=v0.24.0
COPY sdk-api/examples/600-grpc ./
RUN go mod edit -require=github.com/natuleadan/sdk-api@${SDK_VERSION} && go mod tidy && go mod download
RUN rm -rf pb && protoc --proto_path=proto --go_out=. --go_opt=module=600-grpc --go-grpc_out=. --go-grpc_opt=module=600-grpc proto/auth_service.proto proto/url_service.proto proto/file_service.proto proto/ticket_service.proto
RUN CGO_ENABLED=0 go build -o /svc ./url-svc/cmd/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /svc /app/svc
COPY sdk-api/examples/600-grpc/url-svc/service.yaml /app/service.yaml
COPY sdk-api/examples/600-grpc/url-svc/run.sh /app/run.sh
RUN chmod +x /app/run.sh /app/svc
EXPOSE 23602
ENTRYPOINT ["/app/run.sh"]
