#!/bin/bash
set -eu

echo "==> Installing Go IDE tools..."
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install honnef.co/go/tools/cmd/staticcheck@latest

echo "==> Downloading Go module dependencies..."
go mod download

echo "==> Installing frontend dependencies..."
make npm

echo "==> Installing Claude Code..."
curl -fsSL https://claude.ai/install.sh | sh

echo "==> Setting up MCP server venv..."
python3 -m venv mcp/.venv
mcp/.venv/bin/pip install --upgrade pip -q
mcp/.venv/bin/pip install -r mcp/requirements.txt -q

echo "==> Configuring Claude Code plugins..."
claude mcp add playwright -- npx @playwright/mcp@latest --executable-path /usr/lib64/chromium-browser/headless_shell
claude plugin marketplace add openshift-eng/ai-helpers --scope project
claude plugin marketplace add anthropics/claude-plugins-official --scope project
claude plugin install golang@ai-helpers --scope project
claude plugin install typescript-lsp@claude-plugins-official --scope project
claude mcp add sippy-dev -- mcp/run.sh
claude mcp add --transport http atlassian https://mcp.atlassian.com/v1/mcp

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
