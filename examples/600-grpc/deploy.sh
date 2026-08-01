#!/bin/sh
set -e

# ============================================================================
# deploy.sh — Build + start + test + cleanup (uno por uno, con timeouts)
# ============================================================================

SERVICES="auth-svc:23601 url-svc:23602 file-svc:23603 ticket-svc:23604 account-svc:23605 transfer-svc:23606 fraud-svc:23607 receipt-svc:23608"
EXIT_CODE=0
TEST_PATTERN=""

for arg in "$@"; do
    case "$arg" in
        --test:*) TEST_PATTERN="${arg#--test:}" ;;
        -*) ;;
    esac
done

wait_svc() {
    name=$1 port=$2
    echo -n "  $name waiting... "
    for i in $(seq 1 60); do
        if curl -s --max-time 2 "http://localhost:$port/healthz" >/dev/null 2>&1 || \
           curl -s --max-time 2 "http://localhost:$port/" >/dev/null 2>&1; then
            echo "ready (${i}s)"
            return 0
        fi
        sleep 1
    done
    echo "TIMEOUT after 60s"
    return 1
}

# ============================================================================
# Cleanup on exit
# ============================================================================
cleanup() {
    echo ""
    echo "=== cleanup ==="
    docker compose down -v 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# ============================================================================
# Step 1: Infrastructure
# ============================================================================
echo "=== infra: postgres ==="
docker compose up -d postgres 2>&1 | tail -1
for i in $(seq 1 30); do
    docker compose exec postgres pg_isready -U postgres >/dev/null 2>&1 && break
    sleep 1
done
echo "  postgres ready"

echo "=== infra: nats ==="
docker compose up -d nats 2>&1 | tail -1
sleep 3
echo "  nats ready"

echo "=== infra: dragonfly ==="
docker compose up -d dragonfly 2>&1 | tail -1
sleep 2
echo "  dragonfly ready"

echo "=== infra: rustfs ==="
docker compose up -d rustfs createbucket 2>&1 | tail -1
sleep 5
echo "  rustfs ready (bucket created)"

# ============================================================================
# Step 2: Microservices (one by one)
# ============================================================================
for entry in $SERVICES; do
    name="${entry%:*}"
    port="${entry#*:}"
    echo "=== build $name ==="
    docker compose build "$name" 2>&1 | tail -1
    echo "=== start $name ==="
    docker compose rm -f -v "$name" 2>/dev/null
    docker compose up -d "$name" 2>&1 | tail -1
    wait_svc "$name" "$port" || EXIT_CODE=1
done

# ============================================================================
# Step 3: Tests
# ============================================================================
if [ "$EXIT_CODE" -eq 0 ]; then
    echo ""
    echo "=== tests ==="
    if [ -n "$TEST_PATTERN" ]; then
        echo "  pattern: $TEST_PATTERN"
        docker compose run --rm bench -test.v -test.run="$TEST_PATTERN" -test.count=1 -test.timeout=60s 2>&1
    else
        docker compose run --rm bench 2>&1
    fi
    EXIT_CODE=$?
fi

# ============================================================================
# Step 4: Result
# ============================================================================
echo ""
if [ "$EXIT_CODE" -eq 0 ]; then
    echo "✅ ALL TESTS PASSED"
else
    echo "❌ TESTS FAILED (exit=$EXIT_CODE)"
fi
exit $EXIT_CODE
