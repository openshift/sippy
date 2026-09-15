#!/bin/bash
# Run integration tests with automatic PostgreSQL setup.
#
# In a devcontainer (or CI with a sidecar Postgres), uses the existing
# PostgreSQL service. On a bare host, spins up a temporary Postgres
# container with Podman and tears it down on exit.
#
# Set SIPPY_INTEGRATION_DSN to skip container management and connect
# to an already-running Postgres instance directly.

set -euo pipefail

DOCKER="podman"
PSQL_CONTAINER="sippy-integration-test-postgresql"
PSQL_PORT="23434"

EXIT_CODE=0

# ---------------------------------------------------------------------------
# If the caller already provided a DSN, just run the tests.
# ---------------------------------------------------------------------------
if [ -n "${SIPPY_INTEGRATION_DSN:-}" ]; then
    echo "Using caller-provided SIPPY_INTEGRATION_DSN"
    make integration
    exit $?
fi

# ---------------------------------------------------------------------------
# Devcontainer vs host setup
# ---------------------------------------------------------------------------
if [ -f /run/.containerenv ]; then
    # Inside devcontainer: reuse the existing PostgreSQL service.
    ADMIN_DSN="${SIPPY_DATABASE_DSN:-postgresql://postgres:password@sippy-postgres:5432/postgres}"
    export SIPPY_INTEGRATION_DSN="$ADMIN_DSN"
    echo "Detected devcontainer — using existing PostgreSQL"
    echo "  DSN: ${SIPPY_INTEGRATION_DSN%%@*}@***"

    clean_up() {
        ARG=$?
        [ $ARG -ne 0 ] && EXIT_CODE=$ARG
        exit $EXIT_CODE
    }
    trap clean_up EXIT
else
    # On host: start a temporary Postgres container.
    clean_up() {
        ARG=$?
        [ $ARG -ne 0 ] && EXIT_CODE=$ARG
        echo "Tearing down container $PSQL_CONTAINER"
        $DOCKER stop -i $PSQL_CONTAINER 2>/dev/null || true
        $DOCKER rm -i $PSQL_CONTAINER 2>/dev/null || true
        exit $EXIT_CODE
    }
    trap clean_up EXIT

    echo "Cleaning up old integration test container if present"
    $DOCKER stop -i $PSQL_CONTAINER 2>/dev/null || true
    $DOCKER rm -i $PSQL_CONTAINER 2>/dev/null || true

    echo "Starting PostgreSQL container: $PSQL_CONTAINER"
    $DOCKER run --name $PSQL_CONTAINER \
        -e POSTGRES_PASSWORD=password \
        -p $PSQL_PORT:5432 \
        -d quay.io/enterprisedb/postgresql

    echo "Waiting for PostgreSQL to start..."
    TIMEOUT=30
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if $DOCKER exec $PSQL_CONTAINER pg_isready -U postgres > /dev/null 2>&1; then
            echo "PostgreSQL is ready after ${ELAPSED}s"
            break
        fi
        sleep 1
        ELAPSED=$((ELAPSED + 1))
    done
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "Timeout waiting for PostgreSQL to start"
        exit 1
    fi

    export SIPPY_INTEGRATION_DSN="postgresql://postgres:password@localhost:$PSQL_PORT/postgres"
    echo "  DSN: ${SIPPY_INTEGRATION_DSN%%@*}@***"
fi

make integration
EXIT_CODE=$?
