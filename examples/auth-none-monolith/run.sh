#!/bin/sh
set -e
echo "=== auth-none-monolith ==="

/app/svc &
SVC_PID=$!

echo "Waiting for service..."
for i in $(seq 1 20); do
	curl -s http://localhost:13000/health >/dev/null 2>&1 && break
	sleep 1
done

echo "Service ready, running functional tests..."
/app/tester -test.v -test.run=TestNone -test.benchtime=2s

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true; sleep 2

echo "=== wrk POST benchmark (fresh service) ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 20); do
	curl -s http://localhost:13000/health >/dev/null 2>&1 && break
	sleep 1
done
wrk -t10 -c1000 -d30s -s /app/post.lua --latency http://localhost:13000
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true; sleep 2

echo "=== wrk GET benchmark (fresh service) ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 20); do
	curl -s http://localhost:13000/health >/dev/null 2>&1 && break
	sleep 1
done
wrk -t10 -c1000 -d30s -s /app/get.lua --latency http://localhost:13000
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
echo "=== Done ==="
