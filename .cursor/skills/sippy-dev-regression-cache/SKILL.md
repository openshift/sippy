---
name: sippy-dev-regression-cache
description: >-
  Runs the Sippy regression-cache loader via the sippy-dev MCP tool regression_cache
  (BigQuery, Redis, component readiness cache). Use when priming regression cache,
  component readiness cache, rerunning regression-cache logs under sippy-dev-logs,
  or when the user mentions regression_cache or regression-cache loader for Sippy.
---

# Sippy dev MCP — regression-cache

**`call_mcp_tool`**: tool **`regression_cache`**. Server: **`sippy-dev`** or prefixed (e.g. **`project-0-workspace-sippy-dev`**). Do not use shell `go run ./cmd/sippy load --loader regression-cache` instead.

Run **`migrate_db`** first if the DB is new.

**`bigquery_credentials_file`**: path to BigQuery-capable SA JSON (e.g. `sippy-bigquery-job-importer-key.json`); optional if `SIPPY_BIGQUERY_CREDENTIALS_FILE` or `GOOGLE_APPLICATION_CREDENTIALS` is set. See **`mcp/server.py`** for all parameters.
