---
description: "Start both Sippy backend and frontend dev servers"
---

# Sippy dev — backend + frontend

Start the full local Sippy stack using two MCP tool calls (server: **`sippy-dev`**). Run them in order — backend first, then frontend.

1. **`sippy_serve`** — pass **`bigquery_credentials_file`** when `SIPPY_BIGQUERY_CREDENTIALS_FILE` / `GOOGLE_APPLICATION_CREDENTIALS` are not set.
2. **`sippy_ng_start`**

Each tool returns listen hints (typically **8080** / **3000**) and log paths. If a tool reports already running, ask the user if they want to restart it. If yes, call the tool again with **`restart=True`**.
