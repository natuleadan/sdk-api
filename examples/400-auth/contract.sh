#!/usr/bin/env bash
# Runs the shared auth contract (runtime/authtest) against one auth stack.
#
# The contract is driver-agnostic: the same HTTP behaviour checks run against
# every stack, and only the identity/authorization plumbing changes. Use this
# as the anti-drift guard when bumping an Ory/FGA/Zitadel image or swapping
# drivers: the contract must stay green.
#
# Usage:
#   DRIVER=manual ./contract.sh     # manual driver on PostgreSQL
#   DRIVER=libsql ./contract.sh     # manual driver on local libSQL
#   DRIVER=mysql  ./contract.sh     # manual driver on MariaDB
#   DRIVER=fga    ./contract.sh     # Zitadel identity + OpenFGA authorization
#   DRIVER=ory    ./contract.sh     # Kratos identity + Keto authorization
#
# Flags:
#   --clean   recreate the stack volumes (fresh DB; required for a fresh
#             Zitadel machine key on the fga stack)
#
# Dev-only secrets below are the same values the docker-compose files ship;
# nothing here is used outside local testing.
set -euo pipefail

DRIVER="${DRIVER:-manual}"
CLEAN=0
for arg in "$@"; do
	case "$arg" in
		--clean) CLEAN=1 ;;
		*) echo "unknown flag: $arg" >&2; exit 2 ;;
	esac
done

HERE="$(cd "$(dirname "$0")" && pwd)"
export PATH="/usr/local/go/bin:$HOME/go/bin:/opt/homebrew/bin:$PATH"

JWT_SECRET="dev-secret-hs256-change-in-prod"
ENCRYPT_COOKIE_KEY="diPHoCg5vhBrTHCSJhlud1RRMRFpRo+4N/d32S+48t8="
APP_PORT=23400
RUN_DIR="$(mktemp -d /tmp/auth-contract-XXXXXX)"

case "$DRIVER" in
	manual) STACK=manual-pg ;;
	libsql) STACK=manual-libsql ;;
	mysql) STACK=manual-mysql ;;
	fga) STACK=openfga-zitadel ;;
	ory) STACK=ory-stack ;;
	*) echo "unknown DRIVER=$DRIVER (want manual|libsql|mysql|fga|ory)" >&2; exit 2 ;;
esac

STACK_DIR="$HERE/$STACK"
cd "$STACK_DIR"

# Only one stack can own the app port and the shared infra ports.
for other in manual-pg manual-libsql manual-mysql openfga-zitadel ory-stack; do
	if [ "$other" != "$STACK" ] && [ -f "$HERE/$other/docker-compose.yml" ]; then
		(cd "$HERE/$other" && docker compose stop >/dev/null 2>&1 || true)
	fi
done

if [ "$CLEAN" = "1" ]; then
	docker compose down -v >/dev/null 2>&1 || true
	rm -rf "$STACK_DIR/.machinekey" "$STACK_DIR/.runtime"
fi

echo "== stack: $STACK (DRIVER=$DRIVER) =="
# Only the infra services: the compose `bench` image builds against the pinned
# SDK version, while this script runs the working-tree service on the host.
case "$DRIVER" in
	manual) docker compose up -d postgres dragonfly >/dev/null ;;
	libsql) docker compose up -d valkey >/dev/null ;;
	mysql) docker compose up -d mariadb valkey >/dev/null ;;
	*) docker compose up -d >/dev/null ;;
esac
# Wait for the containers the app depends on.
case "$DRIVER" in
	manual)
		for _ in $(seq 1 30); do
			docker exec auth-roles-bench-postgres-1 pg_isready -U postgres >/dev/null 2>&1 && break
			sleep 1
		done
		;;
	fga)
		for _ in $(seq 1 60); do
			curl -sf -o /dev/null http://localhost:18082/.well-known/openid-configuration && break
			sleep 1
		done
		;;
	ory)
		for _ in $(seq 1 60); do
			curl -sf -o /dev/null http://localhost:14433/health/alive && break
			sleep 1
		done
		;;
esac

echo "== build =="
go build -o "$RUN_DIR/svc" ./cmd/

# Stack-specific environment: the service and the test binary share DATABASE_URL
# style values so the contract can inspect state where the stack allows it.
case "$DRIVER" in
	manual)
		# shellcheck disable=SC1091
		set -a; . "$STACK_DIR/oidc_private_key.env"; set +a
		export DB_DRIVER=postgres
		export DATABASE_URL="postgres://postgres:postgres@localhost:25436/auth_roles?sslmode=disable"
		export REDIS_URL="localhost:6379"
		;;
	libsql)
		# shellcheck disable=SC1091
		set -a; . "$STACK_DIR/oidc_private_key.env"; set +a
		export DB_DRIVER=turso
		export DATABASE_URL="$RUN_DIR/contract.db"
		export REDIS_URL="localhost:6379"
		;;
	mysql)
		# shellcheck disable=SC1091
		set -a; . "$STACK_DIR/oidc_private_key.env"; set +a
		export DB_DRIVER=mysql
		export DATABASE_URL="test:pass@tcp(localhost:3306)/auth?parseTime=true"
		export REDIS_URL="localhost:6379"
		;;
	fga)
		export OPENFGA_URL="http://localhost:18080"
		export ZITADEL_URL="http://localhost:18082"
		export ZITADEL_MACHINEKEY="$STACK_DIR/.machinekey/machinekey.json"
		export DATABASE_URL="postgres://postgres:postgres@localhost:25436/auth_fga?sslmode=disable"
		export REDIS_URL="localhost:6379"
		;;
	ory)
		export KRATOS_URL="http://localhost:14433"
		export KRATOS_ADMIN_URL="http://localhost:14434"
		export KETO_READ_URL="http://localhost:4466"
		export KETO_WRITE_URL="http://localhost:4467"
		export DATABASE_URL="postgres://postgres:postgres@localhost:25437/auth_ory?sslmode=disable"
		export REDIS_URL="localhost:6379"
		;;
esac

if lsof -ti ":$APP_PORT" >/dev/null 2>&1; then
	echo "port $APP_PORT busy, stopping the process holding it" >&2
	lsof -ti ":$APP_PORT" | xargs kill -9 2>/dev/null || true
	sleep 1
fi

echo "== start service =="
CONFIG_PATH=service.yaml DOCKER_TEST=1 JWT_SECRET="$JWT_SECRET" \
	ENCRYPT_COOKIE_KEY="$ENCRYPT_COOKIE_KEY" \
	nohup "$RUN_DIR/svc" >"$RUN_DIR/service.log" 2>&1 &
SVC_PID=$!
cleanup() {
	kill "$SVC_PID" 2>/dev/null || true
	wait "$SVC_PID" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 60); do
	if curl -sf -o /dev/null "http://localhost:$APP_PORT/healthz"; then
		break
	fi
	sleep 1
done
if ! curl -sf -o /dev/null "http://localhost:$APP_PORT/healthz"; then
	echo "service did not become ready; last log lines:" >&2
	tail -20 "$RUN_DIR/service.log" >&2
	exit 1
fi

echo "== contract (DRIVER=$DRIVER) =="
DOCKER_TEST=1 go test -run TestAuthContract -v -test.timeout=300s -count=1 .
