#!/bin/sh
set -e

echo "=== Starting url-link-nats service ==="
export CONFIG_PATH=service.docker.yaml
export NATS_URL=nats://nats:4222

/app/svc &
SVC_PID=$!

for i in $(seq 1 15); do
    curl -s http://localhost:18084/health > /dev/null 2>&1 && break
    sleep 1
done

echo "=== Seeding 100 hot keys ==="
for i in $(seq 1 100); do
    code=$(printf "hot%05d" $i)
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:18084/api/v1/links \
        -H "Content-Type: application/json" \
        -d "{\"targetUrl\":\"https://hot-$i.example.com\",\"shortCode\":\"$code\"}")
    if [ "$i" -le 3 ]; then echo "seed $i: HTTP $STATUS shortCode=$code"; fi
done
echo "Seeding complete"

echo "=== Verifying first key ==="
curl -s -w " HTTP %{http_code}\n" http://localhost:18084/api/v1/expand/hot00001
echo ""

echo "=== Running wrk benchmark ==="
wrk -t10 -c1000 -d30s -s /app/expand.lua --latency http://localhost:18084
echo "=== Benchmark complete ==="

echo "=== Running wrk benchmark ==="
wrk -t10 -c1000 -d30s -s /app/expand.lua --latency http://localhost:18084
echo "=== Benchmark complete ==="

kill $SVC_PID 2>/dev/null
wait $SVC_PID 2>/dev/null
