#!/bin/bash

# Use this script for running the sippy-e2e tests locally.
# Examples:
#
#   When using a private registry (where auth.json is one of ~/.docker/config.json for Docker or
#   ${XDG_RUNTIME_DIR}/containers/auth.json for Podman):
#     SIPPY_IMAGE=quay.io/username/sippy GCS_CRED=/path/to/cred.json DOCKERCONFIGJSON=auth.json e2e-scripts/run-e2e.sh
#
#   When using a public registry (where authentication is not needed):
#     SIPPY_IMAGE=quay.io/username/sippy GCS_CRED=/path/to/cred.json e2e-scripts/run-e2e.sh
#
#   When using a local registry where you have loaded the image and don't want to pull remotely
#     (Implies something like: podman build -t sippy . && kind load docker-image sippy):
#
#     SKIP_BUILD=1 SIPPY_IMAGE=localhost/sippy SIPPY_IMAGE_PULL_POLICY=IfNotPresent GCS_CRED=/path/to/cred.json e2e-scripts/run-e2e.sh
#
#   When you need more time to load sippy data from backup:
#     SIPPY_LOAD_TIMEOUT=600s SKIP_BUILD=1 SIPPY_IMAGE=localhost/sippy SIPPY_IMAGE_PULL_POLICY=IfNotPresent GCS_CRED=/path/to/cred.json e2e-scripts/run-e2e.sh


# Print out the current kube context
echo "The cluster context is: $(kubectl config current-context)"

# The sippy executable needs to be in a container image; set SIPPY_IMAGE that the image spec.
if [ -z ${SIPPY_IMAGE} ]; then
  echo "Aborting: Set SIPPY_IMAGE to a valid image pull spec."
  echo "  Example: SIPPY_IMAGE=quay.io/username/sippy:1.0"
  exit 1
fi

# GCS credentials should be read-only for safety.
if [ -z ${GCS_CRED} ]; then
  echo "Aborting: Set GCS_CRED to a valid file where your GCS read-only credentials are located."
  echo "  Example: GCS_CRED=/path/to/gcs-readonly.json"
  exit 1
fi

if [ ! -f ${GCS_CRED} ]; then
  echo "Aborting: Missing GCS credentials file; file not found: ${GCS_CRED}"
fi

# The artifacts directory is for logs and can be any directory you have access to.
export ARTIFACT_DIR="${ARTIFACT_DIR:=/tmp/sippy_artifacts}"
mkdir -p $ARTIFACT_DIR

if [ -z ${DOCKERCONFIGJSON} ]; then
  # Create an empty registry auth file in case we're using public container images that
  # don't need authentication.
  echo "DOCKERCONFIGJSON is not set; assuming public container image registry"
  echo '{ "auths": {} }' > /tmp/empty_auth.json
  export DOCKERCONFIGJSON=/tmp/empty_auth.json
fi
echo "DOCKERCONFIGJSON: ${DOCKERCONFIGJSON}"
if [ ! -f ${DOCKERCONFIGJSON} ]; then
  echo "Aborting: File does not exist: ${DOCKERCONFIGJSON}"
  exit 1
fi

if [ -z ${SKIP_BUILD} ]; then
  SKIP_BUILD=0
fi

# Ensure you have kubectl and that it's installed and on your path
export KUBECTL_CMD=kubectl

set -euo pipefail

if [ $(${KUBECTL_CMD} get ns|grep postgres|wc -l) -gt 0 ]; then
  ${KUBECTL_CMD} delete ns postgres
fi

if [[ ${SKIP_BUILD} == "1" ]]; then
  echo "Skipping build"
else
  echo "Building docker image for ${SIPPY_IMAGE} .."
  docker build -t ${SIPPY_IMAGE} .
  echo "Pushing docker image for ${SIPPY_IMAGE} .."
  docker push ${SIPPY_IMAGE}
fi

e2e-scripts/sippy-e2e-sippy-e2e-setup-commands.sh
e2e-scripts/sippy-e2e-sippy-e2e-test-commands.sh

# Cleanup as needed
${KUBECTL_CMD} delete ns postgres
