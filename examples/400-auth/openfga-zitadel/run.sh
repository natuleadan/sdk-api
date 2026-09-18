#!/bin/sh
set -e

PATTERN="${1:-Test}"
case "$PATTERN" in
	--test:*) PATTERN="${PATTERN#--test:}" ;;
	-*) ;;
	*) PATTERN="$PATTERN" ;;
esac

export CONFIG_PATH=service.yaml
export DOCKER_TEST=1

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 30); do
	curl -s --max-time 2 http://localhost:23400/healthz >/dev/null 2>&1 && break
	sleep 1
done

echo "=== tests: $PATTERN ==="
/app/tester -test.v -test.run="$PATTERN" -test.count=1
EXIT=$?

kill $SVC_PID 2>/dev/null || true
wait $SVC_PID 2>/dev/null || true
exit $EXIT
