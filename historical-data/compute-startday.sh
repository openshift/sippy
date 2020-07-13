#!/bin/sh
# $1 - historical release directory

cur=$(date +"%s")
ga=$(cat $1/gatimestamp)

echo "startDay=$(((cur-ga)/60/60/24))"

