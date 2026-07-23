#!/bin/sh
set -e

export CONFIG_PATH=service.yaml

echo "=== starting service ==="
/app/svc &
SVC_PID=$!

for i in $(seq 1 30); do
	curl -s --max-time 2 http://localhost:23500/api/v1/tickets >/dev/null 2>&1 && break
	sleep 1
done

echo "=== running tests ==="
if [ -f /app/tester ]; then
	PATTERN="${1:-Test}"
	/app/tester -test.v -test.run="$PATTERN" -test.count=1
	EXIT=$?
else
	echo "no test binary found, running service only"
	wait $SVC_PID
	EXIT=$?
fi

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
