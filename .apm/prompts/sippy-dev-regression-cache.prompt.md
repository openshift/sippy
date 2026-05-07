---
description: "Run the Sippy regression-cache loader (BigQuery + Redis + DB)"
---

# Sippy dev — regression-cache

Use the **`regression_cache`** MCP tool (server: **`sippy-dev`**). Do not run `go run ./cmd/sippy load --loader regression-cache` manually — the MCP tool handles credentials, logging, and timeouts.

**`bigquery_credentials_file`**: path to BigQuery-capable SA JSON (e.g. `sippy-bigquery-job-importer-key.json`); optional if `SIPPY_BIGQUERY_CREDENTIALS_FILE` or `GOOGLE_APPLICATION_CREDENTIALS` is set.

Always targets the **prod-like database** (`prodlike`) regardless of `SIPPY_DATA_MODE`. Pass `database_dsn` explicitly to override.

See **`mcp/server.py`** for all parameters. Log: **`sippy-dev-logs/regression_cache.log`**. Typical duration is many minutes.
