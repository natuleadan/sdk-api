#!/bin/sh
set -e

usage() {
	echo "Usage: ./run.sh <example> [test-pattern] [--rps]"
	echo ""
	echo "Examples:"
	echo "  ./run.sh 100                  # functional tests (Docker)"
	echo "  ./run.sh 100 --rps            # functional + RPS (Docker, wrk inside)"
	echo "  ./run.sh 100 TestHealthz_OK  # single test"
	echo ""
	echo "Available examples:"
	echo "  100               - 100-healthz"
	echo "  200/postgres      - 200-url-shortener/postgres"
	echo "  200/nats          - 200-url-shortener/nats"
	echo "  200/kv            - 200-url-shortener/kv-dragonfly"
	echo "  200/pg-dfly       - 200-url-shortener/postgres-dragonfly"
	echo "  200/pgmem-dfly    - 200-url-shortener/postgres-mem-dragonfly"
	echo "  200/mongo         - 200-url-shortener/mongo"
	echo "  200/mariadb       - 200-url-shortener/mariadb"
	echo "  200/sqlite        - 200-url-shortener/sqlite"
	echo "  300/ephemeral     - 300-file-storage/ephemeral"
	echo "  300/cached        - 300-file-storage/cached"
	echo "  300/proxy         - 300-file-storage/proxy"
	echo "  300/pg-nats       - 300-file-storage/pg-nats"
	echo "  300/s3            - 300-file-storage/s3"
	exit 1
}

[ -z "$1" ] && usage

case "$1" in
	100) DIR="100-healthz" ;;
	200/postgres|200/pg) DIR="200-url-shortener/postgres" ;;
	200/nats) DIR="200-url-shortener/nats" ;;
	200/kv|200/kv-dragonfly) DIR="200-url-shortener/kv-dragonfly" ;;
	200/pg-dfly|200/pg-dragonfly) DIR="200-url-shortener/postgres-dragonfly" ;;
	200/pgmem-dfly) DIR="200-url-shortener/postgres-mem-dragonfly" ;;
	200/mongo) DIR="200-url-shortener/mongo" ;;
	200/mariadb) DIR="200-url-shortener/mariadb" ;;
	200/sqlite) DIR="200-url-shortener/sqlite" ;;
	300/ephemeral) DIR="300-file-storage/ephemeral" ;;
	300/cached) DIR="300-file-storage/cached" ;;
	300/proxy) DIR="300-file-storage/proxy" ;;
	300/pg-nats) DIR="300-file-storage/pg-nats" ;;
	300/s3) DIR="300-file-storage/s3" ;;
	400) DIR="400-auth/manual-pg" ;;
	*) echo "Unknown: $1"; usage ;;
esac
shift

cd "$DIR"
docker compose down -v 2>/dev/null
docker compose run --rm bench "$@"
