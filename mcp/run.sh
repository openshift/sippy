#!/bin/sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VENV_DIR="$SCRIPT_DIR/.venv"

if [ ! -x "$VENV_DIR/bin/python" ]; then
    rm -rf "$VENV_DIR"
    python3.12 -m venv "$VENV_DIR"
    "$VENV_DIR/bin/pip" install --upgrade pip -q
    "$VENV_DIR/bin/pip" install -r "$SCRIPT_DIR/requirements.txt" -q
fi

exec "$VENV_DIR/bin/python" "$SCRIPT_DIR/server.py" "$@"
