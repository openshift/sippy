# Sippy dev MCP server

Python [FastMCP](https://github.com/jlowin/fastmcp) server (`server.py`) that exposes Sippy development commands requiring process management to **Cursor**, **Claude Code**, and **Codex**.

Simple local commands are documented in project instructions via [APM](../apm.yml); see `.apm/instructions/dev-commands.instructions.md`.

## Setup

**`mcp/run.sh`** is the entry point. It auto-creates the virtualenv (`mcp/.venv`) and installs dependencies on first run. No manual setup is required. The devcontainer's `post-create.sh` pre-warms the venv with `uv venv --seed` so `mcp/.venv/bin/pip` exists. `make test` and `mcp/run.sh` call that pip. `uv venv` without `--seed` does not install it.

- **Python 3.12** required. The devcontainer image installs it; on macOS use `brew install python@3.12`.
- Dependencies are installed from the hashed lockfile at `mcp/requirements.lock`.
- Regenerate the lockfile after changing `mcp/requirements.txt`:

  ```bash
  uv pip compile mcp/requirements.txt --generate-hashes --python-version 3.12 --output-file mcp/requirements.lock
  ```

- **Manual venv setup** (if you prefer not to use `run.sh`):

  ```bash
  python3.12 -m venv mcp/.venv
  mcp/.venv/bin/pip install --require-hashes -r mcp/requirements.lock
  ```

## Editor configuration

| Client      | Config file             |
| ----------- | ----------------------- |
| Cursor      | `.cursor/mcp.json`      |
| Claude Code | `.mcp.json` (repo root) |
| Codex       | `.codex/config.toml`    |

All point to `mcp/run.sh`. This works on the host and inside the devcontainer without path changes.

## Server id in Cursor

The MCP server key in config is **`sippy-dev`**. Cursor may expose tools under a **prefixed** server name (e.g. `project-0-workspace-sippy-dev`). Use the server id your client lists when calling tools.

## Tools

Commands use the **repo root** as working directory unless noted. Most long outputs go to **`sippy-dev-logs/`** (see `.gitignore`).

| Tool                 | What it runs                                                                | Default log                             |
| -------------------- | --------------------------------------------------------------------------- | --------------------------------------- |
| `regression_cache`   | `go run ./cmd/sippy load --loader regression-cache` (BigQuery + Redis + DB) | `sippy-dev-logs/regression_cache.log`   |
| `restore_prodlike_db`| `scripts/restore_prodlike_db.sh` (drop/recreate `prodlike` from backup)    | `sippy-dev-logs/restore_prodlike_db.log`|
| `sippy_serve`        | Background `go run ./cmd/sippy serve` (API/UI, typically port **8080**)     | `sippy-dev-logs/sippy_serve.log`        |
| `sippy_ng_start`     | Background `npm start` in `sippy-ng/` (typically port **3000**)             | `sippy-dev-logs/sippy_ng_start.log`     |
| `sippy_stop`         | Stops background `sippy_serve` and `sippy_ng_start` processes               | none                                    |
| `run_tests`          | `make test` (BDD, Go, Vitest, MCP)                                         | `sippy-dev-logs/run_tests.log`          |
| `run_lint`           | `make lint` (Go, ESLint, dependency audit)                                 | `sippy-dev-logs/run_lint.log`           |
| `run_e2e`            | `make e2e` (BigQuery coverage is opt-in)                                    | `sippy-dev-logs/run_e2e.log`            |

Optional parameters (timeouts, paths, DSNs, etc.) are documented on each function in **`server.py`**.

`restore_prodlike_db` takes a repo-relative backup path. Supported formats are `.sql` files (restored with `psql`), custom-format `.dump` files, and directory backups containing `toc.dat` (both restored with `pg_restore`). The backup is validated before `prodlike` is dropped, so an ordinary directory or unreadable archive leaves the database untouched. The same rules apply when running `scripts/restore_prodlike_db.sh <backup-path>` directly.

> **Cost caution:** `regression_cache` issues BigQuery queries that cost real money. Run it only when explicitly needed and never more than once per request.

### Credentials and environment

- **Service account JSON** (BigQuery / GCS): pass `bigquery_credentials_file` where supported, or set **`SIPPY_BIGQUERY_CREDENTIALS_FILE`** or **`GOOGLE_APPLICATION_CREDENTIALS`** to an existing file path. Typical local file: `sippy-bigquery-job-importer-key.json` at repo root.
- **E2e BigQuery coverage**: `run_e2e` skips BigQuery coverage by default, even when credentials are configured in the environment. Pass `include_bigquery=true` to opt in and resolve credentials using the precedence above.
- **Data mode**: `SIPPY_DATA_MODE` selects between `seed` (default) and `prod-like`. In `seed` mode, `sippy_serve` uses the postgres data provider with seed data. In `prod-like` mode, it uses bigquery with `config/views.yaml` and the `prodlike` database. On startup, the MCP server loads **`.devcontainer/.env`** if present (`python-dotenv`, `override=False`), so values match the devcontainer when the MCP parent process does not inherit `--env-file` variables.
- **Postgres DSNs**: `SIPPY_SEED_DATABASE_DSN` (seed DB), `SIPPY_PRODLIKE_DATABASE_DSN` (prod-like DB), or the legacy `SIPPY_DATABASE_DSN`. Per-tool arguments override all env vars.
- **Redis**: `REDIS_URL` or per-tool arguments; see `server.py` for defaults.
- **`regression_cache`** always targets the prod-like database regardless of `SIPPY_DATA_MODE`.

### Test execution

Use the **`sippy-dev` MCP `run_tests`, `run_lint`, and `run_e2e` tools** in both
agentic solve and review phases. They execute `make test`, `make lint`, and
`make e2e`, respectively, through the same environment preparation.
Register `mcp/run.sh` in each client's MCP configuration and start it after service
setup, with the setup environment inherited. A successful ordinary e2e presubmit
alone does not verify this MCP execution path.

Environment preparation is target-specific. Lint prepares writable caches without
validating artifact or service settings. Unit tests additionally prepare artifacts
and browser caches while preserving inherited database settings. Only e2e selects
the seed database, derives the Redis database, and sets `SIPPY_TEST_OUTPUT_DIR`.

The e2e runner preserves `SIPPY_SEED_DATABASE_DSN` (preferred) or `SIPPY_DATABASE_DSN`,
including passwords and connection options. PostgreSQL uses password authentication;
do not substitute a passwordless DSN. `SIPPY_E2E_REDIS_URL` overrides Redis for e2e;
otherwise the runner uses logical database 1 on `REDIS_URL`, preserving authentication.
Derived Redis URLs discard query options; explicit e2e Redis URLs with query
options fail the prerequisite check because Sippy's Redis client rejects them.
The probe matches that client's password-only AUTH, ignoring URI usernames.
Rejected Redis commands identify AUTH, SELECT, or PING without exposing replies.
No credential values should appear in instructions, tool responses, or logs.

The unit-test subprocess checks that it can open local listeners; lint does not
require sockets or database services. The e2e subprocess also checks its API
and metrics ports (18080 and 12112). In a devcontainer, it also checks PostgreSQL
authentication and database-creation permission and an authenticated Redis PING.
Host runs additionally check the fixed PostgreSQL and Redis container ports
(23433 and 23479) before provisioning containers. Unknown make targets are rejected.
The PostgreSQL prerequisite uses the same local URI validation and target-override
removal as the e2e launcher, including clearing inherited `PGHOSTADDR` and
`PGSERVICE`. It then decodes the connection URI into libpq `PG*` environment
variables so passwords are absent from `psql` arguments. Other query options,
including credentials and SSL settings, are preserved. Options without a libpq
environment equivalent fail before `psql` runs. The local connection gets a
10-second timeout plus five seconds for the permission query.
It then executes `make e2e` with the same environment and permissions. Outside a
devcontainer, `scripts/e2e.sh` provisions its dedicated service containers.
The e2e launcher uses Python 3 for URI validation and streaming log redaction.
Setup diagnostics distinguish URI validation, executable startup, connection,
timeout, TLS, and rejected Redis operations without exposing credentials or raw
client diagnostics. Explicit empty URI passwords override inherited passwords.
The URI helper accepts only `redact`, `validate`, and `database` commands and
reports credential-free validation reasons.
Before database setup, it strips `host`, `hostaddr`, `dbname`, and `service`
query overrides, including encoded keys, and clears inherited `PGHOSTADDR` and
`PGSERVICE`. Destructive helpers reject remaining target overrides or non-local
URI authorities. Other connection options are preserved.
The runner cannot grant missing socket permissions or start services hidden by the
client's sandbox; configure the MCP process with those permissions in both phases.

Usable inherited cache directories are retained. For unit tests and e2e this
includes Playwright's default browser cache and its `PLAYWRIGHT_BROWSERS_PATH=0`
hermetic-install setting.
Unwritable caches and unset Go, npm and temporary directories fall back to
`sippy-dev-logs/test-cache/`.
For unit tests and e2e, `ARTIFACT_DIR` is preserved and must be writable by the MCP
server; the runner fails early rather than losing CI artifacts. Without it,
artifacts go to `sippy-dev-logs/artifacts/`. The e2e runner sets `SIPPY_TEST_OUTPUT_DIR` to
`ARTIFACT_DIR/e2e/` for e2e logs and coverage, including `e2e-seed.log`, `e2e.log`,
and `e2e-gotestsum.log`. Tool summaries and prerequisite diagnostics are in
`sippy-dev-logs/run_tests.log`, `run_lint.log`, and `run_e2e.log`.

Database setup/seeding errors stop execution immediately. An API process that exits
before readiness reports its exit status and recent log output. Cleanup preserves
the original failure status, and logs remain available for diagnosis and collection.
API readiness has a 600-second elapsed-time deadline, including HTTP requests.
Test output is redacted before it reaches the console or artifact log, including
URI userinfo and `password`/`sslpassword` query parameters, including encoded keys.
Write failures in the seed and test-output pipelines fail the run. Coverage
generation during cleanup is best effort and does not replace the test exit status.
Cleanup only reports test results from the current invocation.
API logs use a private named FIFO and a regular background redactor process,
allowing Bash 3.2 to wait for complete logs before early-exit diagnostics or cleanup.
Make tool calls are serialized within the MCP server because they share logs,
artifacts, listeners, and services. Each make runner starts a separate process
session. On timeout or task cancellation, the runner sends SIGTERM to the process
group, allows up to 60 seconds for e2e cleanup (five seconds for other targets),
then sends SIGKILL to remaining group members and reaps make. Cancellation is
re-raised after cleanup completes.

### Background processes

`sippy_serve` and `sippy_ng_start` spawn detached processes. A second start is refused if a matching process is already running (see `server.py` for detection logic).

## Agent skills

Agent-oriented shortcuts are defined in **`.apm/skills/`** and deployed by **`make apm`** to editor command and skill directories, including **`.claude/commands/`**, **`.claude/skills/`**, **`.agents/skills/`**, and **`.codex/`**, for example:

- `sippy-dev-migrate`, `sippy-dev-regression-cache`, `sippy-dev-serve`, `sippy-dev-frontend`
- `sippy-dev-app` (backend + frontend)
- `sippy-dev-tests` (MCP tool order: `run_lint` → `run_tests` → `run_e2e`, executing the corresponding full make targets)
- `sippy-generate-release-views` (create next-release Component Readiness views from an existing release)
- `sippy-update-ga-release-views` (switch a newly GA release's views from `now` to `ga` dates)

Skills that take arguments accept them positionally and prompt for anything omitted. For example, `/sippy-generate-release-views 4.21 4.22` copies the 4.21 views to 4.22 (Codex users can invoke the skill by name with the same arguments). It previews changes with `python3 scripts/generate_release_views.py <source> <target>` and asks for confirmation before writing `config/views.yaml`.

## Changing the server

After editing **`server.py`**, restart the **sippy-dev** MCP server (or reload the editor) so tool lists stay in sync.

## Adding tools

Add `@mcp.tool()` handlers in `server.py`, mirror existing patterns (`subprocess`, `_repo_path`, `_ensure_dev_log_dir`, credentials helpers), then restart MCP.
