---
name: sippy-dev-frontend
description: >-
  Starts the sippy-ng React dev server via the sippy-dev MCP tool sippy_ng_start
  (background npm start in sippy-ng). Use when running the Sippy UI against a local
  API or when the user mentions sippy_ng_start, sippy-ng dev server, or npm start
  for the frontend.
---

# Sippy dev MCP — frontend (sippy-ng)

**`call_mcp_tool`**: tool **`sippy_ng_start`**. Server: **`sippy-dev`** or prefixed (e.g. **`project-0-workspace-sippy-dev`**). Do not use shell `npm start` in `sippy-ng` instead.

**`open_browser`** defaults to **`false`** (no browser launched). See **`mcp/server.py`** for all parameters.
