---
name: sippy-dev-app
description: >-
  Starts the local Sippy stack by calling sippy-dev MCP tools sippy_serve then
  sippy_ng_start (backend then frontend). Use when the user wants both API/UI dev
  servers, full local Sippy, or a single workflow instead of separate serve and
  frontend steps.
---

# Sippy dev MCP — backend + frontend

Two **`call_mcp_tool`** calls, same server (**`sippy-dev`** or prefixed, e.g. **`project-0-workspace-sippy-dev`**). Do not use shell **`go run ./cmd/sippy serve`** or **`npm start`**.

1. **`sippy_serve`** — pass **`bigquery_credentials_file`** when `SIPPY_BIGQUERY_CREDENTIALS_FILE` / `GOOGLE_APPLICATION_CREDENTIALS` are not set (see **`mcp/server.py`** / **sippy-dev-serve**).
2. **`sippy_ng_start`**

Backend first, then frontend. Each tool returns listen hints (typically **8080** / **3000**) and log/pid paths; if a tool says already running, leave that process as-is.
