---
paths:
  - "mcp/**"
---

The `sippy-dev` MCP server (`server.py`) handles AI-callable dev tasks (migrate, serve, restore prod-like DB, lint, test, e2e). Configuration, tool list, logs, and extension notes: **[README.md](../../mcp/README.md)**.

When adding or modifying MCP tools, follow existing patterns in `server.py` (subprocess, `_repo_path`, `_ensure_dev_log_dir`, credentials helpers). Restart the MCP server after changes.

`mcp/sippy/` is the separate `sippy` MCP server for triage management; see **[sippy/README.md](../../mcp/sippy/README.md)**. Keep it independent of `sippy-dev`: never import `server.py`, `testenv.py`, or dotenv there, and never add development-workflow tools to it. Every mutating `sippy` tool requires `acting_for`, sent as `X-Acting-For`; it must come from the consumer's authenticated context. Read tools take no identity. Credentials come only from runtime configuration (never defaults or image content), missing configuration fails closed, and mutations return the record from Sippy's write response and are never retried automatically. Its unit tests run from `make test` (`sippy/`, excluding the integration module) and its integration tests from `make e2e`.
