#!/bin/sh
set -e

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 3 http://localhost:10123/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== functional tests ==="
/app/tester -test.v -test.run=TestFile -test.count=1
EXIT=$?

if [ "$RPS_BENCH" = "1" ]; then
	echo "=== seeding 200 hot keys ==="
	for i in $(seq 1 200); do
		code=$(printf "hot%05d" $i)
		curl -s --max-time 5 -X POST "http://localhost:10123/api/v1/files/upload/$code.dat" \
			-H "Content-Type: application/octet-stream" \
			-d "seed-data-$i-benchmark" >/dev/null
	done
	echo "Seeding complete"

	bench_one() {
		local label=$1 lua=$2
		echo "--- $label warmup ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:10123" 2>&1 | awk '/Requests\/sec/ {print "  warmup:", $2}'
		sleep 2
		echo "--- $label measure ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:10123" 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
		sleep 1
	}

	bench_one upload     upload.lua
	bench_one download   download.lua
	bench_one sign-only  sign.lua
fi

echo "=== Benchmark complete ==="
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
