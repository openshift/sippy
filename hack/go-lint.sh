#!/bin/bash
# If CI is true, run golangci-lint directly. If we're on a developer's
# local machine, run golangci-lint from a container so we're ensuring
# a consistent environment.

set -ex

if [ "$CI" = "true" ];
then
  go version
  golangci-lint version -v
  golangci-lint --timeout 10m "${@}"
else
  DOCKER=${DOCKER:-podman}

  if ! which "$DOCKER" > /dev/null 2>&1;
  then
    echo "$DOCKER not found, please install."
    exit 1
  fi

  # Check if running on Linux
  VOLUME_OPTION=""
  if [[ "$(uname -s)" == "Linux" ]]; then
    VOLUME_OPTION=":z"
  fi

  $DOCKER run --rm \
    --volume "${PWD}:/go/src/github.com/openshift/sippy${VOLUME_OPTION}" \
    --workdir /go/src/github.com/openshift/sippy \
    docker.io/golangci/golangci-lint:v1.60.1 \
    golangci-lint --timeout 10m "${@}"
fi
