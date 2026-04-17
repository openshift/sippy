#!/bin/bash

set -x

function cleanup() {
  echo "Cleaning up port forward"
  pf_job=$(jobs -p)
  kill ${pf_job} && wait
  echo "Port forward is cleaned up"
}
trap cleanup EXIT

# In Prow CI, SIPPY_IMAGE variable is defined in the sippy-e2e-ref.yaml file as a
# dependency so that the pipeline:sippy image (containing the sippy binary)
# will be available to start the sippy-load and sippy-server pods.
# When running locally, the user has to define SIPPY_IMAGE.
echo "The sippy CI image: ${SIPPY_IMAGE}"

# If you're using Openshift, we use oc, if you're using plain Kubernetes,
# we use kubectl.
#
KUBECTL_CMD="${KUBECTL_CMD:=oc}"
echo "The kubectl command is: ${KUBECTL_CMD}"

# The datasync test runs sippy load as a k8s Job, so it needs these to create the pod.
export SIPPY_E2E_SIPPY_IMAGE="${SIPPY_IMAGE}"

launch_sippy_server() {
  local DATA_PROVIDER=$1
  local EXTRA_ARGS="${2:-}"

  cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: sippy-server
  namespace: sippy-e2e
  labels:
    app: sippy-server
spec:
  containers:
  - name: sippy-server
    image: ${SIPPY_IMAGE}
    imagePullPolicy: ${SIPPY_IMAGE_PULL_POLICY:-Always}
    ports:
    - name: www
      containerPort: 8080
      protocol: TCP
    - name: metrics
      containerPort: 12112
      protocol: TCP
    readinessProbe:
      exec:
        command:
        - echo
        - "Wait for a short time"
    resources:
      limits:
        memory: 8Gi
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    command:
    - /bin/sippy-cover
    args:
    - serve
    - --listen
    - ":8080"
    - --listen-metrics
    -  ":12112"
    - --database-dsn=postgresql://postgres:password@postgres.sippy-e2e.svc.cluster.local:5432/postgres
    - --redis-url=redis://redis.sippy-e2e.svc.cluster.local:6379
    - --data-provider
    - ${DATA_PROVIDER}
    - --log-level
    - debug
    - --enable-write-endpoints
    - --mode
    - ocp
    - --views
    - ./config/e2e-views.yaml
    - --google-service-account-credential-file
    - /tmp/secrets/gcs-cred
    env:
    - name: GCS_SA_JSON_PATH
      value: /tmp/secrets/gcs-cred
    - name: GOCOVERDIR
      value: /tmp/coverage
    volumeMounts:
    - mountPath: /tmp/secrets
      name: gcs-cred
      readOnly: true
    - mountPath: /tmp/coverage
      name: coverage
  imagePullSecrets:
  - name: regcred
  volumes:
    - name: gcs-cred
      secret:
        secretName: gcs-cred
    - name: coverage
      persistentVolumeClaim:
        claimName: sippy-coverage
  dnsPolicy: ClusterFirst
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  terminationGracePeriodSeconds: 30
END

  echo "Waiting for sippy api server pod (${DATA_PROVIDER}) to be Ready ..."
  set +e
  ${KUBECTL_CMD} -n sippy-e2e wait --for=condition=Ready pod/sippy-server --timeout=600s
  local retVal=$?
  set -e

  ${KUBECTL_CMD} -n sippy-e2e get pod -o wide
  ${KUBECTL_CMD} -n sippy-e2e logs sippy-server > ${ARTIFACT_DIR}/sippy-server-${DATA_PROVIDER}.log 2>&1

  if [ ${retVal} -ne 0 ]; then
    echo
    echo "=== SIPPY SERVER FAILURE DIAGNOSTICS (${DATA_PROVIDER}) ==="
    ${KUBECTL_CMD} -n sippy-e2e describe pod/sippy-server
    echo "=== Namespace events ==="
    ${KUBECTL_CMD} -n sippy-e2e get events --sort-by='.lastTimestamp'
    echo "=== END SIPPY SERVER FAILURE DIAGNOSTICS ==="
    echo
    echo "ERROR: sippy-server pod (${DATA_PROVIDER}) never became Ready (timed out after 600s)"
    return 1
  fi
  return 0
}

