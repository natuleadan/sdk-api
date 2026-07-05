#!/bin/sh
# WARNING: Runs INSIDE Docker only. Called by Docker CMD. Do not run directly on host.
set -e

echo "=== url-link-base: starting service ==="
export CONFIG_PATH=service.docker.yaml
export REDIS_ADDR=redis:6379

/app/svc &
SVC_PID=$!

for i in $(seq 1 15); do
	curl -s --max-time 3 http://localhost:18083/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== url-link-base: functional tests ==="
/app/tester -test.v -test.run=TestURLLink -test.count=1

echo "=== url-link-base: seeding 100 hot keys ==="
for i in $(seq 1 100); do
	code=$(printf "hot%05d" $i)
	curl -s --max-time 5 -X POST http://localhost:18083/api/v1/links \
		-H "Content-Type: application/json" \
		-d "{\"targetUrl\":\"https://hot-$i.example.com\",\"shortCode\":\"$code\"}" >/dev/null
done
echo "Seeding complete"

echo "=== url-link-base: wrk benchmark ==="
wrk -t10 -c1000 -d15s -s /app/expand.lua --latency http://localhost:18083
echo "=== Benchmark complete ==="

kill $SVC_PID 2>/dev/null
wait $SVC_PID 2>/dev/null || true
