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

if [ -z "$JWT_SECRET" ]; then
	JWT_SECRET=$(openssl genrsa 2048 2>/dev/null | openssl pkcs8 -topk8 -nocrypt 2>/dev/null)
	export JWT_SECRET
fi
export OLD_JWT_SECRET="${OLD_JWT_SECRET:-$JWT_SECRET}"

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
