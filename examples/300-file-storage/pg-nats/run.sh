#!/bin/sh
set -e

export CONFIG_PATH=service.yaml

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 20); do
	curl -s --max-time 3 http://localhost:23304/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== functional tests ==="
/app/tester -test.v -test.run=TestFile -test.count=1
EXIT=$?

if [ "$RPS_BENCH" = "1" ]; then
	echo "=== seeding 50 products ==="
	for i in $(seq 1 50); do
		curl -s --max-time 5 -X POST http://localhost:23304/api/v1/products \
			-H "Content-Type: application/json" \
			-d "{\"name\":\"product-$i\",\"price\":$i.99}" >/dev/null
	done
	echo "Seeding complete"

	bench_one() {
		local label=$1 lua=$2
		echo "--- $label warmup ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:23304" 2>&1 | awk '/Requests\/sec/ {print "  warmup:", $2}'
		sleep 2
		echo "--- $label measure ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:23304" 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
		sleep 1
	}

	bench_one list     list.lua
	bench_one create   create.lua
fi

echo "=== Benchmark complete ==="
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
