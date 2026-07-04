#!/bin/sh
set -e
echo "=== auth-none-microservices ==="

echo "Starting users-service..."
cd /app/users && /app/users/svc &
UPID=$!

echo "Starting products-service..."
cd /app/products && /app/products/svc &
PPID=$!

echo "Waiting for services..."
for i in $(seq 1 20); do
	curl -s http://localhost:13001/health >/dev/null 2>&1 && break
	sleep 1
done
for i in $(seq 1 20); do
	curl -s http://localhost:13002/health >/dev/null 2>&1 && break
	sleep 1
done

cd /app
echo "Running Go tests..."
/app/tester -test.v -test.run=TestNoneMS -test.bench=. -test.benchtime=5s

echo ""
echo "=== wrk Users GET ==="
wrk -t10 -c1000 -d30s -s /app/users.lua --latency http://localhost:13001
echo ""

echo "=== wrk Products GET ==="
wrk -t10 -c1000 -d30s -s /app/products.lua --latency http://localhost:13002
echo ""

kill $UPID $PPID 2>/dev/null; wait $UPID $PPID 2>/dev/null || true
echo "=== Done ==="
