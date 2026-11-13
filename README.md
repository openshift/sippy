# Sippy

<img src=https://raw.github.com/openshift/sippy/main/sippy.svg height=100 width=100>

Sippy (Continuous Integration Private Investigator) analyzes OpenShift Prow CI job and test results.
It surfaces release health and regressions through reports such as Component Readiness.
Users can filter results by job, test, and configuration variants such as network, upgrade, release, and platform.
Sippy also exposes REST APIs for programmatic access to its data and reports.

Sippy Chat has moved to Chai Bot in Slack. Existing `/chat` links display a transition page.

## Typical usage

See [DEVELOPMENT.md](DEVELOPMENT.md) for information about standing up a
local environment.

Agentic solve and review use the `sippy-dev` MCP `run_tests`, `run_lint`, and
`run_e2e` tools for unit tests, lint, and e2e. Poll `get_make_result` when a tool
returns a run ID, keeping the MCP server alive until completion. See
[MCP test execution](mcp/README.md#test-execution) for service, authentication,
and permission requirements. The runner sets
`SIPPY_TEST_OUTPUT_DIR` to `ARTIFACT_DIR/e2e` for e2e logs and coverage, preserving
CI artifact collection. Direct `scripts/e2e.sh` runs may set this variable to a
writable directory; its default is the current directory.

## API

See [the API documentation](pkg/api/README.md)

## Frontend

See [the front end documentation](sippy-ng/README.md)

## Database

See [database tuning](docs/database-tuning.md) for required PostgreSQL
parameter group settings.

### Backfilling summary tables

Use `sippy backfill` to rebuild daily totals or cumulative summaries for an
inclusive date range. By default, the command processes every release. Repeat
`--exclude-release` to skip releases during both partition setup and the
summary backfill:

```sh
sippy backfill --table=daily-totals --start-date=2026-01-01 --end-date=2026-06-30 --exclude-release=Presubmits
sippy backfill --table=cumulative-summaries --start-date=2026-01-01 --end-date=2026-06-30 --exclude-release=Presubmits
```

To backfill Presubmits for only the most recent 90 calendar days, run a
separate backfill without the exclusion for that 90-day range. Set its start
date to 89 days before its end date, since both dates are included. The flag
skips work for excluded releases; it does not delete existing summary rows.
