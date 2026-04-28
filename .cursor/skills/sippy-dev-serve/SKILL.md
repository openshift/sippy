---
name: sippy-dev-serve
description: >-
  Starts the Sippy HTTP API/UI via the sippy-dev MCP tool sippy_serve (background
  go run ./cmd/sippy serve). Use when running Sippy locally for debugging, component
  readiness UI, or when the user mentions sippy_serve, sippy serve, or local Sippy server.
---

# Sippy dev MCP — serve

**`call_mcp_tool`**: tool **`sippy_serve`**. Server: **`sippy-dev`** or prefixed (e.g. **`project-0-workspace-sippy-dev`**). Do not use shell `go run ./cmd/sippy serve` instead.

**`bigquery_credentials_file`**: same as regression-cache; optional if `SIPPY_BIGQUERY_CREDENTIALS_FILE` or `GOOGLE_APPLICATION_CREDENTIALS` is set. See **`mcp/server.py`** for all parameters.
