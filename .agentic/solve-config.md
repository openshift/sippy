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

- Write tests for new components, hooks, and significant UI logic.
- Place test files next to the component (e.g., `MyComponent.test.jsx`
  alongside `MyComponent.jsx`).
- Follow existing Vitest patterns in `sippy-ng/`.

### API endpoints

- New or modified API endpoints must have tests that verify request handling,
  response structure, and error cases.

## Build, Test, and Verify

1. Run `make test` to verify your changes work.
2. Run `make lint` to check for linting issues.

## Test Locally

Use the sippy-dev MCP tools:
- `sippy_serve` starts the API server (builds automatically)
- `sippy_ng_start` starts the React frontend dev server
- `run_e2e` runs the end-to-end test suite

E2e tests MUST pass before pushing.

For frontend changes, use Playwright MCP tools. Take screenshots and upload via
`upload-screenshot` skill. Include image links in PR description.

## Environment

- PostgreSQL is available at localhost:5432 (user: postgres, trust auth).
- Redis is available at localhost:6379.
- Seed: `./sippy seed-data --init-database`

## Additional Instructions

- Sippy uses dependency injection via function-type fields on structs for testability.
  Search for existing patterns before introducing new ones.
