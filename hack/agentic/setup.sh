#!/bin/bash
set -euo pipefail

# Sippy-specific environment setup for agentic CI workflows.
# Called by the generic TRT agentic workflow after workspace init.

export SIPPY_DATABASE_DSN="postgresql://postgres@localhost:5432/postgres?sslmode=disable"
export SIPPY_SEED_DATABASE_DSN="${SIPPY_DATABASE_DSN}"
export SIPPY_PRODLIKE_DATABASE_DSN="postgresql://postgres@localhost:5432/prodlike?sslmode=disable"
export REDIS_URL="redis://localhost:6379"

echo "Starting services..."
.devcontainer/init-services.sh

echo "Running post-create setup..."
.devcontainer/post-create.sh
