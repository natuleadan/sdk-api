#!/bin/sh
set -e

RPS=false
PATTERN="TestHealthz"
for arg in "$@"; do
	case "$arg" in
		--rps) RPS=true ;;
		--test:*) PATTERN="${arg#--test:}" ;;
		-*) ;;
		*) PATTERN="$arg" ;;
	esac
done

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 10); do
	curl -s --max-time 2 http://localhost:23100/healthz >/dev/null 2>&1 && break
	sleep 1
done

echo "=== tests: $PATTERN ==="
/app/tester -test.v -test.run="$PATTERN" -test.count=1
EXIT=$?

if [ "$RPS" = "true" ]; then
	echo "=== RPS warmup (3s) ==="
	wrk -t10 -c1000 -d3s -s /app/healthz.lua --latency http://localhost:23100 2>&1 > /dev/null
	echo "=== RPS measure (5s) ==="
	wrk -t10 -c1000 -d5s -s /app/healthz.lua --latency http://localhost:23100 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
fi

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
