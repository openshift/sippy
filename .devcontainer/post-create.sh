#!/bin/bash
set -eu

if [[ "${SKIP_GO_TOOLS_INSTALL:-}" != "true" ]]; then
  echo "==> Installing Go IDE tools..."
  go install golang.org/x/tools/gopls@v0.21.1
  go install github.com/go-delve/delve/cmd/dlv@v1.27.1
  go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
fi

echo "==> Downloading Go module dependencies..."
go mod download

echo "==> Installing frontend dependencies..."
make npm

if [[ "${SKIP_CLAUDE_INSTALL:-}" != "true" ]]; then
  echo "==> Installing Claude Code..."
  curl -fsSL https://claude.ai/install.sh | sh
else
  echo "==> Skipping Claude Code install (SKIP_CLAUDE_INSTALL=true)."
fi

if [[ "${INSTALL_OPENCODE:-}" == "true" ]]; then
  echo "==> Installing OpenCode..."
  # Postinstall verifies the binary, which writes OpenCode state under HOME.
  # Run as the dev user so those files are not owned by root.
  npm install -g --prefix "$HOME/.local" --allow-scripts=@opencode/cli \
    "@opencode/cli@${OPENCODE_VERSION:-latest}"
  opencode --version
fi

echo "==> Setting up MCP server venv..."
# --seed installs pip. make test and mcp/run.sh call mcp/.venv/bin/pip.
uv venv --clear --seed mcp/.venv
uv pip install --python mcp/.venv/bin/python3 -r mcp/requirements.txt -q
mcp/.venv/bin/pip --version
mcp/.venv/bin/pytest --version

echo "==> Building sippy and seeding database..."
make sippy
./sippy seed-data --init-database --database-dsn="$SIPPY_SEED_DATABASE_DSN"

echo "==> Migrating prod-like database..."
./sippy migrate --database-dsn="$SIPPY_PRODLIKE_DATABASE_DSN"

if [ -n "${HOST_WORKSPACE_FOLDER:-}" ]; then
  host_project_dir=$(echo "$HOST_WORKSPACE_FOLDER" | sed 's|/|-|g')
  claude_projects="$HOME/.claude/projects"
  if [ -d "$claude_projects/$host_project_dir" ] && [ ! -e "$claude_projects/-workspace" ]; then
    ln -s "$claude_projects/$host_project_dir" "$claude_projects/-workspace"
    echo "==> Linked Claude conversations from host project"
  fi
fi

echo "==> Dev environment ready."
