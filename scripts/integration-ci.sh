#!/bin/bash
set -euo pipefail

export PATH="/usr/pgsql-16/bin:$PATH"

PGDATA="$(mktemp -d)"

cleanup() {
    pg_ctl stop -D "$PGDATA" -m fast 2>/dev/null || true
    rm -rf "$PGDATA"
}
trap cleanup EXIT

initdb -D "$PGDATA" --no-locale --encoding=UTF8 --auth=trust
pg_ctl start -D "$PGDATA" -o "-k /tmp -c listen_addresses=''" -w

export INTEGRATION_DATABASE_DSN="postgresql:///postgres?host=/tmp"
make integration
