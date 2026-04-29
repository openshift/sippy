---
description: "Run Sippy PostgreSQL schema migration"
---

# Sippy dev — migrate

Run the migration command directly:

```bash
go run ./cmd/sippy migrate --database-dsn "$SIPPY_DATABASE_DSN"
```

If `SIPPY_DATABASE_DSN` is not set, use the dev default:

```bash
go run ./cmd/sippy migrate --database-dsn "postgresql://postgres:password@localhost:5432/postgres"
```
