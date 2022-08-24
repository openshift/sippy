#!/bin/sh

# sleep before fetching so that if we're in some sort of fast crashloop/reschedule mode
echo "Doing initial sleep before fetching prow data"
sleep 600
while [ true ]; do
  echo "Fetching new prow data"
  echo "Loading database"
  /bin/sippy --load-database \
    --load-prow=true \
    --load-github=true \
    --load-testgrid=false \
    --release 3.11 \
    --release 4.6 \
    --release 4.7 \
    --release 4.8 \
    --release 4.9 \
    --release 4.10 \
    --release 4.11 \
    --release 4.12 \
    --release Presubmits \
    --arch amd64 \
    --arch arm64 \
    --arch multi \
    --arch s390x \
    --arch ppc64le \
    --config /config/openshift.yaml \
    --skip-bug-lookup \
    --mode=ocp
  echo "Done fetching data, refreshing server"
  curl localhost:8080/refresh
  echo "Done refreshing data, sleeping"
  sleep 3600  # 1 hour
done
