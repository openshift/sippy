#!/bin/sh

# sleep before fetching so that if we're in some sort of fast crashloop/reschedule mode
echo "Doing initial sleep before fetching prow data"
while [ true ]; do
  echo "Fetching new prow data"
  echo "Loading database"
  ./sippy --load-database --load-prow=true --load-testgrid=false --skip-bug-lookup
  echo "Done fetching data, refreshing server"
  curl localhost:8080/refresh
  echo "Done refreshing data, sleeping"
  sleep 3600  # 1 hour
done
