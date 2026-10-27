# Devcontainer Setup

The devcontainer provides a full-stack development environment with Go, Node.js,
PostgreSQL, Redis, and all build tools. It runs on **Podman** and works with both
**Cursor** and **Claude Code**.

## Quick start

Run the `/sippy-dev-setup` slash command in Claude Code or Cursor. It detects your
OS, checks prerequisites, creates the `.env` file, starts the container, and walks
you through GCP authentication.

## What you'll need to provide

- **GCP project ID** (`ANTHROPIC_VERTEX_PROJECT_ID`) — for Claude Code via Vertex AI
- **GCP auth** — run `gcloud auth application-default login` on the host before starting the container (credentials are mounted read-only)
- **BigQuery job SA JSON** at **`/workspace/sippy-bigquery-job-importer-key.json`** (gitignored); **`SIPPY_BIGQUERY_CREDENTIALS_FILE`** defaults to that path in `devcontainer.json`. Override in `.devcontainer/.env` if your key lives elsewhere.

## Manual setup

If you prefer to set up manually:

1. Install [Podman](https://podman.io/) v4+ and [devcontainer CLI](https://github.com/devcontainers/cli) (`npm install -g @devcontainers/cli`)
2. **macOS**: run `podman machine init && podman machine start`
3. **Linux**: run `systemctl --user enable --now podman.socket` and install `podman-docker`
4. Copy `.devcontainer/.env.example` to `.devcontainer/.env` and fill in your values
5. Run `devcontainer up --workspace-folder .` (add `--docker-path podman` on macOS)
6. Exec in: `podman exec -it sippy-dev bash` and run `gcloud auth application-default login`

## Services

| Service              | Container           | Port | Access from devcontainer     |
| -------------------- | ------------------- | ---- | ---------------------------- |
| PostgreSQL (seed)    | `sippy-postgres`    | 5432 | `$SIPPY_SEED_DATABASE_DSN`   |
| PostgreSQL (prod-like) | `sippy-postgres`  | 5432 | `$SIPPY_PRODLIKE_DATABASE_DSN` |
| Redis                | `sippy-redis`       | 6379 | `$REDIS_URL`                 |
| Sippy API            | inside devcontainer | 8080 | `http://localhost:8080`      |
| React dev server     | inside devcontainer | 3000 | `http://localhost:3000`      |

Ports 8080 and 3000 are published to the host.

## Data modes

The devcontainer provides two PostgreSQL databases in the same instance, controlled by `SIPPY_DATA_MODE` (optional in `.devcontainer/.env`; see `.env.example`):

| Mode | DB name | Data provider | Views file | Description |
|------|---------|---------------|------------|-------------|
| `seed` (default) | `postgres` | `postgres` | `config/seed-views.yaml` | Synthetic seed data, no BigQuery needed |
| `prod-like` | `prodlike` | `bigquery` | `config/views.yaml` | Real data loaded via `regression_cache` |

Switch modes: `export SIPPY_DATA_MODE=prod-like`. The MCP `sippy_serve` tool automatically adjusts `data_provider`, `views_file`, and database DSN based on the mode. Restart the server after switching.

## Starting the container

### Claude Code

```bash
devcontainer up --workspace-folder .  # add --docker-path podman on macOS
podman exec -it sippy-dev bash
claude
```

### Cursor

Open the command palette and run "Dev Containers: Attach to Running Container" > `sippy-dev`.

## Lint and e2e

`make lint` works inside the devcontainer (uses the installed `golangci-lint`). Outside the container, the script falls back to running `golangci-lint` via Podman.

**`make e2e`** can run inside the devcontainer: `scripts/e2e.sh` uses `sippy-postgres` with a dedicated `sippy_e2e` database and **`redis://sippy-redis:6379/1`** (Redis logical DB `1`, separate from dev `sippy serve` on DB `0`). Override with **`SIPPY_E2E_REDIS_URL`** if needed.

## Rebuilding

```bash
podman rm -f sippy-dev 2>/dev/null
devcontainer up --workspace-folder . --remove-existing-container  # add --docker-path podman on macOS
```

Or from Cursor: "Dev Containers: Rebuild Container Without Cache".

## Reusing the toolchain

Other devcontainers can build this Dockerfile directly and call
`post-create.sh` from the Sippy checkout to share tool versions without copying
them. Rebuild those containers after Dockerfile changes. Recreate them after
setup script or environment changes to rerun `post-create.sh`.

Claude Code is installed during post-create by default. Set
`SKIP_CLAUDE_INSTALL=true` to skip it. Set `INSTALL_OPENCODE=true` to install
OpenCode V2 using `@opencode/cli`; `OPENCODE_VERSION` defaults to `latest` or can
be an exact package version. Set these in `.devcontainer/.env` for the standalone
container, or `containerEnv` for a consuming devcontainer. They are runtime
variables, not build arguments. No OpenCode credentials are mounted automatically.

For example, a devcontainer that covers several related checkouts can build
this Dockerfile and install OpenCode alongside Claude Code during post-create
setup:

```json
"build": {
  "dockerfile": "../sippy/.devcontainer/Dockerfile",
  "context": "../sippy/.devcontainer"
},
"containerEnv": {
  "INSTALL_OPENCODE": "true"
}
```

Adjust `dockerfile` and `context` relative to the consuming devcontainer's
configuration file. Mount the Sippy checkout and call its `post-create.sh`
from `postCreateCommand`, with that checkout as the working directory. Supply
the database and Redis environment variables from `devcontainer.json`, and run
`init-services.sh` on the host first. The setup script installs frontend and
MCP dependencies, builds Sippy, initializes seed data, and migrates the
prod-like database, so use development databases intended for these operations.
The MCP virtualenv is created with `uv venv --seed` because `make test` and
`mcp/run.sh` call `mcp/.venv/bin/pip`. `uv venv` without `--seed` does not
install pip. Reusing this `post-create.sh` picks up that behavior.
`make test-bdd` installs Playwright Chromium when it runs. That browser is
not part of the image build.

Set `SKIP_CLAUDE_INSTALL` to `"true"` in `containerEnv` to install OpenCode
instead of Claude Code. Neither coding agent is installed during the image build.

OpenCode is installed as the dev user into `~/.local` (already on `PATH`),
with only `@opencode/cli`'s install script explicitly allowed. Do not install it
with `sudo`: its postinstall verifies the binary and can leave root-owned
OpenCode state in the dev user's home directory.

When composing setup scripts, `SKIP_GO_TOOLS_INSTALL=true` skips installing
gopls, delve, and staticcheck if another script already installed them. Module
downloads, frontend setup, MCP setup, and database initialization still run.

## Cleanup

```bash
podman rm -f sippy-dev sippy-postgres sippy-redis 2>/dev/null
podman network rm sippy-net 2>/dev/null
```
