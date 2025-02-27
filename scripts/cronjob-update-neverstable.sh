#!/bin/bash
set -x
set -o errexit
set -o pipefail

# This repo is assumed checked out in current directory
git checkout -b update

# Generate and push mapping
OUTPUT=$(./scripts/update-neverstable.sh)

if git diff --quiet
then
  echo "No changes."
  exit 0
fi

# get token with write ability (after slow work is done; do not give the token a chance to expire)
keyfile="${GHAPP_KEYFILE:-/secrets/ghapp/private.key}"
set +x
trt_token=`gh-token generate --app-id 1046118 --key "$keyfile" --installation-id 57361690 --token-only`  # 57361690 = openshift-trt
git remote add openshift-trt "https://oauth2:${trt_token}@github.com/openshift-trt/sippy.git"
set -x

git commit -a -m "Update never-stable"
git push openshift-trt update --force

pr-creator -org openshift -repo sippy -source openshift-trt:update -branch main \
	   -github-app-private-key-path "${keyfile}" -github-app-id 1046118 \
	   -body "**Note: PLEASE REVIEW CHANGES BEFORE MERGING**<br><br>$OUTPUT" \
	   -title "Automated - Update never-stable" \
	   -confirm
