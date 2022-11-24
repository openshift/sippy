#!/bin/sh

echo "Fetching new testgrid data"
rm -rf /data/*
/bin/sippy --fetch-data /data --dashboard=kube-master=sig-release-master-blocking,sig-release-master-informing= --log-level debug
echo "Loading database"
/bin/sippy --load-database --local-data /data --dashboard=kube-master=sig-release-master-blocking,sig-release-master-informing= --log-level debug --mode=kube
echo "Done fetching data"
