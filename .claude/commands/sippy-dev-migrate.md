---
description: Run Sippy PostgreSQL schema migration
---

# Sippy dev — migrate

The devcontainer has two databases: **seed** (`postgres`) and **prod-like** (`prodlike`). To migrate both:

```bash
go run ./cmd/sippy migrate --database-dsn "$SIPPY_SEED_DATABASE_DSN"
go run ./cmd/sippy migrate --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
```

Or to migrate just the active database (based on `SIPPY_DATA_MODE`):

```bash
go run ./cmd/sippy migrate --database-dsn "$SIPPY_DATABASE_DSN"
```

If no env vars are set, the dev default is: `postgresql://postgres:password@localhost:5432/postgres`