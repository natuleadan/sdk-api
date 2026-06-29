#!/bin/sh
set -e

echo "=== Starting mysql-bench service ==="
export CONFIG_PATH=service.docker.yaml
export MYSQL_URL="${MYSQL_URL}"

/app/svc &
SVC_PID=$!

for i in $(seq 1 20); do
    curl -s http://localhost:18085/health > /dev/null 2>&1 && break
    sleep 1
done

echo "=== Seeding 100 products ==="
for i in $(seq 1 100); do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:18085/api/v1/products \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"product-$i\",\"price\":$i.99,\"stock\":$i}")
    if [ "$i" -le 3 ]; then echo "seed $i: HTTP $STATUS name=product-$i"; fi
done
echo "Seeding complete"

echo "=== Verifying product 1 ==="
curl -s -w " HTTP %{http_code}\n" http://localhost:18085/api/v1/product/1
echo ""

echo "=== Running wrk benchmark (GET by ID) ==="
wrk -t10 -c1000 -d30s -s /app/get.lua --latency http://localhost:18085
echo "=== Benchmark complete ==="

echo "=== Running wrk benchmark (GET by ID) ==="
wrk -t10 -c1000 -d30s -s /app/get.lua --latency http://localhost:18085
echo "=== Benchmark complete ==="

kill $SVC_PID 2>/dev/null
wait $SVC_PID 2>/dev/null
