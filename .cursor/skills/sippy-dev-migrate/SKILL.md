---
name: sippy-dev-migrate
description: >-
  Runs Sippy PostgreSQL schema migration via the sippy-dev MCP tool migrate_db.
  Use when initializing or updating the local Sippy database, fixing missing
  tables (e.g. test_regressions), or when the user mentions migrate_db, sippy
  migrate, or DB schema for Sippy.
---

# Sippy dev MCP — migrate

**`call_mcp_tool`**: tool **`migrate_db`**. Server: **`sippy-dev`** (from **`.cursor/mcp.json`**) or Cursor’s prefixed id (e.g. **`project-0-workspace-sippy-dev`**). Do not use shell `go run ./cmd/sippy migrate` instead.

Optional: **`database_dsn`**. Otherwise the server uses `SIPPY_DATABASE_DSN` or its default (see **`mcp/server.py`**).
