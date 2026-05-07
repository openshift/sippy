---
description: "Restore prod-like PostgreSQL from a backup file"
---

# Sippy dev — restore prod-like DB

Restore the **`prodlike`** database from a backup that lives **under the repo root** (path relative to checkout, no `..`).

## Preconditions

- **`SIPPY_PRODLIKE_DATABASE_DSN`** must end with **`/prodlike`** and use host **`localhost`** or **`sippy-postgres`** (script refuses other hosts).
- Stop **`sippy serve`** (and anything else connected to `prodlike`) so `DROP DATABASE` can run.
- **`pg_restore`** must match the dump format (devcontainer: PostgreSQL 17 client on `PATH`).

## Steps

1. **CLI** (from repo root):

   ```bash
   scripts/restore_prodlike_db.sh <backup-path-relative-to-repo>
   ```

   Examples: `backups/sippy-prodlike.dump`, `sippy-backup-dev-2026-05-07.dump` if that file is in the repo.

   - Custom / directory format: **`pg_restore`**
   - **`*.sql`**: **`psql -f`**

2. **MCP** (server **`sippy-dev`**): call **`restore_prodlike_db`** with **`backup_file`** set to the same repo-relative path. Large dumps: **`timeout_seconds=0`**. Log: **`sippy-dev-logs/restore_prodlike_db.log`**.

3. After restore, migrate if the schema might be behind:

   ```bash
   go run ./cmd/sippy migrate --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
   ```
