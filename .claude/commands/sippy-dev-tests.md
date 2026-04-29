---
description: Run lint, unit tests, and e2e in order (full local CI suite)
---

# Sippy dev — full test suite

Run these three steps in order. Stop if any step fails.

1. **Lint** — run directly:

   ```bash
   CI=true make lint
   ```

   `CI=true` makes `hack/go-lint.sh` use the locally installed `golangci-lint` instead of spawning a container.

2. **Unit tests** — run directly:

   ```bash
   make test
   ```

3. **E2e** — use the **`run_e2e`** MCP tool (server: **`sippy-dev`**). Pass **`bigquery_credentials_file`** when cred env vars are unset. E2e requires Podman/Docker and **does not work inside the devcontainer** — run it on the host.

E2e log: **`sippy-dev-logs/run_e2e.log`**. Timeouts: **`mcp/server.py`**.