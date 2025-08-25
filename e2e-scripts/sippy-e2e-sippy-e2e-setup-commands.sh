#!/bin/bash

# In Prow CI, SIPPY_IMAGE variable is defined in the sippy-e2e-ref.yaml file as a
# dependency so that the pipeline:sippy image (containing the sippy binary)
# will be available to start the sippy-load and sippy-server pods.
# When running locally, the user has to define SIPPY_IMAGE.
echo "The sippy CI image: ${SIPPY_IMAGE}"

# The GCS_CRED allows us to pull artifacts from GCS when importing prow jobs.
# Redefine GCS_CRED to use your own.
GCS_CRED="${GCS_CRED:=/var/run/sippy-bigquery-job-importer/gcs-sa}"
echo "The GCS cred is: ${GCS_CRED}"

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

echo "Checking for presense of GCS credentials ..."
if [ -f ${GCS_CRED} ]; then
  ls -l ${GCS_CRED}
else
  echo "Aborting: GCS credential file ${GCS_CRED} not found"
  exit 1
fi

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
set -e
echo
echo "Saving postgres logs ..."
${KUBECTL_CMD} -n sippy-e2e logs postg1 > ${ARTIFACT_DIR}/postgres.log
echo "Saving redis logs ..."
${KUBECTL_CMD} -n sippy-e2e logs redis1 > ${ARTIFACT_DIR}/redis.log
if [ ${postgres_retVal} -ne 0 ]; then
  echo "Postgres pod never came up"
  exit 1
fi
if [ ${redis_retVal} -ne 0 ]; then
  echo "Redis pod never came up"
  exit 1
fi

${KUBECTL_CMD} -n sippy-e2e get po -o wide
${KUBECTL_CMD} -n sippy-e2e get svc,ep

# Get the gcs credentials out to the cluster-pool cluster.
# These credentials are in vault and maintained by the TRT team (e.g. for updates and rotations).
# See https://vault.ci.openshift.org/ui/vault/secrets/kv/show/selfservice/technical-release-team/sippy-ci-gcs-read-sa
#

${KUBECTL_CMD} create secret generic gcs-cred --from-file gcs-cred=$GCS_CRED -n sippy-e2e

# Get the registry credentials for all build farm clusters out to the cluster-pool cluster.
${KUBECTL_CMD} -n sippy-e2e create secret generic regcred --from-file=.dockerconfigjson=${DOCKERCONFIGJSON} --type=kubernetes.io/dockerconfigjson

# Make the "sippy loader" pod.
cat << END | ${KUBECTL_CMD} apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: sippy-load-job
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
            memory: 3G
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        command:  ["/bin/sh", "-c"]
        args:
          - /bin/sippy load --init-database --log-level=debug --release 4.20 --database-dsn=postgresql://postgres:password@postgres.sippy-e2e.svc.cluster.local:5432/postgres --redis-url=redis://redis.sippy-e2e.svc.cluster.local:6379 --mode=ocp --config ./config/e2e-openshift.yaml --google-service-account-credential-file /tmp/secrets/gcs-cred
        env:
        - name: GCS_SA_JSON_PATH
          value: /tmp/secrets/gcs-cred
        volumeMounts:
        - mountPath: /tmp/secrets
          name: gcs-cred
          readOnly: true
      imagePullSecrets:
      - name: regcred
      volumes:
        - name: gcs-cred
          secret:
            secretName: gcs-cred
      dnsPolicy: ClusterFirst
      restartPolicy: Never
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
  backoffLimit: 1
END

date
echo "Waiting for sippy loader job to finish ..."
${KUBECTL_CMD} -n sippy-e2e get job sippy-load-job
${KUBECTL_CMD} -n sippy-e2e describe job sippy-load-job

# We set +e to avoid the script aborting before we can retrieve logs.
set +e

echo "Waiting up to ${SIPPY_LOAD_TIMEOUT:=1200s} for the sippy-load-job to complete..."
${KUBECTL_CMD} -n sippy-e2e wait --for=condition=complete job/sippy-load-job --timeout ${SIPPY_LOAD_TIMEOUT}
retVal=$?
set -e

job_pod=$(${KUBECTL_CMD} -n sippy-e2e get pod --selector=job-name=sippy-load-job --output=jsonpath='{.items[0].metadata.name}')
${KUBECTL_CMD} -n sippy-e2e logs ${job_pod} > ${ARTIFACT_DIR}/sippy-load.log

if [ ${retVal} -ne 0 ]; then
  echo "sippy loading never finished on time."
  exit 1
fi

date
