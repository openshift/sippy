Extra context for agentic review responses, layered on `/openshift-developer:address-review-pr`.

## Verify and Push

1. Run `run_tests` and `run_lint` through the `sippy-dev` MCP server
   (they execute `make test` and `make lint`).
2. Run e2e tests using `run_e2e` MCP tool. Must pass before pushing.
3. For frontend changes, use Playwright MCP tools. Upload screenshots
   via `upload-screenshot` skill.
4. Commit fixes referencing the review feedback.
5. Push: `git push fork HEAD` (or `git push origin HEAD`).

Make tools return within 30 seconds. If a run ID is returned, keep the MCP server
alive and poll `get_make_result` until the final result arrives before proceeding.
Client request timeouts leave the run active; omit the ID to recover the latest run.
Use `cancel_make_run` to stop a run explicitly.

## Environment

- PostgreSQL uses password authentication. The MCP server must inherit setup's
  `SIPPY_SEED_DATABASE_DSN` (or `SIPPY_DATABASE_DSN`), `REDIS_URL`, and authenticated
  service settings. `SIPPY_E2E_REDIS_URL` is an optional override. Never log
  credentials or retry with a passwordless DSN.
- Use the `sippy-dev` MCP `run_e2e` tool in both solve and review. Its subprocess
  checks local listener permissions, authenticated services, and writable output paths.
- If the tool is unavailable, fix MCP registration/startup for that phase. If its
  prerequisite check fails, fix the MCP environment, socket permissions, or artifact
  mount. Keep `ARTIFACT_DIR` writable so CI can collect results.
- See [MCP test execution](mcp/README.md#test-execution) for logs and artifacts.

## Additional Instructions

- Sippy uses dependency injection via function-type fields on structs for testability.
  Search for existing patterns before introducing new ones.
