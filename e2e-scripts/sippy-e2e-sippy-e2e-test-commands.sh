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

# Launch the sippy api server pod.
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
        memory: 5Gi
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    command:
    - /bin/sippy
    args:
    - serve
    - --listen
    - ":8080"
    - --listen-metrics
    -  ":12112"
    - --database-dsn=postgresql://postgres:password@postgres.sippy-e2e.svc.cluster.local:5432/postgres
    - --redis-url=redis://redis.sippy-e2e.svc.cluster.local:6379
    - --data-provider
    - postgres
    - --log-level
    - debug
    - --enable-write-endpoints
    - --mode
    - ocp
    - --views
    - ./config/e2e-views.yaml
  imagePullSecrets:
  - name: regcred
  dnsPolicy: ClusterFirst
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {}
  terminationGracePeriodSeconds: 30
END

# The basic readiness probe will give us at least 10 seconds before declaring the pod as ready.
echo "Waiting for sippy api server pod to be Ready ..."
${KUBECTL_CMD} -n sippy-e2e wait --for=condition=Ready pod/sippy-server --timeout=600s

${KUBECTL_CMD} -n sippy-e2e get pod -o wide
${KUBECTL_CMD} -n sippy-e2e logs sippy-server > ${ARTIFACT_DIR}/sippy-server.log

echo "Setup services and port forwarding for the sippy api server ..."

export SIPPY_ENDPOINT="127.0.0.1"

# Random port between 18000 and 18500 so we don't collide with other test jobs
SIPPY_API_PORT=$((RANDOM % 501 + 18000))
export SIPPY_API_PORT

# Create the Kubernetes service for the sippy-server pod
# Setup port forward for random port to get to the sippy-server pod
${KUBECTL_CMD} -n sippy-e2e expose pod sippy-server
${KUBECTL_CMD} -n sippy-e2e port-forward pod/sippy-server ${SIPPY_API_PORT}:8080 &

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

${KUBECTL_CMD} -n sippy-e2e delete secret regcred

# only 1 in parallel, some tests will clash if run at the same time
gotestsum --junitfile ${ARTIFACT_DIR}/junit_e2e.xml -- ./test/e2e/... -v -p 1
