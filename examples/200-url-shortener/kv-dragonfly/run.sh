#!/bin/sh
set -e

export CONFIG_PATH=service.docker.yaml
export DRAGONFLY_ADDR=dragonfly:6379

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 3 http://localhost:18091/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== functional tests ==="
/app/tester -test.v -test.run=TestURL -test.count=1
EXIT=$?

if [ "$RPS_BENCH" = "1" ]; then
	echo "=== seeding 200 hot keys ==="
	for i in $(seq 1 200); do
		code=$(printf "hot%05d" $i)
		curl -s --max-time 5 -X POST http://localhost:18091/api/v1/links \
			-H "Content-Type: application/json" \
			-d "{\"targetUrl\":\"https://hot-$i.example.com\",\"shortCode\":\"$code\"}" >/dev/null
	done
	echo "Seeding complete"

	bench_one() {
		local label=$1 lua=$2
		echo "--- $label warmup ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:18091" 2>&1 | awk '/Requests\/sec/ {print "  warmup:", $2}'
		sleep 2
		echo "--- $label measure ---"
		wrk -t10 -c1000 -d30s -s "/app/$lua" --latency "http://localhost:18091" 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
		sleep 1
	}

	bench_one expand   expand.lua
	bench_one list     list.lua
	bench_one getbyid  getbyid.lua
	bench_one create   create.lua
	bench_one update   update.lua
	bench_one delete   delete.lua
fi

echo "=== Benchmark complete ==="
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
