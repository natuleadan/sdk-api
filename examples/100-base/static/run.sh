#!/bin/sh
set -e

export CONFIG_PATH=service.yaml

BENCH=""
PATTERN="TestStatic"
for arg in "$@"; do
	case "$arg" in
		--bench:*) BENCH="${arg#--bench:}" ;;
		--test:*) PATTERN="${arg#--test:}" ;;
		-*) ;;
		*) PATTERN="$arg" ;;
	esac
done

/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 2 http://localhost:23104/healthz >/dev/null 2>&1 && break
	sleep 1
done

if [ -n "$BENCH" ]; then
	echo "=== bench: $BENCH ==="
	/app/tester -test.bench="$BENCH" -test.run=NONE -test.benchtime=2s -test.count=1
	EXIT=$?
else
	echo "=== functional tests ==="
	/app/tester -test.v -test.run="$PATTERN" -test.count=1
	EXIT=$?
fi

kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
