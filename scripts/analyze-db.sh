#!/bin/bash
# Warm up the sippy database after cloning or restoring from a snapshot.
# Runs ANALYZE VERBOSE (updates planner statistics) followed by
# REINDEX DATABASE CONCURRENTLY (forces index pages off lazy-loaded EBS
# storage). Both are necessary for full query performance.
#
# The pod runs detached so your local machine does not need to stay
# connected. Use --wait to poll until completion instead.
#
# Usage:
#   ./scripts/analyze-db.sh [OPTIONS]
#
# Options:
#   --namespace NS    Kubernetes namespace (default: sippy)
#   --db-secret NAME  Database secret name (default: postgres-aws)
#   --wait            Poll until the pod completes, then print logs
#   --dry-run         Print the command without executing

set -euo pipefail

NAMESPACE=sippy
DB_SECRET="postgres-aws"
WAIT=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --namespace)  [[ $# -ge 2 ]] || { echo "Error: --namespace requires a value" >&2; exit 1; }; NAMESPACE="$2"; shift 2 ;;
        --db-secret)  [[ $# -ge 2 ]] || { echo "Error: --db-secret requires a value" >&2; exit 1; }; DB_SECRET="$2"; shift 2 ;;
        --wait)       WAIT=true; shift ;;
        --dry-run)    DRY_RUN=true; shift ;;
        *)            echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

POD_NAME="sippy-analyze-db"
IMAGE="registry.redhat.io/rhel9/postgresql-16:latest"

if [[ "$DRY_RUN" == "true" ]]; then
    echo "Would create pod $POD_NAME to run ANALYZE VERBOSE and REINDEX"
    exit 0
fi

oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

echo "Creating pod $POD_NAME to run ANALYZE VERBOSE and REINDEX..."

oc -n "$NAMESPACE" run "$POD_NAME" --restart=Never \
    --image="$IMAGE" \
    --overrides="{
        \"spec\": {
            \"containers\": [{
                \"name\": \"$POD_NAME\",
                \"image\": \"$IMAGE\",
                \"command\": [\"sh\", \"-c\"],
                \"args\": [\"echo 'Starting ANALYZE VERBOSE...' && psql \\\"\$SIPPY_DATABASE_DSN\\\" -c 'ANALYZE VERBOSE;' && echo 'Starting REINDEX DATABASE...' && psql \\\"\$SIPPY_DATABASE_DSN\\\" -c 'REINDEX DATABASE CONCURRENTLY sippy_openshift;' && echo 'Warmup complete.'\"],
                \"env\": [{
                    \"name\": \"SIPPY_DATABASE_DSN\",
                    \"valueFrom\": {\"secretKeyRef\": {\"name\": \"$DB_SECRET\", \"key\": \"dsn\"}}
                }]
            }]
        }
    }"

if [[ "$WAIT" == "true" ]]; then
    echo "Waiting for pod to complete..."
    oc -n "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=120m "pod/$POD_NAME" 2>/dev/null || {
        STATUS=$(oc -n "$NAMESPACE" get pod "$POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "Pod finished with status: $STATUS" >&2
        oc -n "$NAMESPACE" logs "$POD_NAME" --tail=20 2>/dev/null || true
        exit 1
    }
    echo "Warmup complete."
    oc -n "$NAMESPACE" logs "$POD_NAME" --tail=5
    oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found >/dev/null 2>&1 || true
else
    echo "Pod is running detached. To follow progress:"
    echo "  oc -n $NAMESPACE logs -f $POD_NAME"
    echo "To check status:"
    echo "  oc -n $NAMESPACE get pod $POD_NAME"
fi
