#!/bin/sh
set -eu

MIN_PYTHON_MAJOR=3
MIN_PYTHON_MINOR=12

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VENV_DIR="$SCRIPT_DIR/.venv"
HASH_FILE="$VENV_DIR/.requirements.hash"
LOCK_FILE="$SCRIPT_DIR/.venv-setup.lock"

REQ_HASH=""
if command -v shasum >/dev/null 2>&1; then
    REQ_HASH=$(shasum -a 256 "$SCRIPT_DIR/requirements.txt" | cut -d' ' -f1)
elif command -v sha256sum >/dev/null 2>&1; then
    REQ_HASH=$(sha256sum "$SCRIPT_DIR/requirements.txt" | cut -d' ' -f1)
fi

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

_venv_ok() {
    [ -x "$VENV_DIR/bin/python" ] && "$VENV_DIR/bin/python" -c "import dotenv, fastmcp" >/dev/null 2>&1
}

_needs_install() {
    if [ ! -d "$VENV_DIR" ]; then
        return 0
    fi
    if [ -n "$REQ_HASH" ] && { [ ! -f "$HASH_FILE" ] || [ "$(cat "$HASH_FILE" 2>/dev/null)" != "$REQ_HASH" ]; }; then
        return 0
    fi
    if ! _venv_ok; then
        return 0
    fi
    return 1
}

_remove_venv() {
    if [ -L "$VENV_DIR" ]; then
        rm -f "$VENV_DIR"
        return 0
    fi
    if [ ! -e "$VENV_DIR" ]; then
        return 0
    fi
    chmod -R u+w "$VENV_DIR" 2>/dev/null || true
    stale="$SCRIPT_DIR/.venv.stale.$$"
    mv "$VENV_DIR" "$stale"
    rm -rf "$stale" 2>/dev/null || true
}

_setup_venv() {
    if _needs_install; then
        _remove_venv
        python3 -m venv "$VENV_DIR"
        "$VENV_DIR/bin/pip" install --upgrade pip -q
        "$VENV_DIR/bin/pip" install -r "$SCRIPT_DIR/requirements.txt" -q
        if [ -n "$REQ_HASH" ]; then
            echo "$REQ_HASH" > "$HASH_FILE"
        fi
    fi
}

if [ "${SIPPY_DEV_SETUP_ONLY:-}" = "1" ]; then
    _setup_venv
    exit 0
fi

# fcntl works on Unix systems, including macOS where flock is not installed.
python3 - "$LOCK_FILE" "$0" <<'PY'
import fcntl
import os
import subprocess
import sys

with open(sys.argv[1], "a+") as lock_file:
    fcntl.flock(lock_file, fcntl.LOCK_EX)
    env = dict(os.environ, SIPPY_DEV_SETUP_ONLY="1")
    subprocess.run(["/bin/sh", sys.argv[2]], env=env, check=True)
PY

exec "$VENV_DIR/bin/python" "$SCRIPT_DIR/server.py" "$@"
