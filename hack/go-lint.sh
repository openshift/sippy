#!/bin/bash

set -ex

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "Running golangci-lint..."
go version
golangci-lint version -v
golangci-lint "${@}"
