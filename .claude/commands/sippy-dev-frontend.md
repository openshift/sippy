---
description: Start the sippy-ng React dev server via the sippy-dev MCP tool
---

# Sippy dev — frontend (sippy-ng)

Use the **`sippy_ng_start`** MCP tool (server: **`sippy-dev`**). Do not run `npm start` in `sippy-ng` manually — the MCP tool handles background process management, log routing, and duplicate detection.

**`open_browser`** defaults to **`false`**. Typical URL: **`http://127.0.0.1:3000/sippy-ng`**. Log: **`sippy-dev-logs/sippy_ng_start.log`**.

If the dev server is already running, the tool will report it. Ask the user if they want to restart, and if so call again with **`restart=True`**.

See **`mcp/server.py`** for all parameters.