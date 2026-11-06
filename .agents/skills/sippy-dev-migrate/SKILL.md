---
name: sippy-dev-migrate
description: "Run Sippy PostgreSQL schema migration"
---

# Sippy dev: migrate

The devcontainer has two databases: **seed** (`postgres`) and **prod-like** (`prodlike`). To migrate both:

```bash
go run ./cmd/sippy migrate --database-dsn "$SIPPY_SEED_DATABASE_DSN"
go run ./cmd/sippy migrate --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
```

Or to migrate just the active database (based on `SIPPY_DATA_MODE`):

```bash
case "${SIPPY_DATA_MODE:-seed}" in
seed) active_dsn="${SIPPY_SEED_DATABASE_DSN:-postgresql://postgres:password@localhost:5432/postgres}" ;;
prod-like) active_dsn="${SIPPY_PRODLIKE_DATABASE_DSN:-postgresql://postgres:password@localhost:5432/prodlike}" ;;
*) echo "unsupported SIPPY_DATA_MODE: ${SIPPY_DATA_MODE} (expected seed or prod-like)" >&2; exit 1 ;;
esac
go run ./cmd/sippy migrate --database-dsn "$active_dsn"
```

If no env vars are set, the dev default is: `postgresql://postgres:password@localhost:5432/postgres`
