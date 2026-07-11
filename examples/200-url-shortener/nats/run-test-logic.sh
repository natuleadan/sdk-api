#!/bin/sh
set -e
cd "$(dirname "$0")"
export CONFIG_PATH=service.yaml
echo "Starting dependencies..."
docker compose up -d postgres nats
echo "Waiting for services..."
for i in $(seq 1 15); do
    if docker compose exec -T postgres pg_isready -U dev >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
echo "Running tests..."
go test -v -count=1 -timeout=120s . 2>&1
