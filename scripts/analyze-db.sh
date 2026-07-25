#!/bin/bash
# Warm up the sippy database after cloning or restoring from a snapshot.
# Runs three steps:
#   1. ANALYZE VERBOSE — updates planner statistics
#   2. REINDEX DATABASE CONCURRENTLY — forces index pages off lazy-loaded
#      EBS storage
#   3. Cache warming — SELECT count(*) on all tables, scoped to the last
#      30 days for tables with date/timestamp columns
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
    echo "Would create pod $POD_NAME to run ANALYZE, REINDEX, and cache warming"
    exit 0
fi

oc -n "$NAMESPACE" delete pod "$POD_NAME" --ignore-not-found --wait >/dev/null 2>&1 || true

echo "Creating pod $POD_NAME to warm up the database..."

# The pod runs a shell script that writes a SQL warmup script to a temp
# file, then executes the three warmup steps in sequence.
WARMUP_SCRIPT=$(cat << 'SCRIPT_EOF'
set -e

cat > /tmp/warmup.sql << 'SQL_EOF'
DO $$
DECLARE
    tbl RECORD;
    date_col TEXT;
    row_count BIGINT;
    query TEXT;
BEGIN
    FOR tbl IN
        SELECT t.tablename
        FROM pg_tables t
        WHERE t.schemaname = 'public'
          AND NOT EXISTS (
              SELECT 1 FROM pg_inherits i
              JOIN pg_class c ON c.oid = i.inhrelid
              WHERE c.relname = t.tablename
          )
        ORDER BY t.tablename
    LOOP
        SELECT c.column_name INTO date_col
        FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.table_name = tbl.tablename
          AND c.data_type IN ('date', 'timestamp with time zone', 'timestamp without time zone')
        ORDER BY c.column_name
        LIMIT 1;

        IF date_col IS NOT NULL THEN
            query := format(
                'SELECT count(*) FROM %I WHERE %I >= now() - interval ''30 days''',
                tbl.tablename, date_col
            );
        ELSE
            query := format('SELECT count(*) FROM %I', tbl.tablename);
        END IF;

        EXECUTE query INTO row_count;

        IF date_col IS NOT NULL THEN
            RAISE NOTICE 'Warmed % (last 30 days via %): % rows', tbl.tablename, date_col, row_count;
        ELSE
            RAISE NOTICE 'Warmed % (full table): % rows', tbl.tablename, row_count;
        END IF;
    END LOOP;
END
$$;
SQL_EOF

echo "Step 1/3: ANALYZE VERBOSE..."
psql "$SIPPY_DATABASE_DSN" -c "ANALYZE VERBOSE;"

echo "Step 2/3: REINDEX DATABASE..."
psql "$SIPPY_DATABASE_DSN" -c "REINDEX DATABASE CONCURRENTLY sippy_openshift;"

echo "Step 3/3: Cache warming..."
psql "$SIPPY_DATABASE_DSN" -f /tmp/warmup.sql

echo "Warmup complete."
SCRIPT_EOF
)

oc -n "$NAMESPACE" run "$POD_NAME" --restart=Never \
    --image="$IMAGE" \
    --overrides="{
        \"spec\": {
            \"containers\": [{
                \"name\": \"$POD_NAME\",
                \"image\": \"$IMAGE\",
                \"command\": [\"sh\", \"-c\"],
                \"args\": [$(echo "$WARMUP_SCRIPT" | python3 -c "import sys,json; print(json.dumps(sys.stdin.read()))")],
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
