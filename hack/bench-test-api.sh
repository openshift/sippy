#!/usr/bin/env bash
#
# Benchmark the /api/tests endpoint across production and local Sippy instances.
# Usage: ./hack/bench-test-api.sh [local_base_url]
#   local_base_url defaults to http://localhost:8080

set -euo pipefail

LOCAL="${1:-http://localhost:8080}"
PROD="https://sippy.dptools.openshift.org"
RELEASE="4.19"
RUNS=3

# ANSI colors
bold='\033[1m'
reset='\033[0m'
green='\033[32m'
yellow='\033[33m'

# Each entry: "label|query_params"
queries=(
  "collapsed (default)|release=${RELEASE}&collapse=true"
  "uncollapsed|release=${RELEASE}&collapse=false"
  "collapsed + twoDay|release=${RELEASE}&collapse=true&period=twoDay"
  "uncollapsed + twoDay|release=${RELEASE}&collapse=false&period=twoDay"
  "collapsed + filter name contains sig-node|release=${RELEASE}&collapse=true&filter=$(python3 -c 'import urllib.parse,json; print(urllib.parse.quote(json.dumps({"items":[{"columnField":"name","operatorValue":"contains","value":"sig-node"}]})))')"
  "uncollapsed + filter name contains sig-node|release=${RELEASE}&collapse=false&filter=$(python3 -c 'import urllib.parse,json; print(urllib.parse.quote(json.dumps({"items":[{"columnField":"name","operatorValue":"contains","value":"sig-node"}]})))')"
  "collapsed + filter runs > 14|release=${RELEASE}&collapse=true&filter=$(python3 -c 'import urllib.parse,json; print(urllib.parse.quote(json.dumps({"items":[{"columnField":"current_runs","operatorValue":">","value":"14"}]})))')"
  "uncollapsed + paginated page 0|release=${RELEASE}&collapse=false&perPage=25&page=0"
)

time_url() {
  local url="$1"
  local total=0
  local times=()
  for i in $(seq 1 "$RUNS"); do
    local t
    t=$(curl -so /dev/null -w '%{time_total}' "$url" 2>/dev/null)
    times+=("$t")
    total=$(echo "$total + $t" | bc)
  done
  local avg
  avg=$(echo "scale=3; $total / $RUNS" | bc)
  echo "$avg"
}

printf "${bold}%-50s %12s %12s %12s${reset}\n" "Query" "Prod (s)" "Local (s)" "Speedup"
printf "%-50s %12s %12s %12s\n"   "-----" "--------" "---------" "-------"

for entry in "${queries[@]}"; do
  IFS='|' read -r label params <<< "$entry"
  prod_url="${PROD}/api/tests?${params}"
  local_url="${LOCAL}/api/tests?${params}"

  prod_time=$(time_url "$prod_url")
  local_time=$(time_url "$local_url")

  if (( $(echo "$local_time > 0" | bc -l) )); then
    speedup=$(echo "scale=1; $prod_time / $local_time" | bc 2>/dev/null || echo "n/a")
  else
    speedup="n/a"
  fi

  printf "%-50s %11.3fs %11.3fs %11sx\n" "$label" "$prod_time" "$local_time" "$speedup"
done
