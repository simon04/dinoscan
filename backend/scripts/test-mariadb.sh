#!/bin/sh
# Runs all tests, including the integration tests against a throwaway local
# MariaDB server holding a tiny replica-like fixture (see
# internal/petscan/integration_test.go). Needs mariadbd and
# mariadb-install-db.
set -eu
cd "$(dirname "$0")/.."

dir=$(mktemp -d)
port=$((33000 + $$ % 1000))
cleanup() {
	[ -f "$dir/pid" ] && kill "$(cat "$dir/pid")" 2>/dev/null || true
	sleep 1
	rm -rf "$dir"
}
trap cleanup EXIT

mariadb-install-db --no-defaults --datadir="$dir/data" >"$dir/install.log" 2>&1
mariadbd --no-defaults --datadir="$dir/data" --socket="$dir/sock" --pid-file="$dir/pid" \
	--port="$port" --bind-address=127.0.0.1 --skip-grant-tables --skip-log-error >"$dir/server.log" 2>&1 &
for _ in $(seq 50); do
	mariadb-admin --no-defaults --socket="$dir/sock" ping >/dev/null 2>&1 && break
	sleep 0.2
done

DINOSCAN_TEST_MARIADB="127.0.0.1:$port" go test "$@" ./...
