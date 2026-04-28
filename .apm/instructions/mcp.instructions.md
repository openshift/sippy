---
description: "MCP server (sippy-dev) for AI-callable dev tasks"
applyTo: "mcp/**"
---

Shared MCP server for AI-callable dev tasks (migrate, serve, lint, test, e2e). Configuration, tool list, logs, and extension notes: **[README.md](../../mcp/README.md)**.

When adding or modifying MCP tools, follow existing patterns in `server.py` (subprocess, `_repo_path`, `_ensure_dev_log_dir`, credentials helpers). Restart the MCP server after changes.
