## Write Tests for New Code

You MUST write tests for any new functionality you introduce. PRs that add new
code without corresponding tests are incomplete.

### Go (backend)

- Write unit tests for every new exported function or method.
- Write unit tests for non-trivial unexported functions (validation, data
  transformation, aggregation logic).
- Use **table-driven tests** with descriptive case names. Search the same
  package for existing test patterns before writing new ones.
- Do **not** mock storage clients (BigQuery, GCS, Postgres). Instead, separate
  pure logic from client calls and test the logic directly.
- When a struct method needs a single narrow query or RPC for testability,
  extract it behind a **function-type field** on the struct (see
  `regressiontracker.go` for the pattern).
- Place test files next to the code they test (`foo_test.go` alongside `foo.go`).

### React (frontend)

Test UI behavior with the Playwright BDD suite in `sippy-ng/bdd/`. Add or
update Gherkin scenarios under `sippy-ng/bdd/features/` for new or changed
pages, components, and user flows. Step definitions, page objects, and
fixtures live next to the existing suite. Run it with `make test-bdd`.

Use Vitest only for helper functions (pure utilities such as those in
`sippy-ng/src/helpers.js`). Do not add Vitest or React Testing Library tests
for components, pages, or other UI behavior.

### API endpoints

- New or modified API endpoints must have tests that verify request handling,
  response structure, and error cases. These belong in the **e2e testing suite**
  (run via the `sippy-dev` MCP `run_e2e` tool).

## Build, Test, and Verify

1. Use the `sippy-dev` MCP `run_tests` tool to run `make test`.
2. Use its `run_lint` tool to run `make lint`.

If a make tool returns a run ID, keep the MCP server alive and poll `get_make_result`
until completion before starting the next target or reporting success. Client request
timeouts leave runs active; omit the ID to recover the latest run. Use `cancel_make_run`
for an explicit stop.

## Test Locally

Use the sippy-dev MCP tools:

- `sippy_serve` starts the API server (builds automatically)
- `sippy_ng_start` starts the React frontend dev server
- `run_tests` runs `make test`; `run_lint` runs `make lint` with writable caches/output
- `run_e2e` is the canonical end-to-end test runner in both solve and review

E2e tests MUST pass before pushing.

For frontend changes, use Playwright MCP tools. Take screenshots and upload via
`upload-screenshot` skill. Include image links in PR description.

## Environment

- PostgreSQL at localhost:5432 uses password authentication. Use the setup-supplied
  `SIPPY_SEED_DATABASE_DSN` (or `SIPPY_DATABASE_DSN`), never a passwordless connection.
- Redis settings come from `REDIS_URL`; `SIPPY_E2E_REDIS_URL` selects the isolated
  e2e logical database. Keep authentication settings in the inherited environment.
- The MCP server must inherit setup's service settings, a writable workspace and
  `ARTIFACT_DIR`, and permission to open local sockets. The runner checks these
  prerequisites in its test subprocess before building or seeding.
- If a test/lint tool is unavailable, correct MCP registration/startup in that phase.
  If a prerequisite fails, correct the MCP service environment or permissions.
  Do not replace the canonical runner with repeated shell commands or log credentials.
- See [MCP test execution](../mcp/README.md#test-execution) for logs and artifacts.

## Additional Instructions

- Sippy uses dependency injection via function-type fields on structs for testability.
  Search for existing patterns before introducing new ones.
