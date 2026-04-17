#!/bin/bash

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

echo "The Docker config.json is: ${DOCKERCONFIGJSON}"

is_ready=0
echo "Waiting for cluster to be usable..."

e2e_pause() {
  if [ -z $OPENSHIFT_CI ]; then
    return
  fi

  # In prow, we need these sleeps to keep things consistent -- TODO: we need to figure out why.
  echo "Sleeping 30 seconds ..."
  sleep 30
}

set +e
# We don't want to exit on timeouts if the cluster we got was not quite ready yet.
for i in `seq 1 20`; do
  echo -n "${i})"
  e2e_pause
  echo "Checking cluster nodes"
  ${KUBECTL_CMD} get node
  if [ $? -eq 0 ]; then
    echo "Cluster looks ready"
    is_ready=1
    break
  fi
  echo "Cluster not ready yet..."
done
set -e

# This should be set to the KUBECONFIG for the cluster claimed from the cluster-pool.
echo "KUBECONFIG=${KUBECONFIG}"

echo "Showing kube context"
${KUBECTL_CMD} config current-context

if [ $is_ready -eq 0 ]; then
  echo "Cluster never became ready aborting"
  exit 1
fi

e2e_pause

echo "Starting postgres on cluster-pool cluster..."

# Make the "postgres" namespace and pod.
cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: sippy-e2e
  labels:
    openshift.io/run-level: "0"
    openshift.io/cluster-monitoring: "true"
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/warn: privileged
END

e2e_pause

cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: postg1
  namespace: sippy-e2e
  labels:
    app: postgres
spec:
  volumes:
    - name: postgredb
      emptyDir: {}
  containers:
  - name: postgres
    image: quay.io/enterprisedb/postgresql
    ports:
    - containerPort: 5432
    env:
    - name: POSTGRES_PASSWORD
      value: password
    - name: POSTGRESQL_DATABASE
      value: postgres
    volumeMounts:
      - mountPath: /var/lib/postgresql/data
        name: postgredb
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      runAsNonRoot: true
      runAsUser: 3
      seccompProfile:
        type: RuntimeDefault
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: postgres
  name: postgres
  namespace: sippy-e2e
spec:
  ports:
  - name: postgres
    port: 5432
    protocol: TCP
  selector:
    app: postgres
END

e2e_pause

echo "Starting redis on cluster-pool cluster..."

cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: redis1
  namespace: sippy-e2e
  labels:
    app: redis
spec:
  containers:
  - name: redis
    image: quay.io/openshiftci/redis:latest
    ports:
    - containerPort: 6379
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      runAsNonRoot: true
      runAsUser: 999
      seccompProfile:
        type: RuntimeDefault
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: redis
  name: redis
  namespace: sippy-e2e
spec:
  ports:
  - name: redis
    port: 6379
    protocol: TCP
  selector:
    app: redis
END

echo "Waiting for postgres and redis pods to be Ready ..."

# We set +e to avoid the script aborting before we can retrieve logs.
set +e
TIMEOUT=120s
echo "Waiting up to ${TIMEOUT} for the postgres and redis to come up..."
${KUBECTL_CMD} -n sippy-e2e wait --for=condition=Ready pod/postg1 --timeout=${TIMEOUT}
postgres_retVal=$?
${KUBECTL_CMD} -n sippy-e2e wait --for=condition=Ready pod/redis1 --timeout=${TIMEOUT}
redis_retVal=$?

echo
echo "=== Pod status ==="
${KUBECTL_CMD} -n sippy-e2e get po -o wide
echo

echo "Saving postgres logs ..."
${KUBECTL_CMD} -n sippy-e2e logs postg1 > ${ARTIFACT_DIR}/postgres.log 2>&1
echo "Saving redis logs ..."
${KUBECTL_CMD} -n sippy-e2e logs redis1 > ${ARTIFACT_DIR}/redis.log 2>&1

if [ ${postgres_retVal} -ne 0 ] || [ ${redis_retVal} -ne 0 ]; then
  echo
  echo "=== FAILURE DIAGNOSTICS ==="
  echo

  echo "=== Pod descriptions ==="
  ${KUBECTL_CMD} -n sippy-e2e describe pod/postg1
  echo "---"
  ${KUBECTL_CMD} -n sippy-e2e describe pod/redis1
  echo

  echo "=== Namespace events (sorted by time) ==="
  ${KUBECTL_CMD} -n sippy-e2e get events --sort-by='.lastTimestamp'
  echo

  echo "=== Pod conditions ==="
  ${KUBECTL_CMD} -n sippy-e2e get pod postg1 -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}' 2>/dev/null
  echo "---"
  ${KUBECTL_CMD} -n sippy-e2e get pod redis1 -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}' 2>/dev/null
  echo

  echo "=== Container statuses ==="
  ${KUBECTL_CMD} -n sippy-e2e get pod postg1 -o jsonpath='{range .status.containerStatuses[*]}name={.name} ready={.ready} state={.state}{"\n"}{end}' 2>/dev/null
  echo "---"
  ${KUBECTL_CMD} -n sippy-e2e get pod redis1 -o jsonpath='{range .status.containerStatuses[*]}name={.name} ready={.ready} state={.state}{"\n"}{end}' 2>/dev/null
  echo

  echo "=== Node status for scheduled nodes ==="
  postg1_node=$(${KUBECTL_CMD} -n sippy-e2e get pod postg1 -o jsonpath='{.spec.nodeName}' 2>/dev/null)
  redis1_node=$(${KUBECTL_CMD} -n sippy-e2e get pod redis1 -o jsonpath='{.spec.nodeName}' 2>/dev/null)
  for node in ${postg1_node} ${redis1_node}; do
    if [ -n "${node}" ]; then
      echo "Node ${node} conditions:"
      ${KUBECTL_CMD} get node ${node} -o jsonpath='{range .status.conditions[*]}  {.type}={.status} message={.message}{"\n"}{end}' 2>/dev/null
    fi
  done
  echo

  echo "=== END FAILURE DIAGNOSTICS ==="
