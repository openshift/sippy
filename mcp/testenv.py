"""Prepare and validate the environment used by MCP test subprocesses."""

import os
import socket
import ssl
import subprocess
import sys
import tempfile
from pathlib import Path
from urllib.parse import unquote, urlsplit


def writable_directory(path: Path) -> Path:
    """Probe real write access rather than relying on ownership or access bits."""
    path = path.resolve()
    path.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryFile(dir=path):
        pass
    return path


def prepare_environment(parent: dict[str, str], log_dir: Path) -> dict[str, str]:
    """Keep usable caches and service credentials, with private writable defaults."""
    env = parent.copy()
    if "npm_config_cache" not in env and "NPM_CONFIG_CACHE" in env:
        env["npm_config_cache"] = env["NPM_CONFIG_CACHE"]
    cache_root = log_dir / "test-cache"
    home = Path(env.get("HOME") or Path.home())
    for key in (
        "GOCACHE", "GOMODCACHE", "GOTMPDIR", "TMPDIR", "XDG_CACHE_HOME",
        "npm_config_cache",
    ):
        candidate = env.get(key)
        if key == "XDG_CACHE_HOME" and not candidate:
            candidate = str(home / ".cache")
        try:
            path = writable_directory(Path(candidate)) if candidate else None
        except OSError:
            path = None
        if path is None:
            path = writable_directory(cache_root / key.lower())
        elif key == "XDG_CACHE_HOME" and not env.get(key):
            continue
        env[key] = str(path)
    env["NPM_CONFIG_CACHE"] = env["npm_config_cache"]

    # Browser installs may already live in Playwright's platform default cache.
    # Its special value "0" selects the package-local hermetic installation.
    browser_path = env.get("PLAYWRIGHT_BROWSERS_PATH")
    if browser_path != "0":
        if sys.platform == "darwin":
            browser_cache = home / "Library/Caches/ms-playwright"
        elif sys.platform == "win32":
            browser_cache = Path(env.get("LOCALAPPDATA") or home / "AppData/Local") / "ms-playwright"
        else:
            browser_cache = Path(parent.get("XDG_CACHE_HOME") or home / ".cache") / "ms-playwright"
        try:
            path = writable_directory(Path(browser_path) if browser_path else browser_cache)
        except OSError:
            path = writable_directory(cache_root / "playwright_browsers_path")
            env["PLAYWRIGHT_BROWSERS_PATH"] = str(path)
        else:
            if browser_path or (sys.platform == "linux" and env.get("XDG_CACHE_HOME") != parent.get("XDG_CACHE_HOME")):
                env["PLAYWRIGHT_BROWSERS_PATH"] = str(path)

    # CI must mount its collection directory writable. Falling back silently would
    # lose test results even if the tests themselves succeeded.
    try:
        artifacts = writable_directory(Path(env.get("ARTIFACT_DIR") or log_dir / "artifacts"))
    except OSError as exc:
        raise ValueError("ARTIFACT_DIR must be writable by the MCP server; check the CI artifact mount") from exc
    env["ARTIFACT_DIR"] = str(artifacts)
    env["SIPPY_TEST_OUTPUT_DIR"] = str(writable_directory(artifacts / "e2e"))
    env["SIPPY_DATABASE_DSN"] = env.get("SIPPY_SEED_DATABASE_DSN") or env.get(
        "SIPPY_DATABASE_DSN", "postgresql://postgres:password@localhost:5432/postgres"
    )
    env["SIPPY_E2E_REDIS_URL"] = env.get("SIPPY_E2E_REDIS_URL") or urlsplit(
        env.get("REDIS_URL", "redis://localhost:6379")
    )._replace(path="/1").geturl()
    return env


def redis_ping(url: str) -> None:
    """Check Redis auth, database selection and PING without a redis-cli dependency."""
    parsed = urlsplit(url)
    if parsed.scheme not in ("redis", "rediss") or not parsed.hostname:
        raise ValueError("Invalid Redis URL")
    commands = []
    if parsed.password is not None:
        auth = ["AUTH"]
        if parsed.username:
            auth.append(unquote(parsed.username))
        auth.append(unquote(parsed.password))
        commands.append(auth)
    if parsed.path.strip("/"):
        commands.append(["SELECT", str(int(parsed.path.strip("/")))])
    commands.append(["PING"])
    connection = socket.create_connection((parsed.hostname, parsed.port or 6379), timeout=10)
    try:
        if parsed.scheme == "rediss":
            connection = ssl.create_default_context().wrap_socket(connection, server_hostname=parsed.hostname)
        with connection.makefile("rb") as reader:
            for command in commands:
                parts = [part.encode() for part in command]
                payload = f"*{len(parts)}\r\n".encode() + b"".join(
                    f"${len(part)}\r\n".encode() + part + b"\r\n" for part in parts
                )
                connection.sendall(payload)
                expected = b"+PONG\r\n" if command[0] == "PING" else b"+OK\r\n"
                if reader.readline(4096) != expected:
                    raise RuntimeError("Redis rejected a prerequisite command")
    finally:
        connection.close()


def check_prerequisites(target: str = "e2e") -> None:
    """Validate permissions and authenticated services in the test child context."""
    if target == "lint":
        return
    for port in ((0, 18080, 12112) if target == "e2e" else (0,)):
        try:
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", port))
                listener.listen()
        except OSError as exc:
            raise RuntimeError(
                f"Cannot open test listener on port {port} ({exc.strerror}); "
                "the MCP server needs local socket permissions and free e2e ports"
            ) from exc

    if target != "e2e":
        print("MCP test prerequisites: local listeners OK", flush=True)
        return

    # Host runs provision their own containers in e2e.sh. Check existing services
    # only in the container, where tests will use these exact inherited settings.
    if not Path("/run/.containerenv").exists():
        return
    env = os.environ.copy()
    env["PGCONNECT_TIMEOUT"] = "10"
    command = [
        "psql", env["SIPPY_DATABASE_DSN"], "-X", "-w", "-tAc",
        "SELECT rolcreatedb OR rolsuper FROM pg_roles WHERE rolname = current_user",
    ]
    try:
        result = subprocess.run(command, env=env, capture_output=True, text=True, timeout=15)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise RuntimeError("PostgreSQL prerequisite could not run; check service setup and psql availability") from exc
    if result.returncode or result.stdout.strip() != "t":
        # Client diagnostics can contain credential-bearing URLs. Do not log them.
        raise RuntimeError("PostgreSQL authentication or CREATEDB permission failed; check the MCP server's inherited service settings")
    try:
        redis_ping(env["SIPPY_E2E_REDIS_URL"])
    except (OSError, ValueError, RuntimeError) as exc:
        raise RuntimeError("Redis authentication/PING failed; check the MCP server's inherited service settings") from exc
    print("MCP test prerequisites: listeners, PostgreSQL and Redis OK", flush=True)


if __name__ == "__main__":
    try:
        check_prerequisites(sys.argv[1])
    except RuntimeError as error:
        print(f"MCP test prerequisite failed: {error}", file=sys.stderr)
        sys.exit(1)
    os.execvpe("make", ["make", *sys.argv[1:]], os.environ)
