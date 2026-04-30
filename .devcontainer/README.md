# Devcontainer Setup

The devcontainer provides a full-stack development environment with Go, Node.js,
PostgreSQL, Redis, and all build tools. It runs on **Podman** and works with both
**Cursor** and **Claude Code**.

## Prerequisites

- [Podman](https://podman.io/) v4+
- [devcontainer CLI](https://github.com/devcontainers/cli) — `npm install -g @devcontainers/cli`

### macOS

- Start `podman machine` before using the devcontainer:

  ```bash
  podman machine init   # first time only
  podman machine start
  ```

- For Cursor: set `"dev.containers.dockerPath": "podman"` in user settings (`Cmd+Shift+P` > "Preferences: Open User Settings (JSON)")

### Linux

- Podman runs natively — no machine required. Ensure the Podman socket is active:

  ```bash
  systemctl --user enable --now podman.socket
  ```

- Install `podman-docker` to provide the `docker` CLI alias — this lets devcontainer CLI and Cursor work without `--docker-path podman`:

  ```bash
  # Fedora/RHEL
  sudo dnf install podman-docker
  # Debian/Ubuntu
  sudo apt install podman-docker
  ```

- For Cursor (if not using `podman-docker`): set `"dev.containers.dockerPath": "podman"` in user settings (`Ctrl+Shift+P` > "Preferences: Open User Settings (JSON)")

## First-Time Setup

1. Copy the env file template and fill in your values:

   ```bash
   cp .devcontainer/.env.example .devcontainer/.env
   # Edit .devcontainer/.env with your credentials
   ```

2. Start the container:

   ```bash
   devcontainer up --workspace-folder .  # add --docker-path podman on macOS
   ```

   This automatically starts PostgreSQL and Redis via `init-services.sh`.

3. Exec into the container and authenticate with GCP:

   ```bash
   podman exec -it sippy-dev bash
   gcloud auth application-default login
   ```

## Starting the Container

### For Claude Code

```bash
# Start the container (if not already running)
devcontainer up --workspace-folder .  # add --docker-path podman on macOS

# Exec in and run Claude Code
podman exec -it sippy-dev bash
claude
```

Claude Code picks up the MCP server from `.mcp.json` at the repo root. Tool list
and usage: **[mcp/README.md](../mcp/README.md)**.

#### Claude Code environment variables

Claude Code uses Vertex AI for authentication. The following env vars must be set
in `.devcontainer/.env`:

| Variable                      | Description                    |
| ----------------------------- | ------------------------------ |
| `CLAUDE_CODE_USE_VERTEX`      | Set to `1` to enable Vertex AI |
| `ANTHROPIC_VERTEX_PROJECT_ID` | Your GCP project ID            |
| `CLOUD_ML_REGION`             | GCP region (e.g., `global`)    |

You must also run `gcloud auth application-default login` inside the container
on first use.

### For Cursor

Open the command palette (`Cmd+Shift+P` on macOS, `Ctrl+Shift+P` on Linux):

- **Container already running:** "Dev Containers: Attach to Running Container" > `sippy-dev`
- **Container not running:** "Dev Containers: Reopen in Container" to build and start it

Cursor reads `.cursor/mcp.json` for MCP; see
**[mcp/README.md](../mcp/README.md)** for tools, logs, and credentials.

## Services

| Service          | Container           | Port | Access from devcontainer |
| ---------------- | ------------------- | ---- | ------------------------ |
| PostgreSQL       | `sippy-postgres`    | 5432 | `$SIPPY_DATABASE_DSN`    |
| Redis            | `sippy-redis`       | 6379 | `$REDIS_URL`             |
| Sippy API        | inside devcontainer | 8080 | `http://localhost:8080`  |
| React dev server | inside devcontainer | 3000 | `http://localhost:3000`  |

Ports 8080 and 3000 are published to the host, so you can access them in your
browser at `http://localhost:8080` and `http://localhost:3000`.

## Lint and e2e inside the devcontainer

**`make lint`** (the Go path via `hack/go-lint.sh` without `CI=true`) and **`make e2e`**
(`scripts/e2e.sh`) are **not expected to work** when you run them only inside this
devcontainer today.

**Why:** `go-lint.sh` runs `golangci-lint` in a separate container image via
Podman/Docker. `e2e.sh` starts short-lived Postgres and Redis containers the same
way. That is **nested** container use. In many devcontainer setups (including
typical Cursor/cloud workspaces), rootless Podman inside the outer container cannot
get a working user-space network stack: **`/dev/net/tun`** is missing, so
slirp4netns/pasta fail, and alternatives such as **`--network host`** often hit
**`proc` mount / netlink** restrictions. Without nested networking, those helper
containers never start, so lint and e2e fail.

**What works today:** run **`make lint`** / **`make e2e`** on the **host** (or any
environment where Podman or Docker can run sibling containers normally), or set
**`CI=true`** for the Go part of lint so `hack/go-lint.sh` uses a host-installed
`golangci-lint` instead of spawning an inner container.

### TODO

- [ ] Make **`make lint`** reliable inside the devcontainer without nested Podman
      (for example: always use host `golangci-lint` when present, or document
      `CI=true` as the supported in-container path).
- [ ] Make **`make e2e`** reliable inside the devcontainer without nested Podman
      (for example: optional mode that uses the compose **`sippy-postgres`** /
      **`sippy-redis`** services with an isolated DB name and Redis logical DB, plus
      client tools in the image, or document running e2e only on the host).

## Rebuilding

If you change the Dockerfile or devcontainer config:

```bash
podman rm -f sippy-dev 2>/dev/null
devcontainer up --workspace-folder .  # add --docker-path podman on macOS --remove-existing-container
```

Or from Cursor: open the command palette and run "Dev Containers: Rebuild Container Without Cache".

## Cleanup

```bash
podman rm -f sippy-dev sippy-postgres sippy-redis 2>/dev/null
podman network rm sippy-net 2>/dev/null
```
