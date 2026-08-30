#!/bin/sh
set -e

usage() {
	echo "Usage: ./run.sh <example> [test-pattern] [--rps[:endpoint]]"
	echo ""
	echo "Examples:"
	echo "  ./run.sh 100                  # functional tests (Docker)"
	echo "  ./run.sh 100 --rps            # functional + RPS (Docker, wrk inside)"
	echo "  ./run.sh 100 TestHealthz_OK  # single test"
	echo "  ./run.sh 200/postgres --rps:create   # RPS only for one endpoint"
	echo ""
	echo "Available examples:"
	echo "  100               - 100-healthz"
	echo "  101               - 101-scalar-ui (CORS matrix across entry types)"
	echo "  200/postgres      - 200-url-shortener/postgres"
	echo "  200/nats          - 200-url-shortener/nats"
	echo "  200/kv            - 200-url-shortener/kv-dragonfly"
	echo "  200/pg-dfly       - 200-url-shortener/postgres-dragonfly"
	echo "  200/pgmem-dfly    - 200-url-shortener/postgres-mem-dragonfly"
	echo "  200/mongo         - 200-url-shortener/mongo"
	echo "  200/mariadb       - 200-url-shortener/mariadb"
	echo "  200/turso         - 200-url-shortener/tursogo (local embedded)"
	echo "  200/tsl           - 200-url-shortener/turso-serverless (remote, pure Go)"
	echo "  200/libsql        - 200-url-shortener/libsql (remote hrana)"
	echo "  200/go-libsql     - 200-url-shortener/go-libsql (embedded replica, CGO)"
	echo "  300/ephemeral     - 300-file-storage/ephemeral"
	echo "  300/cached        - 300-file-storage/cached"
	echo "  300/proxy         - 300-file-storage/proxy"
	echo "  300/pg-nats       - 300-file-storage/pg-nats"
	echo "  300/s3            - 300-file-storage/s3"
	echo "  400               - 400-auth/manual-pg"
	echo "  500               - 500-tickets"
	echo "  600               - 600-grpc (uses deploy.sh: infra + services + tests)"
	exit 1
}

[ -z "$1" ] && usage

case "$1" in
	100) DIR="100-healthz" ;;
	101) DIR="101-scalar-ui" ;;
	200/postgres|200/pg) DIR="200-url-shortener/postgres" ;;
	200/nats) DIR="200-url-shortener/nats" ;;
	200/kv|200/kv-dragonfly) DIR="200-url-shortener/kv-dragonfly" ;;
	200/pg-dfly|200/pg-dragonfly) DIR="200-url-shortener/postgres-dragonfly" ;;
	200/pgmem-dfly) DIR="200-url-shortener/postgres-mem-dragonfly" ;;
	200/mongo) DIR="200-url-shortener/mongo" ;;
	200/mariadb) DIR="200-url-shortener/mariadb" ;;
	200/turso) DIR="200-url-shortener/tursogo" ;;
	200/tsl) DIR="200-url-shortener/turso-serverless" ;;
	200/libsql) DIR="200-url-shortener/libsql" ;;
	200/go-libsql) DIR="200-url-shortener/go-libsql" ;;
	300/ephemeral) DIR="300-file-storage/ephemeral" ;;
	300/cached) DIR="300-file-storage/cached" ;;
	300/proxy) DIR="300-file-storage/proxy" ;;
	300/pg-nats) DIR="300-file-storage/pg-nats" ;;
	300/s3) DIR="300-file-storage/s3" ;;
	400) DIR="400-auth/manual-pg" ;;
	500) DIR="500-tickets" ;;
	600) DIR="600-grpc" ;;
	*) echo "Unknown: $1"; usage ;;
esac
shift

if [ "$DIR" = "600-grpc" ]; then
	cd "$DIR"
	exec ./deploy.sh "$@"
fi

cd "$DIR"
docker compose down -v 2>/dev/null
docker compose run --rm bench "$@"