fi
set -e

if [ ${postgres_retVal} -ne 0 ]; then
  echo "ERROR: Postgres pod never became Ready (timed out after ${TIMEOUT})"
  exit 1
fi
if [ ${redis_retVal} -ne 0 ]; then
  echo "ERROR: Redis pod never became Ready (timed out after ${TIMEOUT})"
  exit 1
fi

${KUBECTL_CMD} -n sippy-e2e get svc,ep

# Get the gcs credentials out to the cluster-pool cluster.
# These credentials are in vault and maintained by the TRT team (e.g. for updates and rotations).
# See https://vault.ci.openshift.org/ui/vault/secrets/kv/show/selfservice/technical-release-team/sippy-ci-gcs-read-sa
GCS_CRED="${GCS_CRED:=/var/run/sippy-bigquery-job-importer/gcs-sa}"
if [ -f "${GCS_CRED}" ]; then
  ${KUBECTL_CMD} create secret generic gcs-cred --from-file gcs-cred="${GCS_CRED}" -n sippy-e2e
else
  echo "WARNING: GCS credential file ${GCS_CRED} not found, BigQuery tests will fail"
fi

# Get the registry credentials for all build farm clusters out to the cluster-pool cluster.
${KUBECTL_CMD} -n sippy-e2e create secret generic regcred --from-file=.dockerconfigjson=${DOCKERCONFIGJSON} --type=kubernetes.io/dockerconfigjson

# Create a PVC for coverage data that outlives the server pod.
cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: sippy-coverage
  namespace: sippy-e2e
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Mi
END

# Seed synthetic data into postgres.
cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: sippy-seed-job
  namespace: sippy-e2e
spec:
  template:
    spec:
      containers:
      - name: sippy
        image: ${SIPPY_IMAGE}
        imagePullPolicy: ${SIPPY_IMAGE_PULL_POLICY:-Always}
        resources:
          limits:
            memory: 8G
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        command:  ["/bin/sh", "-c"]
        args:
          - /bin/sippy-cover seed-data --init-database --database-dsn=postgresql://postgres:password@postgres.sippy-e2e.svc.cluster.local:5432/postgres
        env:
        - name: GOCOVERDIR
          value: /tmp/coverage
        volumeMounts:
        - mountPath: /tmp/coverage
          name: coverage
      imagePullSecrets:
      - name: regcred
      volumes:
      - name: coverage
        persistentVolumeClaim:
          claimName: sippy-coverage
      dnsPolicy: ClusterFirst
      restartPolicy: Never
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
  backoffLimit: 1
END

date
echo "Waiting for sippy seed job to finish ..."
${KUBECTL_CMD} -n sippy-e2e get job sippy-seed-job
${KUBECTL_CMD} -n sippy-e2e describe job sippy-seed-job

# We set +e to avoid the script aborting before we can retrieve logs.
set +e

echo "Waiting up to ${SIPPY_LOAD_TIMEOUT:=1200s} for the sippy-seed-job to complete..."
${KUBECTL_CMD} -n sippy-e2e wait --for=condition=complete job/sippy-seed-job --timeout "${SIPPY_LOAD_TIMEOUT}"
retVal=$?
set -e

job_pod=$(${KUBECTL_CMD} -n sippy-e2e get pod --selector=job-name=sippy-seed-job --output=jsonpath='{.items[0].metadata.name}')
${KUBECTL_CMD} -n sippy-e2e logs "${job_pod}" > "${ARTIFACT_DIR}/sippy-seed.log" 2>&1

if [ ${retVal} -ne 0 ]; then
  echo
  echo "=== SIPPY SEED JOB FAILURE DIAGNOSTICS ==="
  echo "=== Job status ==="
  ${KUBECTL_CMD} -n sippy-e2e describe job sippy-seed-job
  echo "=== Job pod status ==="
  ${KUBECTL_CMD} -n sippy-e2e describe pod ${job_pod}
  echo "=== Recent namespace events ==="
  ${KUBECTL_CMD} -n sippy-e2e get events --sort-by='.lastTimestamp'
  echo "=== END SIPPY SEED JOB FAILURE DIAGNOSTICS ==="
  echo
  echo "ERROR: sippy-seed-job did not complete within ${SIPPY_LOAD_TIMEOUT}"
  exit 1
fi

date
