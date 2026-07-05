#!/bin/sh
# WARNING: Runs INSIDE Docker only. Called by Docker CMD. Do not run directly on host.
set -e

echo "=== healthz: functional test ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 10); do
	curl -s --max-time 2 http://localhost:18081/healthz >/dev/null 2>&1 && break
	sleep 1
done
/app/tester -test.v -test.run=TestHealthz_OK -test.count=1
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true; sleep 1

echo "=== healthz: raw Fiber ==="
export RAW=1
/app/svc &
SVC_PID=$!
for i in $(seq 1 10); do
	curl -s --max-time 2 http://localhost:18081/healthz >/dev/null 2>&1 && break
	sleep 1
done
wrk -t10 -c1000 -d15s --latency http://localhost:18081/healthz
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true; sleep 1

echo "=== healthz: SDK full middleware ==="
unset RAW
/app/svc &
SVC_PID=$!
for i in $(seq 1 10); do
	curl -s --max-time 2 http://localhost:18081/healthz >/dev/null 2>&1 && break
	sleep 1
done
wrk -t10 -c1000 -d15s --latency http://localhost:18081/healthz
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
