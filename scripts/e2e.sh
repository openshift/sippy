#!/bin/sh
# Shell script meant for developers to run the e2e tests locally without impacting
# their running postgres container or sippy process.
# It's quite quick to import the older releases below, but in theory
# you can run these commands against your devel sippy process/db and just run the
# go test e2es for faster turnaround.

DOCKER="podman"
PSQL_CONTAINER="sippy-e2e-test-postgresql"
PSQL_PORT="23433"
REDIS_CONTAINER="sippy-e2e-test-redis"
REDIS_PORT="23479"

if [[ -z "$GCS_SA_JSON_PATH" ]]; then
    echo "Must provide path to GCS credential in GCS_SA_JSON_PATH env var" 1>&2
    exit 1
fi


clean_up () {
    ARG=$?
    echo "Killing sippy API child process: $CHILD_PID"
	kill $CHILD_PID
	echo "Tearing down container $PSQL_CONTAINER"
	$DOCKER stop -i $PSQL_CONTAINER
	$DOCKER rm -i $PSQL_CONTAINER
	echo "Tearing down container $REDIS_CONTAINER"
	$DOCKER stop -i $REDIS_CONTAINER
	$DOCKER rm -i $REDIS_CONTAINER
    exit $ARG
}
trap clean_up EXIT

# make sure no old containers are running
echo "Cleaning up old sippy postgresql container if present"
$DOCKER stop -i $PSQL_CONTAINER
$DOCKER rm -i $PSQL_CONTAINER
echo "Cleaning up old sippy redis container if present"
$DOCKER stop -i $REDIS_CONTAINER
$DOCKER rm -i $REDIS_CONTAINER

# start postgresql in a container:
echo "Starting new sippy postgresql container: $PSQL_CONTAINER"
$DOCKER run --name $PSQL_CONTAINER -e POSTGRES_PASSWORD=password -p $PSQL_PORT:5432 -d quay.io/enterprisedb/postgresql

# start redis in a container:
echo "Starting new sippy redis container: $REDIS_CONTAINER"
$DOCKER run --name $REDIS_CONTAINER -p $REDIS_PORT:6379 -d quay.io/openshiftci/redis:latest

echo "Wait 5s for postgresql and redis to start..."
sleep 5

export SIPPY_E2E_DSN="postgresql://postgres:password@localhost:$PSQL_PORT/postgres"
export REDIS_URL="redis://localhost:$REDIS_PORT"

echo "Loading database..."
# use an old release here as they have very few job runs and thus import quickly, ~5 minutes
go build -mod vendor ./cmd/sippy
./sippy seed-data  \
  --init-database \
  --database-dsn="$SIPPY_E2E_DSN" \
  --release="4.14"

# Spawn sippy server off into a separate process:
export SIPPY_API_PORT="18080"
export SIPPY_ENDPOINT="127.0.0.1"
(
./sippy serve \
  --listen ":$SIPPY_API_PORT" \
  --listen-metrics ":12112" \
  --database-dsn="$SIPPY_E2E_DSN" \
  --enable-write-endpoints \
  --log-level debug \
  --views config/e2e-views.yaml \
  --google-service-account-credential-file $GCS_SA_JSON_PATH \
  --redis-url="$REDIS_URL" > e2e.log 2>&1
)&
# store the child process for cleanup
CHILD_PID=$!

# Give it time to start up, and fill the redis cache
echo "Waiting for sippy API to start on port $SIPPY_API_PORT, see e2e.log for output..."
TIMEOUT=600
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    if curl -s "http://localhost:$SIPPY_API_PORT/api/health" > /dev/null 2>&1; then
        echo "Sippy API is ready after ${ELAPSED}s"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "Timeout waiting for sippy API to start after ${TIMEOUT}s"
    exit 1
fi


# Run our tests that request against the API, args ensure serially and fresh test code compile:
gotestsum ./test/e2e/... -count 1 -p 1

# WARNING: do not place more commands here without addressing return code from go test not being overridden by the cleanup func
