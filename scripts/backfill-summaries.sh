#!/bin/bash
# Gradually backfill a summary table one day at a time.
# Each day runs as a separate pod to avoid long-running transactions.
#
# Usage:
#   ./scripts/backfill-summaries.sh --table TABLE [OPTIONS]
#
# Options:
#   --table TABLE     Table to backfill (daily-totals,
#                     cumulative-summaries) [required]
#   --days N          Number of days to backfill (default: 91)
#   --namespace NS    Kubernetes namespace (default: sippy)
#   --image IMAGE     Container image (default: auto-detect from sippy DC)
#   --db-secret NAME  Database secret name [required]
#   --pause SECONDS   Seconds to wait between days (default: 5)
#   --batch N         Days per pod (default: 1)
#   --dry-run         Print commands without executing

set -euo pipefail

DAYS=91
TABLE=""
NAMESPACE=sippy
IMAGE=""
DB_SECRET=""
PAUSE=5
BATCH=1
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --table)      TABLE="$2"; shift 2 ;;
        --days)       DAYS="$2"; shift 2 ;;
        --namespace)  NAMESPACE="$2"; shift 2 ;;
        --image)      IMAGE="$2"; shift 2 ;;
        --db-secret)  DB_SECRET="$2"; shift 2 ;;
        --pause)      PAUSE="$2"; shift 2 ;;
        --batch)      BATCH="$2"; shift 2 ;;
        --dry-run)    DRY_RUN=true; shift ;;
        *)            echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

for arg_name in DAYS PAUSE BATCH; do
    arg_val="${!arg_name}"
    if ! [[ "$arg_val" =~ ^[1-9][0-9]*$ ]]; then
        echo "Error: --$(echo "$arg_name" | tr '[:upper:]' '[:lower:]') must be a positive integer, got '$arg_val'" >&2
        exit 1
    fi
done

if [[ -z "$DB_SECRET" ]]; then
    echo "Error: --db-secret is required" >&2
    exit 1
fi

if [[ -z "$TABLE" ]]; then
    echo "Error: --table is required" >&2
    echo "Valid tables: daily-totals, cumulative-summaries" >&2
    exit 1
fi

case "$TABLE" in
    daily-totals|cumulative-summaries) ;;
    *)
        echo "Error: invalid table '$TABLE'" >&2
        echo "Valid tables: daily-totals, cumulative-summaries" >&2
        exit 1
        ;;
esac

if [[ -z "$IMAGE" ]]; then
    IMAGE=$(oc -n "$NAMESPACE" get dc/sippy -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null) || true
    if [[ -z "$IMAGE" ]]; then
        echo "Could not auto-detect image. Use --image to specify." >&2
        exit 1
    fi
    echo "Using image: $IMAGE"
fi

today=$(date -u +%Y-%m-%d)
completed=0
failed=0

for ((offset=DAYS; offset>0; offset-=BATCH)); do
    batch_end_offset=$((offset - BATCH + 1))
    if (( batch_end_offset < 0 )); then
        batch_end_offset=0
    fi

    start_date=$(date -u -v-${offset}d +%Y-%m-%d 2>/dev/null || date -u -d "$today - ${offset} days" +%Y-%m-%d)
    end_date=$(date -u -v-${batch_end_offset}d +%Y-%m-%d 2>/dev/null || date -u -d "$today - ${batch_end_offset} days" +%Y-%m-%d)

    pod_name="sippy-backfill-$(echo "$start_date" | tr -d '-')"
    echo "[$(date +%H:%M:%S)] Processing $start_date to $end_date ($((DAYS - offset + BATCH))/$DAYS days)..."

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  Would create pod $pod_name: sippy backfill --table=$TABLE --start-date=$start_date --end-date=$end_date"
        completed=$((completed + 1))
        continue
    fi

    # Clean up any leftover pod from a previous run
    oc -n "$NAMESPACE" delete pod "$pod_name" --ignore-not-found --wait=false >/dev/null 2>&1 || true
    sleep 2

    oc -n "$NAMESPACE" run "$pod_name" --rm -i --restart=Never \
        --image="$IMAGE" \
        --overrides="{
            \"spec\": {
                \"containers\": [{
                    \"name\": \"$pod_name\",
                    \"image\": \"$IMAGE\",
                    \"command\": [\"sippy\"],
                    \"args\": [\"backfill\",
                        \"--table=$TABLE\",
                        \"--start-date=$start_date\",
                        \"--end-date=$end_date\"],
                    \"env\": [{
                        \"name\": \"SIPPY_DATABASE_DSN\",
                        \"valueFrom\": {\"secretKeyRef\": {\"name\": \"$DB_SECRET\", \"key\": \"dsn\"}}
                    }]
                }]
            }
        }" < /dev/null 2>&1 | { grep -iE 'summar|error|elapsed|complete|panic|fatal|fail|timeout|warn' || true; } || true
    exit_code=${PIPESTATUS[0]}
    if [[ $exit_code -eq 0 ]]; then
        completed=$((completed + 1))
        echo "  Done."
    else
        failed=$((failed + 1))
        echo "  FAILED (exit $exit_code)" >&2
    fi

    if (( offset - BATCH > 0 )); then
        sleep "$PAUSE"
    fi
done

echo ""
echo "Backfill complete: $completed succeeded, $failed failed out of $(( (DAYS + BATCH - 1) / BATCH )) batches."
