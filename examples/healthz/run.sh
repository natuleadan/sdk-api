#!/bin/sh
set -e

echo "=== healthz: raw Fiber ==="
export RAW=1
/app/svc &
SVC_PID=$!
sleep 2
wrk -t10 -c1000 -d30s --latency http://localhost:18081/healthz
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null; sleep 1

echo "=== healthz: SDK full middleware ==="
unset RAW
/app/svc &
SVC_PID=$!
sleep 2
wrk -t10 -c1000 -d30s --latency http://localhost:18081/healthz
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null
