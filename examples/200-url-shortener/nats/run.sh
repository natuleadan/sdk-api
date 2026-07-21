#!/bin/sh
set -e

RPS=false
PATTERN="TestURL|TestNATS|TestCache|TestWorker"
for arg in "$@"; do
	case "$arg" in
		--rps) RPS=true ;;
		--test:*) PATTERN="${arg#--test:}" ;;
		-*) ;;
		*) PATTERN="$arg" ;;
	esac
done

export CONFIG_PATH=/app/service.docker.yaml

echo "=== starting service ==="
/app/svc &
SVC_PID=$!
for i in $(seq 1 15); do
	curl -s --max-time 3 http://localhost:23202/health >/dev/null 2>&1 && break
	sleep 1
done

echo "=== tests: $PATTERN ==="
/app/tester -test.v -test.run="$PATTERN" -test.count=1
EXIT=$?

if [ "$RPS" = "true" ]; then
	echo "=== seeding 200 PG hot keys ==="
	for i in $(seq 1 200); do
		code=$(printf "hot%05d" $i)
		data=$(printf '{"targetUrl":"https://hot-%s.example.com","shortCode":"%s"}' "$i" "$code")
		curl -s --max-time 5 -X POST http://localhost:23202/api/links \
			-H "Content-Type: application/json" \
			-d "$data" >/dev/null
	done
	echo "PG seed complete"

	echo "=== seeding 100 KV keys ==="
	for i in $(seq 1 100); do
		key=$(printf "kv%05d" $i)
		curl -s --max-time 5 -X PUT "http://localhost:23202/api/nats/kv/$key" \
			-d "seed-value-$i" >/dev/null
	done
	echo "KV seed complete"

	bench_one() {
		local label=$1 lua=$2
		echo "--- $label warmup ---"
		wrk -t10 -c1000 -d3s -s "/app/$lua" --latency "http://localhost:23202" 2>&1 > /dev/null
		echo "--- $label measure ---"
		wrk -t10 -c1000 -d5s -s "/app/$lua" --latency "http://localhost:23202" 2>&1 | awk '/Requests\/sec/ {print "  measure:", $2}'
	}

	bench_one list     list.lua
	bench_one expand   expand.lua
	bench_one create   create.lua
	bench_one update   update.lua
	bench_one delete   delete.lua
	bench_one rpc      rpc.lua
	bench_one kv-get   kv-get.lua
	bench_one kv-set   kv-set.lua
fi

echo "=== Benchmark complete ==="
kill $SVC_PID 2>/dev/null; wait $SVC_PID 2>/dev/null || true
exit $EXIT
