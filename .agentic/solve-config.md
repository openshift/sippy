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
