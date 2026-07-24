#!/bin/sh
set -e

SERVICES="auth-svc:23601 url-svc:23602 file-svc:23603 ticket-svc:23604 account-svc:23605 transfer-svc:23606 fraud-svc:23607 receipt-svc:23608"

echo "=== waiting for services ==="
for svc in $SERVICES; do
    name="${svc%:*}"
    port="${svc#*:}"
    echo -n "  $name "
    for i in $(seq 1 60); do
        if curl -s --max-time 2 "http://$name:$port/healthz" >/dev/null 2>&1 || \
           curl -s --max-time 2 "http://$name:$port/" >/dev/null 2>&1; then
            echo "ready ($((i))s)"
            break
        fi
        sleep 1
    done
done

echo "=== running tests ==="
/app/tester -test.v -test.count=1 -test.timeout=300s "$@"
