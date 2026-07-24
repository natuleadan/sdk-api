#!/bin/sh
set -e

echo "=== starting services ==="

start_svc() {
    name=$1
    port=$2
    echo "  $name:$port"
    CONFIG_PATH=/app/configs/$name/service.yaml /app/svcs/$name &
    eval "${name}_pid=\$!"
    for i in $(seq 1 30); do
        curl -s --max-time 2 http://localhost:$port/healthz >/dev/null 2>&1 && break
        sleep 1
    done
}

start_svc auth 23601
start_svc url 23602
start_svc file 23603
start_svc ticket 23604
start_svc account 23605
start_svc transfer 23606
start_svc fraud 23607
start_svc receipt 23608

echo "=== running tests ==="
/app/tester -test.v -test.count=1 -test.timeout=300s
EXIT=$?

echo "=== stopping services ==="
for pid_var in auth_pid url_pid file_pid ticket_pid account_pid transfer_pid fraud_pid receipt_pid; do
    eval "pid=\$$pid_var"
    kill $pid 2>/dev/null || true
done
wait 2>/dev/null || true

exit $EXIT
