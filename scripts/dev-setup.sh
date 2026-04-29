#!/bin/sh
# Stand up a seeded PostgreSQL + Redis environment for local development.
# By default only seeds the database and prints connection info.
# Pass --serve to also start the sippy API server.
#
# Usage:
#   make dev                 # build + seed only
#   make dev SERVE=1         # build + seed + start sippy
#   scripts/dev-setup.sh     # seed only (assumes sippy binary exists)
#   scripts/dev-setup.sh --serve  # seed + start sippy
#
# To tear down:  Ctrl-C (containers are cleaned up automatically)

set -e

SERVE=false
for arg in "$@"; do
    case "$arg" in
        --serve) SERVE=true ;;
    esac
done

DOCKER="${DOCKER:-podman}"
PSQL_CONTAINER="sippy-dev-postgresql"
PSQL_PORT="${PSQL_PORT:-25433}"
REDIS_CONTAINER="sippy-dev-redis"
REDIS_PORT="${REDIS_PORT:-25479}"
SIPPY_API_PORT="${SIPPY_API_PORT:-8080}"

clean_up() {
    echo ""
    echo "Shutting down..."
    if [ -n "$CHILD_PID" ]; then
        kill $CHILD_PID 2>/dev/null && wait $CHILD_PID 2>/dev/null
    fi
    echo "Stopping $PSQL_CONTAINER"
    $DOCKER stop $PSQL_CONTAINER 2>/dev/null
    $DOCKER rm $PSQL_CONTAINER 2>/dev/null
    echo "Stopping $REDIS_CONTAINER"
    $DOCKER stop $REDIS_CONTAINER 2>/dev/null
    $DOCKER rm $REDIS_CONTAINER 2>/dev/null
}
trap clean_up EXIT

# Clean up any stale containers from a previous run
$DOCKER stop $PSQL_CONTAINER 2>/dev/null || true
$DOCKER rm $PSQL_CONTAINER 2>/dev/null || true
$DOCKER stop $REDIS_CONTAINER 2>/dev/null || true
$DOCKER rm $REDIS_CONTAINER 2>/dev/null || true

echo "Starting PostgreSQL on port $PSQL_PORT..."
$DOCKER run --name $PSQL_CONTAINER -e POSTGRES_PASSWORD=password -p $PSQL_PORT:5432 -d quay.io/enterprisedb/postgresql

echo "Starting Redis on port $REDIS_PORT..."
$DOCKER run --name $REDIS_CONTAINER -p $REDIS_PORT:6379 -d quay.io/openshiftci/redis:latest

echo "Waiting for PostgreSQL to be ready..."
timeout=30
elapsed=0
until $DOCKER exec $PSQL_CONTAINER psql -U postgres -d postgres -c '\q' 2>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: PostgreSQL did not become ready within ${timeout}s"
        exit 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
done

DSN="postgresql://postgres:password@localhost:$PSQL_PORT/postgres"
REDIS_URL="redis://localhost:$REDIS_PORT"

echo "Seeding database..."
./sippy seed-data --init-database --database-dsn="$DSN"

echo ""
echo "================================================"
echo "  Dev environment ready"
echo "  PostgreSQL: $DSN"
echo "  Redis:      $REDIS_URL"
echo "================================================"

if [ "$SERVE" = true ]; then
    set -- \
      --listen ":$SIPPY_API_PORT" \
      --listen-metrics ":12112" \
      --database-dsn="$DSN" \
      --enable-write-endpoints \
      --log-level debug \
      --views config/e2e-views.yaml \
      --redis-url="$REDIS_URL" \
      --data-provider postgres
    if [ -n "$GCS_SA_JSON_PATH" ]; then
        set -- "$@" --google-service-account-credential-file "$GCS_SA_JSON_PATH"
    fi

    echo ""
    echo "Starting sippy on http://localhost:$SIPPY_API_PORT ..."
    echo "Press Ctrl-C to stop"
    echo ""

    ./sippy serve "$@" &
    CHILD_PID=$!

    wait $CHILD_PID
else
    echo ""
    echo "To start sippy against this database:"
    echo "  ./sippy serve --database-dsn=\"$DSN\" --redis-url=\"$REDIS_URL\" --data-provider postgres --views config/e2e-views.yaml --log-level debug"
    echo ""
    echo "Press Ctrl-C to tear down containers"
    # Keep containers alive until user hits Ctrl-C
    while true; do sleep 60; done
fi
