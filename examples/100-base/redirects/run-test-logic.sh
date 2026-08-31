#!/bin/sh
set -e

export CONFIG_PATH=service.yaml

/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 2 http://localhost:23103/healthz >/dev/null 2>&1 && break
	sleep 1
done

echo "=== functional tests ==="
/app/tester -test.v -test.run="TestRedirect" -test.count=1
EXIT=$?

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
