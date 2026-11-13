---
name: sippy-dev-tests
description: "Run lint, unit tests, and e2e in order (full local CI suite)"
---

# Sippy dev: full test suite

Run these three steps in order. Stop if any step fails.

Use the `sippy-dev` MCP tools in both agentic solve and review phases. They run the full make targets with writable caches, artifact output, and prerequisite checks in the test subprocess.

Each make tool waits at most 30 seconds per request. If it returns a run ID, keep the MCP server alive and poll `get_make_result` until it returns the final result before proceeding. A client request timeout leaves the run active; omit the ID to recover the latest run. Use `cancel_make_run` for an explicit stop. The run's `timeout_seconds` applies independently of client request deadlines.

1. **Lint**: call `run_lint` (`make lint`). Read the result and `sippy-dev-logs/run_lint.log`.

2. **Unit tests**: call `run_tests` (`make test`). This includes the Component Readiness Playwright BDD suite, Go tests, Vitest, and MCP pytest. Read the result and `sippy-dev-logs/run_tests.log`.

3. **E2e**: call `run_e2e` (`make e2e`). BigQuery coverage is skipped by default; pass `include_bigquery=true` only when that coverage is needed and credentials are configured. Read the result and `sippy-dev-logs/run_e2e.log`. E2e logs and coverage are retained under `ARTIFACT_DIR/e2e/` (default: `sippy-dev-logs/artifacts/e2e/`).

E2e works on the host (starts dedicated PostgreSQL/Redis containers via Podman) and inside the devcontainer (creates a temporary `sippy_e2e` database on the existing PostgreSQL). Preserve inherited service credentials and connection options; do not replace authenticated settings with passwordless defaults.

If tools are unavailable or prerequisites fail, follow `mcp/README.md` to configure `mcp/run.sh` with the setup environment and required permissions. After correcting the setup, rerun the failed step before continuing. Report failed or unrun targets explicitly; passing individual components does not mean the full target passed.
