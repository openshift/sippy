# Sippy dev MCP server

Python [FastMCP](https://github.com/jlowin/fastmcp) server (`server.py`) that exposes Sippy development commands requiring process management to **Cursor** and **Claude Code**.

Simple commands (migrate, lint, test) are documented in project instructions via [APM](../apm.yml) and can be run directly — see `.apm/instructions/dev-commands.instructions.md`.

## Setup

**`mcp/run.sh`** is the entry point. It auto-creates the virtualenv (`mcp/.venv`) and installs dependencies on first run — no manual setup required. The devcontainer's `post-create.sh` pre-warms the venv during build.

- **Python 3.12** required. The devcontainer image installs it; on macOS use `brew install python@3.12`.
- **Manual venv setup** (if you prefer not to use `run.sh`):

  ```bash
  python3.12 -m venv mcp/.venv
  mcp/.venv/bin/pip install --upgrade pip
  mcp/.venv/bin/pip install -r mcp/requirements.txt
  ```

## Editor configuration

| Client      | Config file             |
| ----------- | ----------------------- |
| Cursor      | `.cursor/mcp.json`      |
| Claude Code | `.mcp.json` (repo root) |

Both point to `mcp/run.sh`. This works on the host and inside the devcontainer without path changes.

## Server id in Cursor

The MCP server key in config is **`sippy-dev`**. Cursor may expose tools under a **prefixed** server name (e.g. `project-0-workspace-sippy-dev`). Use the server id your client lists when calling tools.

## Tools

Commands use the **repo root** as working directory unless noted. Most long outputs go to **`sippy-dev-logs/`** (see `.gitignore`).

| Tool               | What it runs                                                                | Default log                           |
| ------------------ | --------------------------------------------------------------------------- | ------------------------------------- |
| `regression_cache` | `go run ./cmd/sippy load --loader regression-cache` (BigQuery + Redis + DB) | `sippy-dev-logs/regression_cache.log` |
| `sippy_serve`      | Background `go run ./cmd/sippy serve` (API/UI, typically port **8080**)     | `sippy-dev-logs/sippy_serve.log`      |
| `sippy_ng_start`   | Background `npm start` in `sippy-ng/` (typically port **3000**)             | `sippy-dev-logs/sippy_ng_start.log`   |
| `run_e2e`          | `make e2e`                                                                  | `sippy-dev-logs/run_e2e.log`          |

Optional parameters (timeouts, paths, DSNs, etc.) are documented on each function in **`server.py`**.

> **Cost caution:** `run_e2e` and `regression_cache` issue BigQuery queries that cost real money. Run them only when explicitly needed and never more than once per request.

### Credentials and environment

- **Service account JSON** (BigQuery / GCS): pass `bigquery_credentials_file` where supported, or set **`SIPPY_BIGQUERY_CREDENTIALS_FILE`** or **`GOOGLE_APPLICATION_CREDENTIALS`** to an existing file path. Typical local file: `sippy-bigquery-job-importer-key.json` at repo root.
- **`run_e2e`** sets **`GCS_SA_JSON_PATH`** for `scripts/e2e.sh` from that same resolution.
- **Postgres / Redis**: `SIPPY_DATABASE_DSN`, `REDIS_URL`, or per-tool arguments; see `server.py` for defaults.

### E2E containers

`scripts/e2e.sh` uses **`DOCKER`** if set; otherwise **Podman** if on `PATH`, else **Docker**. Install one of them, or set `DOCKER` to the CLI you use.

### Background processes

`sippy_serve` and `sippy_ng_start` spawn detached processes. A second start is refused if a matching process is already running (see `server.py` for detection logic).

## Cursor skills

Agent-oriented shortcuts live under **`.cursor/skills/`**, for example:

- `sippy-dev-migrate`, `sippy-dev-regression-cache`, `sippy-dev-serve`, `sippy-dev-frontend`
- `sippy-dev-app` (backend + frontend)
- `sippy-dev-tests` (order: `CI=true make lint` → `make test` → `run_e2e`)

## Changing the server

After editing **`server.py`**, restart the **sippy-dev** MCP server (or reload the editor) so tool lists stay in sync.

## Adding tools

Add `@mcp.tool()` handlers in `server.py`, mirror existing patterns (`subprocess`, `_repo_path`, `_ensure_dev_log_dir`, credentials helpers), then restart MCP.
