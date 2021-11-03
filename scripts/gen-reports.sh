#!/bin/sh

if [ ! -f /data/test-reports/current-reports.json ]
then
  /bin/sippy -v 4 --gen-reports --local-data /data --release 3.11 --release 4.6 --release 4.7 --release 4.8 --release 4.9 --release 4.10
fi
