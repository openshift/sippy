#!/usr/bin/env bash
# Restore the prod-like PostgreSQL database from a backup file under the repo root.
# Usage: scripts/restore_prodlike_db.sh <backup-path-relative-to-repo>
# Env: SIPPY_PRODLIKE_DATABASE_DSN (must end with /prodlike; host localhost or sippy-postgres only).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [ "${#}" -lt 1 ]; then
	echo "usage: $0 <backup-file>" >&2
	echo "  backup-file: path relative to repo root (e.g. backups/foo.dump)" >&2
	exit 1
fi

rel="${1#./}"
if [[ "${rel}" == /* ]]; then
	echo "error: use a path relative to the repo root, not an absolute path" >&2
	exit 1
fi

backup="${REPO_ROOT}/${rel}"
if [ ! -f "${backup}" ]; then
	echo "error: backup not found: ${backup}" >&2
	exit 1
fi
case "$(realpath "${backup}")/" in
"${REPO_ROOT}/"*) ;;
*)
	echo "error: backup must resolve inside the repository" >&2
	exit 1
	;;
esac

prodl="${SIPPY_PRODLIKE_DATABASE_DSN:-postgresql://postgres:password@sippy-postgres:5432/prodlike}"
case "${prodl}" in
*@localhost:* | *@localhost/* | *@sippy-postgres:* | *@sippy-postgres/*) ;;
*)
	echo "error: SIPPY_PRODLIKE_DATABASE_DSN host must be localhost or sippy-postgres" >&2
	exit 1
	;;
esac
case "${prodl}" in
*/prodlike|*/prodlike\?*)
	clean="${prodl%%\?*}"
	admin="${clean%/prodlike}/postgres"
	;;
*)
	echo "error: SIPPY_PRODLIKE_DATABASE_DSN must end with /prodlike" >&2
	exit 1
	;;
esac

# Redact credentials from log output
redacted_prodl=$(echo "${prodl}" | sed 's|://[^@]*@|://REDACTED@|')
echo "Using prod-like DSN: ${redacted_prodl}"
echo "Restoring from: ${backup}"

psql "${admin}" -v ON_ERROR_STOP=1 -c \
	"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'prodlike' AND pid <> pg_backend_pid();" \
	|| true
psql "${admin}" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS prodlike;"
psql "${admin}" -v ON_ERROR_STOP=1 -c "CREATE DATABASE prodlike;"

if [[ "${backup}" == *.sql ]]; then
	psql "${prodl}" -v ON_ERROR_STOP=1 -f "${backup}"
else
	pg_restore --no-owner --exit-on-error -d "${prodl}" "${backup}"
fi

echo "Restore finished. Run migrations if needed:"
echo "  go run ./cmd/sippy migrate --database-dsn \"\$SIPPY_PRODLIKE_DATABASE_DSN\""
