#!/bin/sh

# sleep before fetching so that if we're in some sort of fast crashloop/reschedule mode,
# we don't ping testgrid everytime we come back up
echo "Doing initial sleep before fetching testgrid data"
sleep 600 # 10 minutes
while [ true ]; do
  echo "Fetching new testgrid data"
  rm -rf /data/*
  /bin/sippy -v 4 --fetch-data /data --release 3.11 --release 4.6 --release 4.7 --release 4.8 --release 4.9
  echo "Done fetching data, refreshing server"
  curl localhost:8080/refresh
  echo "Done refreshing data, sleeping"
  sleep 3600  # 1 hour
done
