---
description: "Start the Sippy HTTP API server via the sippy-dev MCP tool"
---

# Sippy dev — serve

Use the **`sippy_serve`** MCP tool (server: **`sippy-dev`**). Do not run `go run ./cmd/sippy serve` manually — the MCP tool handles background process management, log routing, and duplicate detection.

Defaults are derived from **`SIPPY_DATA_MODE`** (`seed` or `prod-like`). In **seed** mode (default), the server uses `data_provider=postgres` with seed data and **does not require BigQuery credentials**. In **prod-like** mode, it uses `data_provider=bigquery` with `views_file=config/views.yaml` and the prod-like database (`prodlike`). Switch modes by setting `SIPPY_DATA_MODE=prod-like` in the environment.

Explicit parameter values always override mode-derived defaults.

If the server is already running, the tool will report it. Ask the user if they want to restart, and if so call again with **`restart=True`**.

See **`mcp/server.py`** for all parameters. Typical listen address: **`:8080`**. Log: **`sippy-dev-logs/sippy_serve.log`**.