stop_sippy_server() {
  local DATA_PROVIDER=$1
  echo "Stopping sippy-server (${DATA_PROVIDER}) to flush coverage data..."
  ${KUBECTL_CMD} -n sippy-e2e logs sippy-server > ${ARTIFACT_DIR}/sippy-server-${DATA_PROVIDER}.log 2>&1 || true
  ${KUBECTL_CMD} -n sippy-e2e delete pod sippy-server --wait=true --timeout=60s || true
  ${KUBECTL_CMD} -n sippy-e2e delete svc sippy-server || true
}

# Phase 1: Launch postgres-backed server
launch_sippy_server postgres || exit 1

echo "Setup services and port forwarding for the sippy api server ..."

export SIPPY_ENDPOINT="127.0.0.1"

# Random port between 18000 and 18500 so we don't collide with other test jobs
SIPPY_API_PORT=$((RANDOM % 501 + 18000))
export SIPPY_API_PORT

# Create the Kubernetes service for the sippy-server pod
# Setup port forward for random port to get to the sippy-server pod
${KUBECTL_CMD} -n sippy-e2e expose pod sippy-server
${KUBECTL_CMD} -n sippy-e2e port-forward pod/sippy-server ${SIPPY_API_PORT}:8080 &
PF_PID_SERVER=$!

# Random port for postgres as well, between 18500 and 19000
# Direct postgres access is used for some e2e test to seed data and cleanup things we don't expose on the api,
# and to test gorm mappings.
SIPPY_PSQL_PORT=$((RANDOM % 501 + 18500))
export SIPPY_PSQL_PORT
export SIPPY_E2E_DSN="postgresql://postgres:password@localhost:${SIPPY_PSQL_PORT}/postgres"
echo $SIPPY_E2E_DSN
${KUBECTL_CMD} -n sippy-e2e expose pod postg1
${KUBECTL_CMD} -n sippy-e2e port-forward pod/postg1 ${SIPPY_PSQL_PORT}:5432 &

# Random port for redis as well, between 19000 and 19500
SIPPY_REDIS_PORT=$((RANDOM % 501 + 19000))
export SIPPY_REDIS_PORT
export REDIS_URL="redis://localhost:${SIPPY_REDIS_PORT}"
echo $REDIS_URL
${KUBECTL_CMD} -n sippy-e2e expose pod redis1
${KUBECTL_CMD} -n sippy-e2e port-forward pod/redis1 ${SIPPY_REDIS_PORT}:6379 &

${KUBECTL_CMD} -n sippy-e2e get svc,ep

E2E_EXIT_CODE=0

# Prime the component readiness cache so triage tests can find cached reports
echo "Priming component readiness cache..."
VIEWS=$(curl -sf "http://localhost:${SIPPY_API_PORT}/api/component_readiness/views") || { echo "Failed to fetch views"; exit 1; }
for VIEW in $(echo "$VIEWS" | jq -r '.[].name'); do
    echo "  Priming cache for view: $VIEW"
    curl -sf "http://localhost:${SIPPY_API_PORT}/api/component_readiness?view=$VIEW" > /dev/null || { echo "Failed to prime cache for view: $VIEW"; exit 1; }
done
echo "Cache priming complete"

echo "=== Phase 1: Running postgres-backed e2e tests ==="
gotestsum --junitfile ${ARTIFACT_DIR}/junit_e2e_postgres.xml -- \
  ./test/e2e/componentreadiness/postgres/... \
  ./test/e2e/componentreadiness/bugs/... \
  ./test/e2e/datasync/... \
  ./test/e2e/ \
  -v -p 1 -coverprofile=${ARTIFACT_DIR}/e2e-test-coverage.out -coverpkg=./pkg/...,./cmd/...
POSTGRES_EXIT=$?
if [ ${POSTGRES_EXIT} -ne 0 ]; then
    E2E_EXIT_CODE=${POSTGRES_EXIT}
