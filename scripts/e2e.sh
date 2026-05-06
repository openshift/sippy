#!/bin/sh
# Shell script meant for developers to run the e2e tests locally without impacting
# their running postgres container or sippy process.
# It's quite quick to import the older releases below, but in theory
# you can run these commands against your devel sippy process/db and just run the
# go test e2es for faster turnaround.

DOCKER="podman"
PSQL_CONTAINER="sippy-e2e-test-postgresql"
PSQL_PORT="23433"
REDIS_CONTAINER="sippy-e2e-test-redis"
REDIS_PORT="23479"

E2E_DB_NAME="sippy_e2e"

if [ -z "$GCS_SA_JSON_PATH" ]; then
    echo "WARNING: GCS_SA_JSON_PATH not set, data sync and BigQuery tests will be skipped" 1>&2
fi

E2E_EXIT_CODE=0

# ---------------------------------------------------------------------------
# Safety helpers for database create/drop
# ---------------------------------------------------------------------------
e2e_safe_db_name() {
    db_name="$1"
    case "$db_name" in
        *_e2e*) ;;
        *) echo "FATAL: refusing to operate on database '$db_name' — name must contain '_e2e'" >&2; exit 1 ;;
    esac
}

e2e_safe_dsn_host() {
    dsn="$1"
    case "$dsn" in
        *@localhost:*|*@localhost/*|*@sippy-postgres:*|*@sippy-postgres/*) ;;
        *) echo "FATAL: refusing to operate on non-local database: $dsn" >&2; exit 1 ;;
    esac
}

e2e_create_database() {
    db_name="$1"
    admin_dsn="$2"
    e2e_safe_db_name "$db_name"
    e2e_safe_dsn_host "$admin_dsn"
    echo "Dropping e2e database if it exists: $db_name"
    psql "$admin_dsn" -c "DROP DATABASE IF EXISTS $db_name"
    echo "Creating e2e database: $db_name"
    psql "$admin_dsn" -c "CREATE DATABASE $db_name"
}

e2e_drop_database() {
    db_name="$1"
    admin_dsn="$2"
    e2e_safe_db_name "$db_name"
    e2e_safe_dsn_host "$admin_dsn"
    echo "Dropping e2e database: $db_name"
    psql "$admin_dsn" -c "DROP DATABASE IF EXISTS $db_name" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Devcontainer vs host setup
# ---------------------------------------------------------------------------
if [ -f /run/.containerenv ]; then
    # Inside devcontainer — use existing PostgreSQL and Redis, separate database
    echo "Detected devcontainer — using existing PostgreSQL and Redis services"
    ADMIN_DSN="${SIPPY_DATABASE_DSN:-postgresql://postgres:password@sippy-postgres:5432/postgres}"
    E2E_DSN=$(echo "$ADMIN_DSN" | sed "s|/[^/]*$|/${E2E_DB_NAME}|")

    e2e_create_database "$E2E_DB_NAME" "$ADMIN_DSN"

    export SIPPY_E2E_DSN="$E2E_DSN"
    export REDIS_URL="${REDIS_URL:-redis://sippy-redis:6379}"
    export SIPPY_E2E_REPO_ROOT="$(pwd)"
    echo "E2E DSN: $SIPPY_E2E_DSN"
    echo "Redis:   $REDIS_URL"

    clean_up () {
        ARG=$?
        if [ $ARG -ne 0 ]; then
            E2E_EXIT_CODE=$ARG
        fi
        echo "Stopping sippy API child process: $CHILD_PID"
        kill $CHILD_PID 2>/dev/null && wait $CHILD_PID 2>/dev/null
        if [ -d "$COVDIR" ] && find "$COVDIR" -name 'covcounters.*' -print -quit | grep -q .; then
            echo "Generating coverage report..."
            go tool covdata percent -i="$COVDIR"
            go tool covdata textfmt -i="$COVDIR" -o=e2e-coverage.out
            for f in e2e-test-coverage.out e2e-bq-test-coverage.out unit-test-coverage.out; do
                if [ -f "$f" ]; then
                    echo "Merging $f into server coverage..."
                    tail -n +2 "$f" >> e2e-coverage.out
                    rm -f "$f"
                fi
            done
            echo "Coverage data written to e2e-coverage.out"
            echo "View HTML report: go tool cover -html=e2e-coverage.out -o=e2e-coverage.html"
        fi
        e2e_drop_database "$E2E_DB_NAME" "$ADMIN_DSN"
        exit $E2E_EXIT_CODE
    }
    trap clean_up EXIT
else
    # On host — start dedicated containers
    clean_up () {
        ARG=$?
        if [ $ARG -ne 0 ]; then
            E2E_EXIT_CODE=$ARG
        fi
        echo "Stopping sippy API child process: $CHILD_PID"
        kill $CHILD_PID 2>/dev/null && wait $CHILD_PID 2>/dev/null
        if [ -d "$COVDIR" ] && find "$COVDIR" -name 'covcounters.*' -print -quit | grep -q .; then
            echo "Generating coverage report..."
            go tool covdata percent -i="$COVDIR"
            go tool covdata textfmt -i="$COVDIR" -o=e2e-coverage.out
            for f in e2e-test-coverage.out e2e-bq-test-coverage.out unit-test-coverage.out; do
                if [ -f "$f" ]; then
                    echo "Merging $f into server coverage..."
                    tail -n +2 "$f" >> e2e-coverage.out
                    rm -f "$f"
                fi
            done
            echo "Coverage data written to e2e-coverage.out"
            echo "View HTML report: go tool cover -html=e2e-coverage.out -o=e2e-coverage.html"
        fi
        echo "Tearing down container $PSQL_CONTAINER"
        $DOCKER stop -i $PSQL_CONTAINER
        $DOCKER rm -i $PSQL_CONTAINER
        echo "Tearing down container $REDIS_CONTAINER"
        $DOCKER stop -i $REDIS_CONTAINER
        $DOCKER rm -i $REDIS_CONTAINER
        exit $E2E_EXIT_CODE
    }
    trap clean_up EXIT

    # make sure no old containers are running
    echo "Cleaning up old sippy postgresql container if present"
    $DOCKER stop -i $PSQL_CONTAINER
    $DOCKER rm -i $PSQL_CONTAINER
    echo "Cleaning up old sippy redis container if present"
    $DOCKER stop -i $REDIS_CONTAINER
    $DOCKER rm -i $REDIS_CONTAINER

    # start postgresql in a container:
    echo "Starting new sippy postgresql container: $PSQL_CONTAINER"
    $DOCKER run --name $PSQL_CONTAINER -e POSTGRES_PASSWORD=password -p $PSQL_PORT:5432 -d quay.io/enterprisedb/postgresql

    # start redis in a container:
    echo "Starting new sippy redis container: $REDIS_CONTAINER"
    $DOCKER run --name $REDIS_CONTAINER -p $REDIS_PORT:6379 -d quay.io/openshiftci/redis:latest

    echo "Wait 5s for postgresql and redis to start..."
    sleep 5

    export SIPPY_E2E_DSN="postgresql://postgres:password@localhost:$PSQL_PORT/postgres"
    export REDIS_URL="redis://localhost:$REDIS_PORT"
    export SIPPY_E2E_REPO_ROOT="$(pwd)"
fi

# Build with coverage instrumentation
COVDIR="$(pwd)/e2e-coverage"
rm -rf "$COVDIR"
mkdir -p "$COVDIR"
echo "Building sippy with coverage instrumentation..."
go build -cover -coverpkg=./cmd/...,./pkg/... -mod vendor -o ./sippy ./cmd/sippy

echo "Loading database..."
GOCOVERDIR="$COVDIR" ./sippy seed-data  \
  --init-database \
  --database-dsn="$SIPPY_E2E_DSN"

# Spawn sippy server off into a separate process:
export SIPPY_API_PORT="18080"
export SIPPY_ENDPOINT="127.0.0.1"

set -- \
  --listen ":$SIPPY_API_PORT" \
  --listen-metrics ":12112" \
  --database-dsn="$SIPPY_E2E_DSN" \
  --enable-write-endpoints \
  --log-level debug \
  --views config/seed-views.yaml \
  --redis-url="$REDIS_URL" \
  --data-provider postgres
if [ -n "$GCS_SA_JSON_PATH" ]; then
    set -- "$@" --google-service-account-credential-file "$GCS_SA_JSON_PATH"
fi

GOCOVERDIR="$COVDIR" ./sippy serve "$@" > e2e.log 2>&1 &
CHILD_PID=$!

wait_for_sippy() {
    echo "Waiting for sippy API to start on port $SIPPY_API_PORT..."
    TIMEOUT=600
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -s "http://localhost:$SIPPY_API_PORT/api/health" > /dev/null 2>&1; then
            echo "Sippy API is ready after ${ELAPSED}s"
            return 0
        fi
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    done
    echo "Timeout waiting for sippy API to start after ${TIMEOUT}s"
    return 1
}

wait_for_sippy || exit 1

# Prime the component readiness cache so triage tests can find cached reports
echo "Priming component readiness cache..."
VIEWS=$(curl -sf "http://localhost:$SIPPY_API_PORT/api/component_readiness/views") || { echo "Failed to fetch views"; exit 1; }
for VIEW in $(echo "$VIEWS" | grep -o '"name":"[^"]*"' | cut -d'"' -f4); do
    echo "  Priming cache for view: $VIEW"
    curl -sf "http://localhost:$SIPPY_API_PORT/api/component_readiness?view=$VIEW" > /dev/null || { echo "Failed to prime cache for view: $VIEW"; exit 1; }
done
echo "Cache priming complete"

# Run e2e tests
gotestsum ./test/e2e/... -count 1 -p 1 -coverprofile=e2e-test-coverage.out -coverpkg=./pkg/...,./cmd/...
E2E_EXIT_CODE=$?
