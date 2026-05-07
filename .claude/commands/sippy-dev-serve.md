---
description: Start the Sippy HTTP API server via the sippy-dev MCP tool
---

# Sippy dev — serve

Use the **`sippy_serve`** MCP tool (server: **`sippy-dev`**). Do not run `go run ./cmd/sippy serve` manually — the MCP tool handles background process management, log routing, and duplicate detection.

The default **`data_provider`** is `"postgres"`, which uses seed data and **does not require BigQuery credentials**. Set `data_provider="bigquery"` and provide **`bigquery_credentials_file`** to use BigQuery instead.

If the server is already running, the tool will report it. Ask the user if they want to restart, and if so call again with **`restart=True`**.

See **`mcp/server.py`** for all parameters. Typical listen address: **`:8080`**. Log: **`sippy-dev-logs/sippy_serve.log`**.