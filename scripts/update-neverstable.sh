#!/usr/bin/env bash
# This script updates the never-stable

# Use consistent locale, otherwise sort produces different results!
export LC_ALL="C.utf8"
export LANG="C.utf8"

SIPPY_URL=${SIPPY_URL:-https://sippy.dptools.openshift.org}
RELEASES=$(curl -s "${SIPPY_URL}/api/releases" | jq -r '.releases | join(" ")' | sed "s/Presubmits//")

NEVER_STABLE_ACTUAL="$(dirname "${BASH_SOURCE[0]}")/../pkg/testidentification/ocp_never_stable.txt"
NEVER_STABLE_TMP=$(mktemp)

for release in $RELEASES
do
    URL="${SIPPY_URL}/api/jobs?release=$release&filter=%7B%22items%22%3A%5B%7B%22columnField%22%3A%22current_pass_percentage%22%2C%22operatorValue%22%3A%22%3D%22%2C%22value%22%3A%220%22%7D%2C%7B%22columnField%22%3A%22previous_pass_percentage%22%2C%22operatorValue%22%3A%22%3D%22%2C%22value%22%3A%220%22%7D%5D%2C%22linkOperator%22%3A%22and%22%7D&period=default&sortField=current_runs&sort=desc"
    JOB_OUTPUT=$(curl -s "$URL")
    echo "$JOB_OUTPUT" | jq -r '.[].name'
done | sort > "$NEVER_STABLE_TMP"

echo "NEVER-STABLE NEW ADDITIONS"
echo "##########################"
comm -13 "$NEVER_STABLE_ACTUAL" "$NEVER_STABLE_TMP"

echo -e "\n\n"

echo "NEVER-STABLE GRADUATES"
echo "######################"
comm -23 "$NEVER_STABLE_ACTUAL" "$NEVER_STABLE_TMP"

mv "$NEVER_STABLE_TMP" "$NEVER_STABLE_ACTUAL"

echo -e "\n\n"

echo "*** Never stable update complete; review the results, commit and open a pull request."
