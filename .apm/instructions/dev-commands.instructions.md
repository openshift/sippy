---
description: "Common dev commands for database migration, linting, and testing"
applyTo: "**"
---

### Database migration

The devcontainer has two databases: **seed** (`postgres`) and **prod-like** (`prodlike`), controlled by `SIPPY_DATA_MODE` (default: `seed`).

To migrate both databases:

```
go run ./cmd/sippy migrate --database-dsn "$SIPPY_SEED_DATABASE_DSN"
go run ./cmd/sippy migrate --database-dsn "$SIPPY_PRODLIKE_DATABASE_DSN"
```

If no env vars are set, the dev default is: `postgresql://postgres:password@localhost:5432/postgres`

Restore **prod-like** from a backup: **`scripts/restore_prodlike_db.sh`** or MCP **`restore_prodlike_db`**. Instructions and slash-command text are defined in **`.apm/prompts/sippy-dev-restore-prodlike.prompt.md`**; run **`make apm`** after changing that file so **`apm install`** redeploys generated command files. Stop **`sippy serve`** first.

### Linting

Run lint: `make lint`

### Testing

Run unit tests: `make test`

This runs Go tests via gotestsum, sippy-ng Vitest tests, mcp pytest, and then the Component Readiness Playwright BDD suite.

Run only the Component Readiness browser tests: `make test-bdd`

That target installs Playwright's Chromium and runs `npm run test:bdd` in `sippy-ng`. The suite uses that browser, not `CHROME_PATH`. The **sippy-dev-bdd** slash command runs this target. Instructions are in **`.apm/prompts/sippy-dev-bdd.prompt.md`**.

Run integration tests: `make integration`

This runs SQL-level tests against a real PostgreSQL instance using testcontainers-go. Requires Docker or Podman.
