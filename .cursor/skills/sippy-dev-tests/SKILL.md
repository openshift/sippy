---
name: sippy-dev-tests
description: >-
  Runs make lint, make test, and make e2e via three sippy-dev MCP tools (run_lint,
  run_test, run_e2e) in that order. Use for the full local CI suite or when the user
  mentions lint plus unit tests plus e2e for Sippy.
---

# Sippy dev MCP — full test suite

Three **`call_mcp_tool`** calls, same server (**`sippy-dev`** or prefixed, e.g. **`project-0-workspace-sippy-dev`**). Order: **`run_lint`** → **`run_test`** → **`run_e2e`**. Stop if any step fails. Do not run **`make lint` / `make test` / `make e2e`** manually for this workflow.

1. **`run_lint`** — inside the devcontainer, set **`CI=true`** (the MCP tool does this automatically) so `hack/go-lint.sh` uses the host-installed `golangci-lint` instead of spawning a nested container.
2. **`run_test`**
3. **`run_e2e`** — pass **`bigquery_credentials_file`** when cred env vars are unset (same SA JSON as other tools; sets **`GCS_SA_JSON_PATH`**). **Note:** e2e requires Podman/Docker-in-Docker support and **does not work inside the devcontainer** — run it on the host instead.

Logs: **`sippy-dev-logs/run_lint.log`**, **`sippy-dev-logs/run_test.log`**, **`sippy-dev-logs/run_e2e.log`**. Timeouts: **`mcp/server.py`**.
