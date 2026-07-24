FROM golang:1.26-alpine AS builder
RUN apk add --no-cache protobuf protoc
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
WORKDIR /build
COPY sdk-api /sdk-api
COPY sdk-api/examples/600-grpc ./
RUN go mod edit -replace github.com/natuleadan/sdk-api=/sdk-api && go mod tidy && go mod download
RUN protoc --proto_path=proto --go_out=. --go_opt=module=600-grpc --go-grpc_out=. --go-grpc_opt=module=600-grpc proto/auth_service.proto proto/url_service.proto proto/file_service.proto proto/ticket_service.proto proto/account_service.proto proto/transfer_service.proto
RUN CGO_ENABLED=0 go test -tags=integration -c -o /tester ./tests/

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /tester /app/tester
COPY sdk-api/examples/600-grpc/tests/run.sh /app/run.sh
RUN chmod +x /app/run.sh /app/tester
ENTRYPOINT ["/app/run.sh"]
