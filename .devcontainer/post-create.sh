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

echo "==> Setting up MCP server venv..."
python3 -m venv mcp/.venv
mcp/.venv/bin/pip install --upgrade pip -q
mcp/.venv/bin/pip install -r mcp/requirements.txt -q

echo "==> Configuring Playwright MCP server for Claude..."
claude mcp add playwright -- npx @playwright/mcp@latest --executable-path /usr/lib64/chromium-browser/headless_shell

echo "==> Building sippy and seeding database..."
make sippy
./sippy seed-data --init-database --database-dsn="$SIPPY_DATABASE_DSN"

echo "==> Dev environment ready."
