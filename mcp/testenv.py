"""Prepare and validate the environment used by MCP test subprocesses."""

import os
import socket
import ssl
import subprocess
import sys
import tempfile
from pathlib import Path
from urllib.parse import unquote, urlsplit

from serviceurl import local_database_uri


def default_database_dsn(parent: dict[str, str]) -> str:
    """Resolve the legacy database setting shared by MCP tools and test children."""
    return parent.get("SIPPY_DATABASE_DSN", "postgresql://postgres:password@localhost:5432/postgres")


def seed_database_dsn(parent: dict[str, str]) -> str:
    """Select the seed database with the same precedence in tools and test children."""
    return parent.get("SIPPY_SEED_DATABASE_DSN") or default_database_dsn(parent)


class RedisPrerequisiteError(RuntimeError):
    """Carry a credential-free Redis operation diagnostic to the test child."""


class RedisCommandError(RedisPrerequisiteError):
    """Identify a rejected prerequisite operation without retaining Redis's reply."""


def postgres_environment(parent: dict[str, str]) -> dict[str, str]:
    """Move prerequisite connection parameters into libpq's environment variables."""
    env = parent.copy()
    parsed = urlsplit(env["SIPPY_DATABASE_DSN"])
    if parsed.scheme not in ("postgres", "postgresql"):
        raise ValueError("Invalid PostgreSQL URI")
    authority = parsed.netloc.rsplit("@", 1)[-1]
    hosts = [urlsplit("//" + host) for host in authority.split(",")]
    parameters = {
        "host": ",".join(unquote(host.hostname or "") for host in hosts),
        "port": ",".join(str(host.port or "") for host in hosts),
        "dbname": unquote(parsed.path.removeprefix("/")),
    }
    if parsed.username is not None:
        parameters["user"] = unquote(parsed.username)
    if parsed.password is not None:
        parameters["password"] = unquote(parsed.password)
    for item in filter(None, parsed.query.split("&")):
        key, separator, value = item.partition("=")
        if not separator:
            raise ValueError("Invalid PostgreSQL URI option")
        parameters[unquote(key)] = unquote(value)
    names = {
        "dbname": "PGDATABASE", "user": "PGUSER",
        "application_name": "PGAPPNAME", "client_encoding": "PGCLIENTENCODING",
        "connect_timeout": "PGCONNECT_TIMEOUT",
    }
    for key in (
        "host", "hostaddr", "port", "password", "passfile", "service", "options", "sslmode",
        "requiressl", "sslcompression", "sslcert", "sslkey", "sslrootcert", "sslcrl",
        "sslcrldir", "sslsni", "sslcertmode", "sslnegotiation", "requirepeer",
        "ssl_min_protocol_version", "ssl_max_protocol_version", "gssencmode",
        "krbsrvname", "gsslib", "gssdelegation", "require_auth", "channel_binding",
        "target_session_attrs", "load_balance_hosts", "min_protocol_version", "max_protocol_version",
    ):
        names[key] = "PG" + key.replace("_", "").upper()
    for key, value in parameters.items():
        if key not in names:
            raise ValueError("PostgreSQL prerequisite option has no libpq environment equivalent")
        if value or key == "password":
            env[names[key]] = value
    env["PGCONNECT_TIMEOUT"] = "10"
    return env


def writable_directory(path: Path) -> Path:
    """Probe real write access rather than relying on ownership or access bits."""
    path = path.resolve()
    path.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryFile(dir=path):
        pass
    return path


def prepare_environment(parent: dict[str, str], log_dir: Path, target: str = "e2e") -> dict[str, str]:
    """Keep usable caches and service credentials, with private writable defaults."""
    validate_target(target)
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

    if target == "lint":
        return env

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
    if target == "test":
        return env
    env["SIPPY_TEST_OUTPUT_DIR"] = str(writable_directory(artifacts / "e2e"))
    env["SIPPY_DATABASE_DSN"] = seed_database_dsn(env)
    if not env.get("SIPPY_E2E_REDIS_URL"):
        try:
            env["SIPPY_E2E_REDIS_URL"] = urlsplit(
                env.get("REDIS_URL", "redis://localhost:6379")
            )._replace(path="/1", query="").geturl()
        except ValueError:
            raise ValueError("REDIS_URL has a malformed URI authority") from None
    return env


def redis_ping(url: str) -> None:
    """Check Redis auth, database selection and PING without a redis-cli dependency."""
    try:
        parsed = urlsplit(url)
        port = parsed.port or 6379
        database = str(int(parsed.path.strip("/"))) if parsed.path.strip("/") else None
    except ValueError:
        raise ValueError("SIPPY_E2E_REDIS_URL has a malformed authority, port or database") from None
    if parsed.scheme not in ("redis", "rediss") or not parsed.hostname:
        raise ValueError("SIPPY_E2E_REDIS_URL requires a redis/rediss scheme and hostname")
    if parsed.query:
        raise ValueError("Redis URL query options are not supported by Sippy")
    commands = []
    if parsed.password is not None:
        # Sippy's redis.v5 client ignores usernames and uses password-only AUTH.
        commands.append(["AUTH", unquote(parsed.password)])
    if database is not None:
        commands.append(["SELECT", database])
    commands.append(["PING"])
    try:
        connection = socket.create_connection((parsed.hostname, port), timeout=10)
    except TimeoutError:
        raise RedisPrerequisiteError("Redis connection timed out; check service reachability") from None
    except OSError:
        raise RedisPrerequisiteError("Redis connection failed; check service address and availability") from None
    try:
        if parsed.scheme == "rediss":
            try:
                connection = ssl.create_default_context().wrap_socket(connection, server_hostname=parsed.hostname)
            except OSError:
                raise RedisPrerequisiteError("Redis TLS handshake failed; check TLS configuration") from None
        with connection.makefile("rb") as reader:
            for command in commands:
                parts = [part.encode() for part in command]
                payload = f"*{len(parts)}\r\n".encode() + b"".join(
                    f"${len(part)}\r\n".encode() + part + b"\r\n" for part in parts
                )
                try:
                    connection.sendall(payload)
                    reply = reader.readline(4096)
                except TimeoutError:
                    raise RedisPrerequisiteError(f"Redis prerequisite {command[0]} timed out") from None
                except OSError:
                    raise RedisPrerequisiteError(f"Redis prerequisite {command[0]} transport failed") from None
                expected = b"+PONG\r\n" if command[0] == "PING" else b"+OK\r\n"
                if reply != expected:
                    raise RedisCommandError(f"Redis rejected prerequisite {command[0]}")
    finally:
        connection.close()


