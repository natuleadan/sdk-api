#!/bin/sh
set -e

RPS=false
PATTERN="Test"
for arg in "$@"; do
	case "$arg" in
		--rps) RPS=true ;;
		--test:*) PATTERN="${arg#--test:}" ;;
		-*) ;;
		*) PATTERN="$arg" ;;
	esac
done

echo "=== starting service ==="
export DOCKER_TEST=1
/app/svc &
SVC_PID=$!
for i in $(seq 1 30); do
	curl -s --max-time 2 http://localhost:23400/healthz >/dev/null 2>&1 && break
	sleep 1
done

echo "=== tests: $PATTERN ==="
/app/tester -test.v -test.run="$PATTERN" -test.count=1
EXIT=$?

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
