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
- **GCP auth** — the setup will prompt you to run `gcloud auth application-default login`

## Manual setup

If you prefer to set up manually:

1. Install [Podman](https://podman.io/) v4+ and [devcontainer CLI](https://github.com/devcontainers/cli) (`npm install -g @devcontainers/cli`)
2. **macOS**: run `podman machine init && podman machine start`
3. **Linux**: run `systemctl --user enable --now podman.socket` and install `podman-docker`
4. Copy `.devcontainer/.env.example` to `.devcontainer/.env` and fill in your values
5. Run `devcontainer up --workspace-folder .` (add `--docker-path podman` on macOS)
6. Exec in: `podman exec -it sippy-dev bash` and run `gcloud auth application-default login`

## Services

| Service          | Container           | Port | Access from devcontainer |
| ---------------- | ------------------- | ---- | ------------------------ |
| PostgreSQL       | `sippy-postgres`    | 5432 | `$SIPPY_DATABASE_DSN`    |
| Redis            | `sippy-redis`       | 6379 | `$REDIS_URL`             |
| Sippy API        | inside devcontainer | 8080 | `http://localhost:8080`  |
| React dev server | inside devcontainer | 3000 | `http://localhost:3000`  |

Ports 8080 and 3000 are published to the host.

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

`make lint` (without `CI=true`) and `make e2e` require nested containers and
**do not work inside the devcontainer**. Run them on the host, or use `CI=true make lint`
inside the container for the Go linter.

### TODO

- [ ] Make **`make lint`** reliable inside the devcontainer without nested Podman
      (e.g. always use host `golangci-lint` when present, or default to `CI=true`).
- [ ] Make **`make e2e`** reliable inside the devcontainer without nested Podman
      (e.g. reuse the `sippy-postgres` / `sippy-redis` services instead of spawning new containers).

## Rebuilding

```bash
podman rm -f sippy-dev 2>/dev/null
devcontainer up --workspace-folder . --remove-existing-container  # add --docker-path podman on macOS
```

Or from Cursor: "Dev Containers: Rebuild Container Without Cache".

## Cleanup

```bash
podman rm -f sippy-dev sippy-postgres sippy-redis 2>/dev/null
podman network rm sippy-net 2>/dev/null
```
