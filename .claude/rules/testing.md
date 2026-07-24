---
paths:
  - "**/*_test.go"
---

* Use `go vet` and `go test ./pkg/...` to validate changes before resorting to a full e2e run.
* Run `make e2e 2>&1 | tee e2e-test.log` to verify your changes don't break end-to-end tests. You **MUST** read the log file (`e2e-test.log`) for results. Do not re-run e2e just to grep for different things. All output is already in the log file.
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