fi

# Stop the postgres server, kill the port-forward, and restart with bigquery
kill ${PF_PID_SERVER} 2>/dev/null || true
stop_sippy_server postgres

# Phase 2: Launch bigquery-backed server
echo "=== Phase 2: Running BigQuery-backed e2e tests ==="
launch_sippy_server bigquery || exit 1

${KUBECTL_CMD} -n sippy-e2e expose pod sippy-server
${KUBECTL_CMD} -n sippy-e2e port-forward pod/sippy-server ${SIPPY_API_PORT}:8080 &
PF_PID_SERVER=$!

gotestsum --junitfile ${ARTIFACT_DIR}/junit_e2e_bigquery.xml -- \
  ./test/e2e/componentreadiness/bigquery/... \
  -v -p 1 -coverprofile=${ARTIFACT_DIR}/e2e-bq-test-coverage.out -coverpkg=./pkg/...,./cmd/...
BQ_EXIT=$?
if [ ${BQ_EXIT} -ne 0 ]; then
    E2E_EXIT_CODE=${BQ_EXIT}
fi

kill ${PF_PID_SERVER} 2>/dev/null || true
stop_sippy_server bigquery

echo "=== Running unit tests for coverage ==="
gotestsum --junitfile ${ARTIFACT_DIR}/junit_unit.xml -- \
  ./pkg/... \
  -v -coverprofile=${ARTIFACT_DIR}/unit-test-coverage.out -coverpkg=./pkg/...,./cmd/...
UNIT_EXIT=$?
if [ ${UNIT_EXIT} -ne 0 ]; then
    E2E_EXIT_CODE=${UNIT_EXIT}
fi

# Collect coverage data from both server runs
cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: coverage-helper
  namespace: sippy-e2e
spec:
  containers:
  - name: helper
    image: ${SIPPY_IMAGE}
    command: ["sleep", "300"]
    volumeMounts:
    - mountPath: /tmp/coverage
      name: coverage
      readOnly: true
  imagePullSecrets:
  - name: regcred
  volumes:
  - name: coverage
    persistentVolumeClaim:
      claimName: sippy-coverage
  restartPolicy: Never
END

${KUBECTL_CMD} -n sippy-e2e wait --for=condition=Ready pod/coverage-helper --timeout=60s

COVDIR=$(mktemp -d)
${KUBECTL_CMD} -n sippy-e2e cp coverage-helper:/tmp/coverage "${COVDIR}" -c helper || true

COVERAGE_ROOT=$(find "${COVDIR}" -name 'covcounters.*' -print -quit 2>/dev/null | xargs -r dirname)
COVERAGE_ROOT="${COVERAGE_ROOT:-${COVDIR}}"
if find "${COVERAGE_ROOT}" -name 'covcounters.*' -print -quit 2>/dev/null | grep -q .; then
    echo "Generating coverage report from ${COVERAGE_ROOT}..."
    go tool covdata percent -i="${COVERAGE_ROOT}"
    go tool covdata textfmt -i="${COVERAGE_ROOT}" -o="${ARTIFACT_DIR}/e2e-coverage.out"
    for f in ${ARTIFACT_DIR}/e2e-test-coverage.out ${ARTIFACT_DIR}/e2e-bq-test-coverage.out ${ARTIFACT_DIR}/unit-test-coverage.out; do
        if [ -f "$f" ]; then
            echo "Merging $f into server coverage..."
            tail -n +2 "$f" >> "${ARTIFACT_DIR}/e2e-coverage.out"
            rm -f "$f"
        fi
    done
    go tool cover -html="${ARTIFACT_DIR}/e2e-coverage.out" -o="${ARTIFACT_DIR}/e2e-coverage.html"
    echo "Coverage report written to ${ARTIFACT_DIR}/e2e-coverage.html"
else
    echo "WARNING: No coverage data found"
fi
rm -rf "${COVDIR}"

${KUBECTL_CMD} -n sippy-e2e delete secret regcred || true

exit ${E2E_EXIT_CODE}
