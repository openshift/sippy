#!/bin/bash
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
SERVICE_URL_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../mcp" && pwd)/serviceurl.py"

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
    SIPPY_DATABASE_DSN="$1" python3 "$SERVICE_URL_SCRIPT" validate || exit $?
}

e2e_create_database() {
    db_name="$1"
    admin_dsn="$2"
    e2e_safe_db_name "$db_name"
    e2e_safe_dsn_host "$admin_dsn"
    echo "Dropping e2e database if it exists: $db_name"
    psql "$admin_dsn" -X -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $db_name" || return $?
    echo "Creating e2e database: $db_name"
    psql "$admin_dsn" -X -v ON_ERROR_STOP=1 -c "CREATE DATABASE $db_name"
}

e2e_drop_database() {
    db_name="$1"
    admin_dsn="$2"
    e2e_safe_db_name "$db_name"
    e2e_safe_dsn_host "$admin_dsn"
    echo "Dropping e2e database: $db_name"
    psql "$admin_dsn" -X -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $db_name" 2>/dev/null || true
}

e2e_configure_existing_services() {
    ADMIN_DSN="${SIPPY_DATABASE_DSN:-postgresql://postgres:password@sippy-postgres:5432/postgres}"
    ADMIN_DSN=$(SIPPY_DATABASE_DSN="$ADMIN_DSN" python3 "$SERVICE_URL_SCRIPT" database) || return $?
    # URI authority/path must win over inherited libpq target overrides as well.
    unset PGHOSTADDR PGSERVICE
    # Preserve connection options (including sslmode) when changing databases.
    local admin_base="${ADMIN_DSN%%\?*}"
    local admin_query="${ADMIN_DSN#"$admin_base"}"
    local admin_authority="${admin_base#*://}"
    if [[ "$admin_authority" == */* ]]; then
        admin_base="${admin_base%/*}"
    fi
    E2E_DSN="${admin_base}/${E2E_DB_NAME}${admin_query}"

    e2e_create_database "$E2E_DB_NAME" "$ADMIN_DSN" || return $?

    export SIPPY_E2E_DSN="$E2E_DSN"
    # Redis logical DB 1: dev `sippy serve` uses default DB 0; e2e must not share that keyspace.
    # Override with SIPPY_E2E_REDIS_URL if Redis is not sippy-redis:6379.
    export REDIS_URL="${SIPPY_E2E_REDIS_URL:-redis://sippy-redis:6379/1}"
    export SIPPY_E2E_REPO_ROOT="$(pwd)"
}

e2e_redact_logs() {
    python3 -u "$SERVICE_URL_SCRIPT" redact
}

e2e_log_tail() {
    echo "Last API log lines ($OUTPUT_DIR/e2e.log):" >&2
    tail -n 60 "$OUTPUT_DIR/e2e.log" >&2
}

e2e_prime_component_readiness_cache() {
    # Prior runs can leave synthetic triage fixtures in these reports for an hour.
    echo "Priming component readiness cache..."
    local views view
    views=$(curl -sf "http://localhost:$SIPPY_API_PORT/api/component_readiness/views") || { echo "Failed to fetch views"; return 1; }
    for view in $(echo "$views" | grep -o '"name":"[^"]*"' | cut -d'"' -f4); do
        echo "  Priming cache for view: $view"
        curl -sf "http://localhost:$SIPPY_API_PORT/api/component_readiness?view=$view&forceRefresh=true" > /dev/null || { echo "Failed to prime cache for view: $view"; return 1; }
    done
    echo "Cache priming complete"
}

wait_for_sippy() {
    echo "Waiting for sippy API to start on port $SIPPY_API_PORT..."
    local started=$SECONDS deadline=$((SECONDS + 600)) status remaining
    while [ "$SECONDS" -lt "$deadline" ]; do
        if ! kill -0 "$CHILD_PID" 2>/dev/null; then
            wait "$CHILD_PID"
            status=$?
            echo "Sippy API exited before readiness (exit $status)" >&2
            if [ -n "${API_LOG_PID:-}" ]; then
                wait "$API_LOG_PID"
            fi
            e2e_log_tail
            [ "$status" -ne 0 ] || status=1
            return "$status"
        fi
        # Capabilities needs no release filters, unlike /api/health.
        remaining=$((deadline - SECONDS))
        [ "$remaining" -gt 0 ] || break
        if curl -fsS --max-time "$((remaining < 2 ? remaining : 2))" "http://localhost:$SIPPY_API_PORT/api/capabilities" > /dev/null 2>&1; then
            echo "Sippy API is ready after $((SECONDS - started))s"
            return 0
        fi
        remaining=$((deadline - SECONDS))
        if [ "$remaining" -gt 0 ]; then
            sleep "$((remaining < 2 ? remaining : 2))"
        fi
    done
    echo "Timeout waiting for sippy API to start after 600s" >&2
    e2e_log_tail
    return 1
}

clean_up() {
    local status=$?
    trap - EXIT
    # Cleanup must not replace the failure that triggered it.
    set +e
    if [ -n "${CHILD_PID:-}" ]; then
        echo "Stopping sippy API child process: $CHILD_PID"
        kill "$CHILD_PID" 2>/dev/null
        wait "$CHILD_PID" 2>/dev/null
    fi
    if [ -n "${API_LOG_PID:-}" ]; then
        if [ -z "${CHILD_PID:-}" ]; then
            kill "$API_LOG_PID" 2>/dev/null
        fi
        wait "$API_LOG_PID"
    fi
    if [ -n "${API_LOG_DIR:-}" ]; then
        rm -f "$API_LOG_DIR/api.fifo"
        rmdir "$API_LOG_DIR"
    fi
    if [ -d "${COVDIR:-}" ] && find "$COVDIR" -name 'covcounters.*' -print -quit | grep -q .; then
        echo "Generating coverage report..."
        go tool covdata percent -i="$COVDIR"
        go tool covdata textfmt -i="$COVDIR" -o="$OUTPUT_DIR/e2e-coverage.out"
        for f in "$OUTPUT_DIR/e2e-test-coverage.out" e2e-bq-test-coverage.out unit-test-coverage.out; do
            if [ -f "$f" ]; then
                tail -n +2 "$f" >> "$OUTPUT_DIR/e2e-coverage.out"
                rm -f "$f"
            fi
        done
    fi
    if [ "${EXISTING_SERVICES:-false}" = true ]; then
        if [ -n "${ADMIN_DSN:-}" ]; then
            e2e_drop_database "$E2E_DB_NAME" "$ADMIN_DSN"
        fi
    elif [ "${HOST_CONTAINERS:-false}" = true ]; then
        "$DOCKER" stop -i "$PSQL_CONTAINER"
        "$DOCKER" rm -i "$PSQL_CONTAINER"
        "$DOCKER" stop -i "$REDIS_CONTAINER"
        "$DOCKER" rm -i "$REDIS_CONTAINER"
    fi
    if [ -f "$OUTPUT_DIR/e2e-gotestsum.log" ]; then
        grep -E '^(DONE|FAIL)' "$OUTPUT_DIR/e2e-gotestsum.log" | tail -1
    fi
    exit "$status"
}

if [ "${BASH_SOURCE[0]}" != "$0" ]; then
    return 0
fi

set -o pipefail
OUTPUT_DIR="${SIPPY_TEST_OUTPUT_DIR:-$(pwd)}"
mkdir -p "$OUTPUT_DIR" || exit 1
rm -f "$OUTPUT_DIR/e2e-gotestsum.log" || exit $?
trap clean_up EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -z "$GCS_SA_JSON_PATH" ]; then
    echo "WARNING: GCS_SA_JSON_PATH not set, data sync and BigQuery tests will be skipped" >&2
fi

if [ -f /run/.containerenv ]; then
    echo "Detected devcontainer, using existing PostgreSQL and Redis services"
    EXISTING_SERVICES=true
    e2e_configure_existing_services || { status=$?; echo "Database setup failed" >&2; exit "$status"; }
else
    HOST_CONTAINERS=true
    # make sure no old containers are running
    echo "Cleaning up old sippy postgresql container if present"
    $DOCKER stop -i $PSQL_CONTAINER
    $DOCKER rm -i $PSQL_CONTAINER
    echo "Cleaning up old sippy redis container if present"
    $DOCKER stop -i $REDIS_CONTAINER
    $DOCKER rm -i $REDIS_CONTAINER

    # start postgresql in a container:
    echo "Starting new sippy postgresql container: $PSQL_CONTAINER"
    $DOCKER run --name $PSQL_CONTAINER -e POSTGRES_PASSWORD=password -p $PSQL_PORT:5432 -d docker.io/library/postgres:18.4 || exit $?

    # start redis in a container:
    echo "Starting new sippy redis container: $REDIS_CONTAINER"
    $DOCKER run --name $REDIS_CONTAINER -p $REDIS_PORT:6379 -d quay.io/openshiftci/redis:latest || exit $?

    echo "Wait 5s for postgresql and redis to start..."
    sleep 5

    export SIPPY_E2E_DSN="postgresql://postgres:password@localhost:$PSQL_PORT/postgres"
    export REDIS_URL="redis://localhost:$REDIS_PORT"
    export SIPPY_E2E_REPO_ROOT="$(pwd)"
fi

# Build with coverage instrumentation
COVDIR="$OUTPUT_DIR/e2e-coverage"
rm -rf "$COVDIR" || exit $?
mkdir -p "$COVDIR" || exit $?
echo "Building sippy with coverage instrumentation..."
go build -cover -coverpkg=./cmd/...,./pkg/... -mod vendor -o ./sippy ./cmd/sippy || { echo "Build failed"; exit 1; }

echo "Loading database..."
GOCOVERDIR="$COVDIR" ./sippy seed-data  \
  --init-database \
  --database-dsn="$SIPPY_E2E_DSN" 2>&1 | e2e_redact_logs | tee "$OUTPUT_DIR/e2e-seed.log"
status=$?
if [ "$status" -ne 0 ]; then
    echo "Database seeding failed (exit $status)" >&2
    exit "$status"
fi

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

# Force DEV_MODE off: callers such as hack/agentic_setup.sh may export DEV_MODE=1, which bypasses the
# component report cache that triage e2e fixtures inject synthetic regressions into. e2e requests
# already send X-Forwarded-User, so the mock user is not needed.
# A regular background child is waitable on macOS's system Bash 3.2.
# Artifact mounts and inherited TMPDIR can be virtiofs, which cannot create FIFOs.
# Only the private pipe lives on the system temporary filesystem; logs stay in OUTPUT_DIR.
API_LOG_DIR=$(mktemp -d /tmp/sippy-api-log.XXXXXX) || exit $?
mkfifo "$API_LOG_DIR/api.fifo" || exit $?
# Defer signal exits until each launched child's PID is recorded for cleanup.
API_START_STATUS=0
trap 'API_START_STATUS=130' INT
trap 'API_START_STATUS=143' TERM
e2e_redact_logs < "$API_LOG_DIR/api.fifo" > "$OUTPUT_DIR/e2e.log" &
API_LOG_PID=$!
if [ "$API_START_STATUS" -eq 0 ]; then
    DEV_MODE=0 GOCOVERDIR="$COVDIR" ./sippy serve "$@" > "$API_LOG_DIR/api.fifo" 2>&1 &
    CHILD_PID=$!
fi
trap 'exit 130' INT
trap 'exit 143' TERM
[ "$API_START_STATUS" -eq 0 ] || exit "$API_START_STATUS"

wait_for_sippy || exit $?

# Prime fresh reports so triage tests can inject fixtures without reusing prior runs.
e2e_prime_component_readiness_cache || exit $?

# Run e2e tests
gotestsum ./test/e2e/... -count 1 -p 1 -coverprofile="$OUTPUT_DIR/e2e-test-coverage.out" -coverpkg=./pkg/...,./cmd/... 2>&1 | e2e_redact_logs | tee "$OUTPUT_DIR/e2e-gotestsum.log"
exit "$?"
