#!/usr/bin/env bash
# Restore the prod-like PostgreSQL database from a backup file or directory under the repo root.
# Usage: scripts/restore_prodlike_db.sh <backup-path-relative-to-repo>
# Env: SIPPY_PRODLIKE_DATABASE_DSN (single host, localhost or sippy-postgres; must end with /prodlike).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [ "${#}" -lt 1 ]; then
	echo "usage: $0 <backup-path>" >&2
	echo "  backup-path: .sql file, custom-format dump, or directory backup (with toc.dat) relative to repo root" >&2
	exit 1
fi

rel="${1#./}"
if [[ "${rel}" == /* ]]; then
	echo "error: use a path relative to the repo root, not an absolute path" >&2
	exit 1
fi

backup="${REPO_ROOT}/${rel}"
if [ ! -e "${backup}" ]; then
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
# Match the whole DSN so multi-host lists (host1,host2) and userinfo tricks cannot pass.
dsn_re='^postgres(ql)?://([^@/?]*@)?(localhost|sippy-postgres)(:[0-9]+)?/prodlike(\?.*)?$'
if [[ ! "${prodl}" =~ ${dsn_re} ]]; then
	echo "error: SIPPY_PRODLIKE_DATABASE_DSN must be a single-host DSN for localhost or sippy-postgres ending with /prodlike" >&2
	exit 1
fi
prodl_host="${BASH_REMATCH[3]}${BASH_REMATCH[4]}"
# libpq query parameters can override the host or database named in the URI, and libpq
# percent-decodes parameter names (ho%73t is host), so encoded names are rejected outright.
# Values stay untouched so encoded credentials keep working.
query=""
if [[ "${prodl}" == *\?* ]]; then
	query="${prodl#*\?}"
fi
IFS='&' read -r -a query_params <<<"${query}"
for param in ${query_params[@]+"${query_params[@]}"}; do
	name="${param%%=*}"
	case "${name}" in
	*%*)
		echo "error: SIPPY_PRODLIKE_DATABASE_DSN query parameter names must not be percent-encoded" >&2
		exit 1
		;;
	host | hostaddr | service | dbname)
		echo "error: SIPPY_PRODLIKE_DATABASE_DSN must not set host, hostaddr, service, or dbname query parameters" >&2
		exit 1
		;;
	esac
done
# Keep the query on the admin DSN so query-only credentials and port reach both connections.
clean="${prodl%%\?*}"
admin="${clean%/prodlike}/postgres${query:+?${query}}"

# Validate the backup before the destructive DROP DATABASE below.
if [[ -f "${backup}" && "${backup}" == *.sql ]]; then
	:
elif [[ -d "${backup}" && ! -f "${backup}/toc.dat" ]]; then
	echo "error: invalid directory backup, toc.dat not found: ${backup}" >&2
	exit 1
elif ! pg_restore --list "${backup}" >/dev/null; then
	echo "error: backup is not a valid pg_restore archive: ${backup}" >&2
	exit 1
fi

# Log only allowlisted details; the DSN can carry credentials in several places.
echo "Using prod-like database: prodlike on ${prodl_host}"
echo "Restoring from: ${backup}"

psql "${admin}" -v ON_ERROR_STOP=1 -c \
	"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'prodlike' AND pid <> pg_backend_pid();" \
	|| true
psql "${admin}" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS prodlike;"
psql "${admin}" -v ON_ERROR_STOP=1 -c "CREATE DATABASE prodlike;"

if [[ -f "${backup}" && "${backup}" == *.sql ]]; then
	psql "${prodl}" -v ON_ERROR_STOP=1 -f "${backup}"
else
	pg_restore --no-owner --exit-on-error -d "${prodl}" "${backup}"
fi

echo "Restore finished. Run migrations if needed:"
echo "  go run ./cmd/sippy migrate --database-dsn \"\$SIPPY_PRODLIKE_DATABASE_DSN\""
