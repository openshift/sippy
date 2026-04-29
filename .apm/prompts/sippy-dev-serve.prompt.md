---
description: "Start the Sippy HTTP API server via the sippy-dev MCP tool"
---

# Sippy dev — serve

Use the **`sippy_serve`** MCP tool (server: **`sippy-dev`**). Do not run `go run ./cmd/sippy serve` manually — the MCP tool handles background process management, log routing, and duplicate detection.

**`bigquery_credentials_file`**: path to BigQuery-capable SA JSON (e.g. `sippy-bigquery-job-importer-key.json`); optional if `SIPPY_BIGQUERY_CREDENTIALS_FILE` or `GOOGLE_APPLICATION_CREDENTIALS` is set.

See **`mcp/server.py`** for all parameters. Typical listen address: **`:8080`**. Log: **`sippy-dev-logs/sippy_serve.log`**.
