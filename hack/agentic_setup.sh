#!/bin/bash
set -euo pipefail

# Sippy-specific environment setup for agentic CI workflows.
# Called by the generic TRT agentic workflow after workspace init.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export SIPPY_DATABASE_DSN="postgresql://postgres@localhost:5432/postgres?sslmode=disable"
export SIPPY_SEED_DATABASE_DSN="${SIPPY_DATABASE_DSN}"
export SIPPY_PRODLIKE_DATABASE_DSN="postgresql://postgres@localhost:5432/prodlike?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
export SIPPY_E2E_REDIS_URL="redis://localhost:6379/1"

echo "Starting services..."
"${REPO_ROOT}/.devcontainer/init-services.sh"

echo "Running post-create setup..."
SKIP_CLAUDE_INSTALL=true "${REPO_ROOT}/.devcontainer/post-create.sh"
