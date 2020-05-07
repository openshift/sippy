#!/bin/sh

# sleep before fetching so that if we're in some sort of fast crashloop/reschedule mode, 
# we don't ping testgrid everytime we come back up
echo "Doing initial sleep before fetching testgrid data"
sleep 600 # 10 minutes
while [ true ]; do
  echo "Fetching new testgrid data"
  rm -rf /data/*
  /tmp/src/sippy --fetch-data /data --release 4.2 --release 4.3 --release 4.4 --release 4.5 -v 4
  echo "Done fetching data, refreshing server"
  curl localhost:8080/refresh
  echo "Done refreshing data, sleeping"
  sleep 7200  # 2 hours
done
