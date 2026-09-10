#!/bin/sh
set -eu
# Starts PostgreSQL and Redis as standalone Podman containers (runs on the host before the devcontainer starts)

# Ensure host directories exist before the container bind-mounts them.
mkdir -p "${HOME}/.config/gcloud" "${HOME}/.config/gh" "${HOME}/.claude"

podman network create sippy-net 2>/dev/null || true

podman start sippy-postgres 2>/dev/null || \
    podman run -d --name sippy-postgres \
        --network sippy-net \
        -e POSTGRESQL_ADMIN_PASSWORD=password \
        -p 127.0.0.1:5432:5432 \
        quay.io/openshift/ci:ci_postgresql_postgresql-18-c9s

podman start sippy-redis 2>/dev/null || \
    podman run -d --name sippy-redis \
        --network sippy-net \
        --restart=always \
        --memory=4g \
        -p 127.0.0.1:6379:6379 \
        quay.io/openshift/ci:ci_redis_redis-7-c9s

echo "Waiting for PostgreSQL..."
pg_ready=false
for i in $(seq 1 60); do
    if podman exec sippy-postgres pg_isready -U postgres >/dev/null 2>&1; then
        pg_ready=true
        break
    fi
    sleep 1
done
if [ "$pg_ready" = false ]; then
    echo "ERROR: PostgreSQL did not become ready within 60 seconds."
    exit 1
fi

echo "Creating prod-like database (prodlike)..."
podman exec -e PGPASSWORD=password sippy-postgres psql -U postgres -tc \
    "SELECT 1 FROM pg_database WHERE datname = 'prodlike'" \
    | grep -q 1 \
    || podman exec -e PGPASSWORD=password sippy-postgres psql -U postgres -c "CREATE DATABASE prodlike"

echo "Waiting for Redis..."
redis_ready=false
for i in $(seq 1 15); do
    if podman exec sippy-redis redis-cli -p 6379 PING >/dev/null 2>&1; then
        redis_ready=true
        break
    fi
    sleep 1
done
if [ "$redis_ready" = false ]; then
    echo "ERROR: Redis did not become ready within 15 seconds."
    exit 1
fi

echo "Services ready."
