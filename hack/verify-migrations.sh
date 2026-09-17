#!/bin/bash
set -euo pipefail

MIGRATIONS_DIR="pkg/db/migrations"
MANIFEST="${MIGRATIONS_DIR}/MANIFEST"

if [ ! -f "$MANIFEST" ]; then
    echo "ERROR: ${MANIFEST} not found."
    exit 1
fi

errors=0

# Check each manifest entry has matching .up.sql and .down.sql files.
prev_num=0
while IFS= read -r entry; do
    [ -z "$entry" ] && continue
    [[ "$entry" == \#* ]] && continue

    num=$(echo "$entry" | grep -oE '^[0-9]+' | sed 's/^0*//')
    if [ -z "$num" ]; then
        echo "ERROR: Invalid manifest entry '${entry}' (must start with a zero-padded number)."
        errors=$((errors + 1))
        continue
    fi

    # Check sequential numbering.
    expected=$((prev_num + 1))
    if [ "$num" -ne "$expected" ]; then
        echo "ERROR: Expected migration $(printf '%06d' "$expected") but found $(printf '%06d' "$num") (gap or duplicate)."
        errors=$((errors + 1))
    fi
    prev_num=$num

    if [ ! -f "${MIGRATIONS_DIR}/${entry}.up.sql" ]; then
        echo "ERROR: Manifest lists '${entry}' but ${MIGRATIONS_DIR}/${entry}.up.sql does not exist."
        errors=$((errors + 1))
    fi
    if [ ! -f "${MIGRATIONS_DIR}/${entry}.down.sql" ]; then
        echo "ERROR: Manifest lists '${entry}' but ${MIGRATIONS_DIR}/${entry}.down.sql does not exist."
        errors=$((errors + 1))
    fi
done < "$MANIFEST"

# Check no extra SQL files exist beyond what the manifest lists.
expected_sql=$((prev_num * 2))
actual_sql=$(find "$MIGRATIONS_DIR" -maxdepth 1 -name '*.sql' | wc -l | tr -d ' ')
if [ "$actual_sql" -ne "$expected_sql" ]; then
    echo "ERROR: Expected ${expected_sql} .sql files (${prev_num} up + ${prev_num} down) but found ${actual_sql}."
    errors=$((errors + 1))
fi

if [ "$errors" -gt 0 ]; then
    echo "FAILED: ${errors} migration verification error(s)."
    exit 1
fi

echo "Migrations OK: ${prev_num} migrations verified."
