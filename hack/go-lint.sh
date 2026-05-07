#!/bin/bash

set -ex

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

if command -v golangci-lint &>/dev/null; then
  go version
  golangci-lint version -v
  golangci-lint "${@}"
else
  DOCKER=${DOCKER:-podman}

  if ! command -v "$DOCKER" &>/dev/null; then
    echo "$DOCKER not found and golangci-lint not installed."
    exit 1
  fi

  VOLUME_OPTION=""
  if [[ "$(uname -s)" == "Linux" ]]; then
    VOLUME_OPTION=":z"
  fi

  $DOCKER run --rm \
    --volume "${PWD}:/go/src/github.com/openshift/sippy${VOLUME_OPTION}" \
    --workdir /go/src/github.com/openshift/sippy \
    docker.io/golangci/golangci-lint:v1.64.8 \
    golangci-lint --timeout 10m "${@}"
fi
