#!/bin/sh
set -e

echo "=== Starting mongo-bench service ==="
export MONGO_URI="mongodb://mongo:27017"

/app/svc &
SVC_PID=$!

for i in $(seq 1 20); do
    curl -s http://localhost:18087/api/v1/product/1 > /dev/null 2>&1 && break
    sleep 1
done

echo "=== Seeding 100 products ==="
for i in $(seq 1 100); do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:18087/api/v1/products \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"product-$i\",\"price\":$i.99,\"stock\":$i}")
    if [ "$i" -le 3 ]; then echo "seed $i: HTTP $STATUS name=product-$i"; fi
done
echo "Seeding complete"

echo "=== Verifying product 1 ==="
curl -s http://localhost:18087/api/v1/product/1
echo ""

echo "=== Running wrk benchmark (GET by ID) ==="
wrk -t10 -c1000 -d30s -s /app/get.lua --latency http://localhost:18087
echo "=== Benchmark complete ==="

echo "=== Running wrk benchmark (GET by ID) ==="
wrk -t10 -c1000 -d30s -s /app/get.lua --latency http://localhost:18087
echo "=== Benchmark complete ==="

kill $SVC_PID 2>/dev/null
wait $SVC_PID 2>/dev/null
