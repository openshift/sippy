---
description: "MCP servers: sippy-dev (dev tasks) and sippy (production Sippy data)"
applyTo: "mcp/**"
---

The `sippy-dev` MCP server (`server.py`) handles AI-callable dev tasks (migrate, serve, restore prod-like DB, lint, test, e2e). Configuration, tool list, logs, and extension notes: **[README.md](../../mcp/README.md)**.

When adding or modifying MCP tools, follow existing patterns in `server.py` (subprocess, `_repo_path`, `_ensure_dev_log_dir`, credentials helpers). Restart the MCP server after changes.

`mcp/sippy/` is the separate `sippy` MCP server, a general-purpose server for working with production Sippy data through Sippy's authenticated API. Triage management is its first toolset; add new capabilities as new toolsets (see **[sippy/README.md](../../mcp/sippy/README.md)**). Keep it independent of `sippy-dev`: never import `server.py`, `testenv.py`, or dotenv there, and never add development-workflow tools to it. Every mutating tool requires `acting_for`, sent as `X-Acting-For` and taken from the consumer's authenticated context; read tools take no identity. Its unit tests run from `make test`.