def check_frontend_dependencies(repo_root: Path) -> None:
    """Diagnose reused Node modules with native binaries for a different platform."""
    frontend = repo_root / "sippy-ng"
    if not (frontend / "node_modules").is_dir():
        # make installs dependencies when they are absent.
        return
    script = """
try {
    require('rollup');
    require('esbuild').transformSync('let probe = 1');
} catch {
    process.exit(1);
}
"""
    try:
        result = subprocess.run(
            ["node", "-e", script], cwd=frontend,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30,
        )
    except FileNotFoundError:
        raise RuntimeError("Node executable not found; install Node or fix PATH") from None
    except (OSError, subprocess.TimeoutExpired):
        raise RuntimeError("Node native dependency check could not complete; check Node installation and permissions") from None
    if result.returncode != 0:
        raise RuntimeError(
            "Rollup/esbuild native dependencies cannot load on this Node platform. "
            "node_modules may come from another OS/architecture. "
            "Reinstall in this environment with: cd sippy-ng && npm ci --no-audit --ignore-scripts"
        )


def validate_target(target: str) -> None:
    """Reject unknown make targets before preparing settings or opening listeners."""
    if target not in ("lint", "test", "e2e"):
        raise ValueError("Unsupported test target; expected lint, test or e2e")


def check_prerequisites(target: str = "e2e") -> None:
    """Validate permissions and authenticated services in the test child context."""
    validate_target(target)
    if target == "lint":
        return
    in_container = Path("/run/.containerenv").exists()
    ports = (0, 18080, 12112) if target == "e2e" else (0,)
    if target == "e2e" and not in_container:
        ports += (23433, 23479)
    for port in ports:
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
    if not in_container:
        return
    command = [
        "psql", "-X", "-w", "-tAc",
        "SELECT rolcreatedb OR rolsuper FROM pg_roles WHERE rolname = current_user",
    ]
    try:
        env = os.environ.copy()
        env["SIPPY_DATABASE_DSN"] = local_database_uri(env["SIPPY_DATABASE_DSN"], strip_overrides=True)
        env.pop("PGHOSTADDR", None)
        env.pop("PGSERVICE", None)
        env = postgres_environment(env)
        host_count = max(len(env.get(key, "").split(",")) for key in ("PGHOST", "PGHOSTADDR"))
        # libpq applies connect_timeout separately to each failover host.
        timeout = int(env["PGCONNECT_TIMEOUT"]) * host_count + 5
    except ValueError as exc:
        raise RuntimeError(f"PostgreSQL SIPPY_DATABASE_DSN validation failed: {exc}") from None
    try:
        result = subprocess.run(command, env=env, capture_output=True, text=True, timeout=timeout)
    except FileNotFoundError:
        raise RuntimeError("PostgreSQL psql executable not found; install psql or fix PATH") from None
    except subprocess.TimeoutExpired:
        raise RuntimeError("PostgreSQL permission query timed out; check connectivity and server responsiveness") from None
    except OSError:
        raise RuntimeError("PostgreSQL psql startup failed; check executable permissions") from None
    # Client diagnostics can contain credential-bearing URLs. Do not log them.
    if result.returncode == 2:
        raise RuntimeError("PostgreSQL connection/authentication failed; check service availability and inherited credentials")
    if result.returncode:
        raise RuntimeError("PostgreSQL permission query failed; check server configuration and role access")
    if result.stdout.strip() != "t":
        raise RuntimeError("PostgreSQL CREATEDB permission denied; use a role allowed to create the e2e database")
    try:
        redis_ping(env["SIPPY_E2E_REDIS_URL"])
    except RedisPrerequisiteError as exc:
        raise RuntimeError(f"{exc}; check the MCP server's inherited service settings") from exc
    except ValueError as exc:
        raise RuntimeError(f"Redis URL validation failed: {exc}") from None
    except (OSError, RuntimeError):
        raise RuntimeError("Redis prerequisite failed unexpectedly; check service setup") from None
    print("MCP test prerequisites: listeners, PostgreSQL and Redis OK", flush=True)


if __name__ == "__main__":
    try:
        if sys.argv[1] == "test":
            check_frontend_dependencies(Path(__file__).resolve().parent.parent)
        check_prerequisites(sys.argv[1])
    except (RuntimeError, ValueError) as error:
        print(f"MCP test prerequisite failed: {error}", file=sys.stderr)
        sys.exit(1)
    os.execvpe("make", ["make", *sys.argv[1:]], os.environ)
