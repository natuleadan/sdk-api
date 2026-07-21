#!/bin/sh
set -e

RPS=false
PATTERN="TestFile"
for arg in "$@"; do
	case "$arg" in
		--rps) RPS=true ;;
		--test:*) PATTERN="${arg#--test:}" ;;
		-*) ;;
		*) PATTERN="$arg" ;;
	esac
done

export CONFIG_PATH=service.yaml

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 3 http://localhost:23302/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== tests: $PATTERN ==="
/app/tester -test.v -test.run="$PATTERN" -test.count=1
EXIT=$?

if [ "$RPS" = "true" ]; then
	echo "=== seeding 200 hot keys ==="
	for i in $(seq 1 200); do
		code=$(printf "hot%05d" $i)
		curl -s --max-time 5 -X POST "http://localhost:23302/api/files/upload/$code.dat" \
			-H "Content-Type: application/octet-stream" \
			-d "seed-data-$i-benchmark" >/dev/null
	done
	echo "Seeding complete"

	bench_one() {
		local label=$1 lua=$2
		echo "--- $label warmup ---"
		wrk -t10 -c1000 -d3s -s "/app/$lua" --latency "http://localhost:23302" 2>&1 > /dev/null
		echo "--- $label measure ---"
		wrk -t10 -c1000 -d5s -s "/app/$lua" --latency "http://localhost:23302" 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
	}

	bench_one upload   upload.lua
	bench_one download download.lua
fi

echo "=== Benchmark complete ==="
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
