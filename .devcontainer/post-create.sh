#!/bin/bash
set -e

echo "==> Installing Go IDE tools..."
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install honnef.co/go/tools/cmd/staticcheck@latest

echo "==> Downloading Go module dependencies..."
go mod download

echo "==> Installing frontend dependencies..."
cd sippy-ng
npm install --ignore-scripts
cd ..

echo "==> Setting up MCP server venv..."
python3.12 -m venv mcp/.venv
mcp/.venv/bin/python -m pip install --upgrade pip
mcp/.venv/bin/python -m pip install -r mcp/requirements.txt

echo "==> Checking GCP auth..."
if command -v gcloud >/dev/null 2>&1; then
    if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then
        echo "    GCP credentials not found. Run 'gcloud auth application-default login' to authenticate."
    fi
else
    echo "    gcloud not found — skipping auth check."
fi

echo "==> Dev environment ready."
