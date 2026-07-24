#!/bin/sh
set -eu

MIN_PYTHON_MAJOR=3
MIN_PYTHON_MINOR=12

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VENV_DIR="$SCRIPT_DIR/.venv"

if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 not found on PATH" >&2
    exit 1
fi

py_version=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
py_ok=$(python3 -c "import sys; print(int(sys.version_info >= ($MIN_PYTHON_MAJOR, $MIN_PYTHON_MINOR)))")
if [ "$py_ok" != "1" ]; then
    echo "python3 $py_version found but >= $MIN_PYTHON_MAJOR.$MIN_PYTHON_MINOR is required" >&2
    exit 1
fi

if [ ! -x "$VENV_DIR/bin/python" ]; then
    rm -rf "$VENV_DIR"
    python3 -m venv "$VENV_DIR"
    "$VENV_DIR/bin/pip" install --upgrade pip -q
    "$VENV_DIR/bin/pip" install -r "$SCRIPT_DIR/requirements.txt" -q
fi

exec "$VENV_DIR/bin/python" "$SCRIPT_DIR/server.py" "$@"
