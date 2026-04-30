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
python3.12 -m venv mcp/.venv
mcp/.venv/bin/pip install --upgrade pip -q
mcp/.venv/bin/pip install -r mcp/requirements.txt -q

echo "==> Dev environment ready."
