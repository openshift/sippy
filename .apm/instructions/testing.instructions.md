---
description: "Testing guidelines and constraints for Sippy"
applyTo: "**/*_test.go"
---

* Use `go vet` and `go test ./pkg/...` to validate changes before resorting to a full e2e run.
* Use the `sippy-dev` MCP `run_tests`, `run_lint`, and `run_e2e` tools for the full `make test`, `make lint`, and `make e2e` targets in both agentic solve and review phases. See `mcp/README.md` for setup, inherited service settings, and required permissions.
* Run the MCP `run_e2e` tool to verify your changes don't break end-to-end tests. BigQuery coverage is opt-in via `include_bigquery=true`. You **MUST** read the tool result and `sippy-dev-logs/run_e2e.log` for results; e2e logs and coverage are retained under `ARTIFACT_DIR/e2e/` (default: `sippy-dev-logs/artifacts/e2e/`). Do not re-run e2e just to search different output. Report failures or unrun checks accurately; component passes do not mean the full target passed.
* Do not mock storage clients (BigQuery, GCS, Postgres, etc.) when constructing golang unit tests.
  Go does not make it easy to substitute mocks for concrete SDK clients, and mock-heavy tests tend
  to verify the mock implementation rather than real behavior. Instead, structure code to separate
  pure logic from client calls. Unit test the logic functions directly (validation, data
  transformation, result aggregation and analysis). Enable testing against real storage systems with
  functional tests that skip unless the user supplies connection credentials via environment
  variables (see `releasesync_functional_test.go` for the pattern).
* When a struct method needs a **single, narrow query or RPC** that would otherwise force a
  database connection in tests, extract that call behind a **function type field** on the struct
  (e.g. `type counterFunc func(id uint) (int, error)`). This is a thin adapter seam, not a
  general-purpose mock — the function type replaces one specific call, not an entire storage
  client. The production constructor wires the real implementation; tests supply a closure.
  See `regressiontracker.go` (`failureCounterFunc`) for the pattern. This does **not** override
  the rule above: do not mock or stub BigQuery, GCS, Postgres clients, or any broad storage
  interface.
* Prefer **table-driven tests** with descriptive case names. Search the same package for
  existing test patterns before writing new ones.
* New or modified functionality must include test coverage:
  new Go functions and methods should have corresponding unit tests,
  bug fixes should include a regression test that fails without the fix,
  and pure functions (no DB/external dependencies) should always be tested.
  If a function is hard to test, consider refactoring it into simpler functions 
  (disentangle separate conceptual chunks).
  Exceptions: trivial changes (renaming, formatting, comments), generated code or
  configuration-only changes, and refactors already covered by existing tests.
