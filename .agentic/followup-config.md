## Write Tests for New Code

If your follow-up changes introduce new functions, components, or endpoints,
write tests for them. See `.agentic/solve-config.md` for the full testing
requirements (table-driven Go tests, no storage client mocks, function-type
seams for narrow dependencies, Vitest for React).

## Verify and Push

1. Run `make test` and `make lint`.
2. Run e2e tests using `run_e2e` MCP tool. Must pass before pushing.
3. For frontend changes, use Playwright MCP tools. Upload screenshots
   via `upload-screenshot` skill.
4. Commit fixes referencing the review feedback.
5. Push: `git push fork HEAD` (or `git push origin HEAD`).

## Environment

- PostgreSQL is available at localhost:5432 (user: postgres, trust auth).
- Redis is available at localhost:6379.

## Additional Instructions

- Sippy uses dependency injection via function-type fields on structs for testability.
  Search for existing patterns before introducing new ones.
