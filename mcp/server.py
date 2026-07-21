import asyncio
import os
import re
import signal
import subprocess
import sys
import urllib.error
import urllib.request
from collections.abc import Callable
from pathlib import Path

from dotenv import load_dotenv
from fastmcp import FastMCP

REPO_ROOT = Path(__file__).resolve().parent.parent
_DEVCONTAINER_ENV = REPO_ROOT / ".devcontainer" / ".env"
if _DEVCONTAINER_ENV.is_file():
    load_dotenv(_DEVCONTAINER_ENV, override=False)

mcp = FastMCP("sippy-dev")

DEV_LOG_DIR = REPO_ROOT / "sippy-dev-logs"

_MAX_TOOL_CHARS = 28000


def _ensure_dev_log_dir() -> None:
    DEV_LOG_DIR.mkdir(parents=True, exist_ok=True)


def _repo_path(p: str) -> Path:
    """Resolve *p* relative to REPO_ROOT and reject paths that escape it."""
    root = str(REPO_ROOT.resolve())
    sanitized = os.path.normpath(os.path.expanduser(p))
    if os.path.isabs(sanitized):
        resolved = Path(sanitized).resolve()
    else:
        if sanitized.startswith(".."):
            raise ValueError(f"path escapes repo root: {p}")
        resolved = Path(os.path.normpath(os.path.join(root, sanitized))).resolve()
    if not str(resolved).startswith(root + os.sep) and str(resolved) != root:
        raise ValueError(f"path escapes repo root: {resolved}")
    return resolved


def _trim(s: str, max_len: int = _MAX_TOOL_CHARS) -> str:
    if len(s) <= max_len:
        return s
    head = max_len // 2
    tail = max_len - head - 120
    omitted = len(s) - head - tail
    return f"{s[:head]}\n... [{omitted} characters omitted] ...\n{s[-tail:]}"


def _default_database_dsn() -> str:
    return os.environ.get(
        "SIPPY_DATABASE_DSN",
        "postgresql://postgres:password@localhost:5432/postgres",
    )


def _default_redis_url() -> str:
    return os.environ.get("REDIS_URL", "redis://localhost:6379")


_DSN_RE = re.compile(r"^postgresql://[^\s]+$")
_REDIS_RE = re.compile(r"^rediss?://[^\s]+$")


def _validate_dsn(dsn: str) -> str | None:
    if not _DSN_RE.match(dsn):
        return "invalid database DSN: must match postgresql://<host>/<db>"
    return None


def _validate_redis_url(url: str) -> str | None:
    if not _REDIS_RE.match(url):
        return "invalid Redis URL: must match redis:// or rediss://"
    return None


def _data_mode() -> str:
    """Return the active data mode: 'seed' or 'prod-like'.

    Uses ``SIPPY_DATA_MODE`` from the process environment, which
    ``load_dotenv(override=False)`` at startup populates from
    ``.devcontainer/.env`` when no existing env var is set.
    """
    mode = os.environ.get("SIPPY_DATA_MODE", "seed").lower()
    if mode not in ("seed", "prod-like"):
        mode = "seed"
    return mode


def _dsn_for_mode(mode: str) -> str:
    """Return the database DSN for the given mode."""
    if mode == "prod-like":
        return os.environ.get(
            "SIPPY_PRODLIKE_DATABASE_DSN",
            "postgresql://postgres:password@localhost:5432/prodlike",
        )
    return os.environ.get(
        "SIPPY_SEED_DATABASE_DSN",
        os.environ.get(
            "SIPPY_DATABASE_DSN",
            "postgresql://postgres:password@localhost:5432/postgres",
        ),
    )


def _resolve_bigquery_creds(explicit: str | None) -> tuple[Path | None, str | None]:
    if explicit:
        p = Path(explicit).expanduser().resolve()
        if not p.is_file():
            return None, f"BigQuery credentials file not found: {p}"
        return p, None
    for key in ("SIPPY_BIGQUERY_CREDENTIALS_FILE", "GOOGLE_APPLICATION_CREDENTIALS"):
        v = os.environ.get(key)
        if v:
            p = Path(v).expanduser().resolve()
            if p.is_file():
                return p, None
            return None, f"{key} is set but file not found: {p}"
    return (
        None,
        "Set bigquery_credentials_file to your GCP service account JSON path "
        "(e.g. sippy-bigquery-job-importer-key.json), or set SIPPY_BIGQUERY_CREDENTIALS_FILE "
        "or GOOGLE_APPLICATION_CREDENTIALS.",
    )


@mcp.tool()
async def regression_cache(
    bigquery_credentials_file: str | None = None,
    database_dsn: str | None = None,
    redis_url: str | None = None,
    views_file: str = "config/views.yaml",
    config_file: str = "config/openshift.yaml",
    log_file: str = "sippy-dev-logs/regression_cache.log",
    skip_matview_refresh: bool = True,
    timeout_seconds: int = 7200,
) -> str:
    """Run the regression-cache loader (BigQuery + Redis + DB).

    Equivalent to a line-buffered ``go run ./cmd/sippy load --loader regression-cache`` with
    logging merged to ``log_file``. Typical duration is many minutes.

    Always targets the prod-like database (``prodlike``) by default, regardless of
    ``SIPPY_DATA_MODE``. Pass ``database_dsn`` explicitly to override.

    ``bigquery_credentials_file`` should be a JSON key with BigQuery job permissions (e.g.
    ``sippy-bigquery-job-importer-key.json``). Relative paths are resolved from the repo root.
    If omitted, ``SIPPY_BIGQUERY_CREDENTIALS_FILE`` or ``GOOGLE_APPLICATION_CREDENTIALS`` must
    point to an existing file.

    Other settings default from arguments or ``SIPPY_PRODLIKE_DATABASE_DSN`` / ``REDIS_URL``
    environment variables with sensible dev fallbacks.
    """
    creds_path, err = _resolve_bigquery_creds(bigquery_credentials_file)
    if err:
        return err

    dsn = database_dsn or _dsn_for_mode("prod-like")
    redis = redis_url or _default_redis_url()
    for check in (_validate_dsn(dsn), _validate_redis_url(redis)):
        if check:
            return check
    try:
        views = _repo_path(views_file)
        config = _repo_path(config_file)
        log_path = _repo_path(log_file)
    except ValueError as e:
        return str(e)

    for label, p in ("views", views), ("config", config):
        if not p.is_file():
            return f"{label} file not found: {p}"

    args = [
        "stdbuf",
        "-oL",
        "-eL",
        "go",
        "run",
        "./cmd/sippy",
        "load",
        "--loader",
        "regression-cache",
        "--views",
        str(views),
        "--database-dsn",
        dsn,
        "--redis-url",
        redis,
        "--config",
        str(config),
        "--google-service-account-credential-file",
        str(creds_path),
    ]
    if skip_matview_refresh:
        args.append("--skip-matview-refresh")

    _ensure_dev_log_dir()
    log_path.parent.mkdir(parents=True, exist_ok=True)
    logf = open(log_path, "w", encoding="utf-8")
    try:
        proc = await asyncio.create_subprocess_exec(
            *args,
            cwd=REPO_ROOT,
            env=os.environ.copy(),
            stdout=logf,
            stderr=asyncio.subprocess.STDOUT,
        )
        tout = timeout_seconds if timeout_seconds > 0 else None
        try:
            returncode = await asyncio.wait_for(proc.wait(), timeout=tout)
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()
            return (
                f"regression_cache timed out after {timeout_seconds}s. "
                f"Partial log: {log_path}\n"
                "Increase timeout_seconds or check BigQuery / network."
            )
    finally:
        logf.close()

    try:
        log_text = log_path.read_text(encoding="utf-8", errors="replace")
    except OSError as e:
        log_text = f"(could not read log file: {e})"

    tail_lines = log_text.splitlines()[-40:]
    tail = "\n".join(tail_lines)
    status = "succeeded" if returncode == 0 else "failed"
    return (
        f"regression_cache {status} (exit {returncode}). "
        f"Full log: {log_path}\n--- last {len(tail_lines)} lines ---\n{tail}"
    )


def _proc_cmdline(pid_dir: Path) -> str:
    raw = (pid_dir / "cmdline").read_bytes()
    return raw.replace(b"\0", b" ").decode(errors="replace")


def _proc_cwd(pid_dir: Path) -> Path | None:
    try:
        return (pid_dir / "cwd").resolve()
    except OSError:
        return None


def _pgrep_pids(pattern: str) -> list[int]:
    try:
        r = subprocess.run(
            ["pgrep", "-f", pattern],
            capture_output=True,
            text=True,
            timeout=5,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return []
    if r.returncode != 0:
        return []
    return [int(x) for x in r.stdout.split() if x.strip().isdigit()]


def _filter_pids_by_cwd(pids: list[int], expected_cwd: Path) -> list[int]:
    """Filter PIDs to those whose working directory matches expected_cwd (macOS/lsof fallback)."""
    if not pids:
        return []
    filtered: list[int] = []
    for pid in pids:
        try:
            r = subprocess.run(
                ["lsof", "-a", "-p", str(pid), "-d", "cwd", "-Fn"],
                capture_output=True,
                text=True,
                timeout=5,
            )
        except (FileNotFoundError, subprocess.TimeoutExpired):
            return pids
        for line in r.stdout.splitlines():
            if line.startswith("n") and Path(line[1:]).resolve() == expected_cwd:
                filtered.append(pid)
                break
    return filtered


def _find_pids(
    expected_cwd: Path,
    cmdline_match: Callable[[str], bool],
    pgrep_patterns: list[str],
) -> list[int]:
    """Find PIDs with a given cwd whose cmdline passes *cmdline_match*.

    On Linux, scans /proc directly. Falls back to pgrep + lsof filtering.
    """
    found: list[int] = []
    if sys.platform.startswith("linux"):
        for pid_dir in Path("/proc").iterdir():
            if not pid_dir.name.isdigit():
                continue
            try:
                if _proc_cwd(pid_dir) != expected_cwd:
                    continue
                cmd = _proc_cmdline(pid_dir)
            except OSError:
                continue
            if cmdline_match(cmd):
                found.append(int(pid_dir.name))
        if found:
            return sorted(set(found))
    for pat in pgrep_patterns:
        p = _filter_pids_by_cwd(_pgrep_pids(pat), expected_cwd)
        if p:
            return sorted(set(p))
    return []


def _pids_sippy_serve() -> list[int]:
    def _match(cmd: str) -> bool:
        if " migrate" in cmd or " load" in cmd:
            return False
        if " serve" not in cmd and not cmd.rstrip().endswith(" serve"):
            return False
        return "cmd/sippy" in cmd or "exe/sippy" in cmd or "/sippy serve" in cmd

    return _find_pids(
        REPO_ROOT.resolve(),
        _match,
        ["./cmd/sippy serve", "cmd/sippy serve", "exe/sippy serve"],
    )


def _pids_sippy_ng_dev() -> list[int]:
    def _match(cmd: str) -> bool:
        return "react-scripts" in cmd or "npm start" in cmd

    return _find_pids(
        (REPO_ROOT / "sippy-ng").resolve(),
        _match,
        ["react-scripts/scripts/start.js"],
    )


async def _stop_pids(pids: list[int]) -> str:
    """Send SIGTERM then SIGKILL to each PID. Returns a summary."""
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    await asyncio.sleep(1)
    killed = []
    for pid in pids:
        try:
            os.kill(pid, 0)
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        killed.append(pid)
    return ", ".join(str(p) for p in killed)


@mcp.tool()
async def sippy_serve(
    bigquery_credentials_file: str | None = None,
    database_dsn: str | None = None,
    redis_url: str | None = None,
    views_file: str | None = None,
    config_file: str | None = None,
    log_file: str = "sippy-dev-logs/sippy_serve.log",
    log_level: str = "debug",
    mode: str = "ocp",
    listen: str = ":8080",
    enable_write_endpoints: bool = True,
    data_provider: str | None = None,
    restart: bool = False,
) -> str:
    """Start the Sippy HTTP server (``go run ./cmd/sippy serve``) in the background.

    Long-running: returns after spawn with PID, log path, and listen address. Skips starting
    if a matching ``sippy serve`` process is already running, unless ``restart`` is True.

    Always verifies HTTP readiness by polling the listen address before reporting ready,
    even when a process is already running.

    Defaults are derived from ``SIPPY_DATA_MODE`` (``seed`` or ``prod-like``):

    - **seed** (default): ``data_provider=postgres``, ``views_file=config/seed-views.yaml``,
      DSN points to the seed database. No BigQuery credentials needed.
    - **prod-like**: ``data_provider=bigquery``, ``views_file=config/views.yaml``,
      DSN points to the prod-like database. Requires BigQuery credentials.

    Explicit parameter values always override mode-derived defaults.
    """
    data_mode = _data_mode()

    if data_provider is None:
        data_provider = "bigquery" if data_mode == "prod-like" else "postgres"
    if views_file is None:
        views_file = "config/views.yaml" if data_mode == "prod-like" else "config/seed-views.yaml"
    if database_dsn is None:
        database_dsn = _dsn_for_mode(data_mode)

    dsn = database_dsn
    redis = redis_url or _default_redis_url()
    for check in (_validate_dsn(dsn), _validate_redis_url(redis)):
        if check:
            return check
    try:
        views = _repo_path(views_file)
        log_path = _repo_path(log_file)
    except ValueError as e:
        return str(e)

    if not views.is_file():
        return f"views file not found: {views}"

    existing = _pids_sippy_serve()
    if existing:
        if not restart:
            host_hint = f"http://127.0.0.1{listen}" if listen.startswith(":") else listen
            pids = ", ".join(str(p) for p in existing)
            err = await _wait_for_url(host_hint, 120, existing)
            if err:
                return (
                    f"sippy_serve already running (pid(s) {pids}) but {err}. Listen: {host_hint} "
                    f"log: {log_path}. Call with restart=True to restart."
                )
            return (
                f"sippy_serve already running and ready (pid(s) {pids}). Listen: {host_hint} "
                f"log: {log_path}. Call with restart=True to restart."
            )
        await _stop_pids(existing)

    args = [
        "stdbuf",
        "-oL",
        "-eL",
        "go",
        "run",
        "./cmd/sippy",
        "serve",
        "--views",
        str(views),
        "--log-level",
        log_level,
        "--database-dsn",
        dsn,
        "--mode",
        mode,
        "--redis-url",
        redis,
        "--listen",
        listen,
        "--data-provider",
        data_provider,
    ]

    creds_path, creds_err = _resolve_bigquery_creds(bigquery_credentials_file)
    if creds_path:
        args.extend(["--google-service-account-credential-file", str(creds_path)])
    elif data_provider == "bigquery":
        return f"BigQuery credentials required for data_provider=bigquery: {creds_err}"

    if enable_write_endpoints:
        args.append("--enable-write-endpoints")
    if config_file:
        try:
            cfg = _repo_path(config_file)
        except ValueError as e:
            return str(e)
        if not cfg.is_file():
            return f"config file not found: {cfg}"
        args.extend(["--config", str(cfg)])

    host_hint = f"http://127.0.0.1{listen}" if listen.startswith(":") else listen
    pid_or_err = await _spawn_background(
        label="sippy_serve", args=args, cwd=REPO_ROOT, log_path=log_path,
        ready_url=host_hint,
    )
    if isinstance(pid_or_err, str):
        return pid_or_err
    return f"sippy_serve started and ready (pid {pid_or_err}). Listen: {host_hint} log: {log_path}"


@mcp.tool()
async def sippy_stop() -> str:
    """Stop running sippy_serve and sippy_ng_start processes."""
    results = []
    serve_pids = _pids_sippy_serve()
    if serve_pids:
        await _stop_pids(serve_pids)
        results.append(f"Stopped sippy_serve (pid(s) {', '.join(str(p) for p in serve_pids)})")
    ng_pids = _pids_sippy_ng_dev()
    if ng_pids:
        await _stop_pids(ng_pids)
        results.append(f"Stopped sippy_ng (pid(s) {', '.join(str(p) for p in ng_pids)})")
    if not results:
        return "No running sippy processes found."
    return ". ".join(results) + "."


@mcp.tool()
async def sippy_ng_start(
    log_file: str = "sippy-dev-logs/sippy_ng_start.log",
    open_browser: bool = False,
    restart: bool = False,
) -> str:
    """Start the React dev server (``npm start`` in ``sippy-ng``) in the background.

    CRA defaults to port 3000. ``log_file`` is resolved relative to the repo root;
    absolute paths outside the repo are rejected. Skips starting if a matching
    ``npm start`` / react-scripts process is already running, unless ``restart`` is True.

    Always verifies HTTP readiness by polling the listen address before reporting ready,
    even when a process is already running.
    """
    ng_dir = REPO_ROOT / "sippy-ng"
    if not (ng_dir / "package.json").is_file():
        return f"sippy-ng not found or missing package.json: {ng_dir}"

    try:
        log_path = _repo_path(log_file)
    except ValueError as e:
        return str(e)

    existing = _pids_sippy_ng_dev()
    if existing:
        if not restart:
            pids = ", ".join(str(p) for p in existing)
            err = await _wait_for_url("http://127.0.0.1:3000/sippy-ng/", 120, existing)
            if err:
                return (
                    f"sippy_ng_start already running (pid(s) {pids}) but {err}. "
                    f"Typical URL: http://127.0.0.1:3000/sippy-ng/ log: {log_path}. "
                    f"Call with restart=True to restart."
                )
            return (
                f"sippy_ng_start already running and ready (pid(s) {pids}). "
                f"Typical URL: http://127.0.0.1:3000/sippy-ng/ log: {log_path}. "
                f"Call with restart=True to restart."
            )
        await _stop_pids(existing)

    env = os.environ.copy()
    if not open_browser:
        env["BROWSER"] = "none"

    pid_or_err = await _spawn_background(
        label="sippy_ng_start",
        args=["stdbuf", "-oL", "-eL", "npm", "start"],
        cwd=ng_dir,
        log_path=log_path,
        env=env,
        ready_url="http://127.0.0.1:3000/sippy-ng/",
    )
    if isinstance(pid_or_err, str):
        return pid_or_err
    return (
        f"sippy_ng_start started and ready (pid {pid_or_err}). URL: http://127.0.0.1:3000/sippy-ng/ "
        f"log: {log_path}"
    )


async def _wait_for_ready(url: str, timeout: int, proc: subprocess.Popen) -> str | None:
    """Poll *url* until it responds or *timeout* seconds elapse. Returns an error string or None."""
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        code = proc.poll()
        if code is not None:
            return f"process exited (exit {code}) while waiting for readiness"
        try:
            await asyncio.to_thread(urllib.request.urlopen, url, None, 2)
            return None
        except (urllib.error.URLError, OSError, TimeoutError):
            await asyncio.sleep(1)
    return f"not ready after {timeout}s (checked {url})"


def _pid_alive(pid: int) -> bool:
    """Return True if *pid* is still running."""
    try:
        os.kill(pid, 0)  # signal 0: no signal sent, just checks process exists
        return True
    except OSError:
        return False


async def _wait_for_url(url: str, timeout: int, pids: list[int]) -> str | None:
    """Poll *url* until it responds or *timeout* seconds elapse.

    Like ``_wait_for_ready`` but works with bare PIDs instead of a ``Popen`` object.
    Returns an error string or ``None`` on success.
    """
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        if not any(_pid_alive(p) for p in pids):
            return "process(es) exited while waiting for readiness"
        try:
            await asyncio.to_thread(urllib.request.urlopen, url, None, 2)
            return None
        except (urllib.error.URLError, OSError, TimeoutError):
            await asyncio.sleep(1)
    return f"not ready after {timeout}s (checked {url})"


async def _spawn_background(
    label: str,
    args: list[str],
    cwd: Path,
    log_path: Path,
    env: dict[str, str] | None = None,
    ready_url: str | None = None,
    ready_timeout: int = 120,
) -> int | str:
    """Spawn a detached process, returning its PID or an error string.

    If *ready_url* is set, polls it until it responds (up to *ready_timeout* seconds).
    """
    _ensure_dev_log_dir()
    log_path.parent.mkdir(parents=True, exist_ok=True)
    logf = open(log_path, "a", encoding="utf-8")
    try:
        proc = subprocess.Popen(
            args,
            cwd=cwd,
            env=env or os.environ.copy(),
            stdout=logf,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
        )
    except OSError as e:
        logf.close()
        return f"{label} failed to start: {e}"

    logf.close()
    await asyncio.sleep(0.75)
    code = proc.poll()
    if code is not None:
        try:
            tail = log_path.read_text(encoding="utf-8", errors="replace")[-4000:]
        except OSError:
            tail = "(no log output)"
        return f"{label} exited immediately (exit {code}). Log: {log_path}\n--- tail ---\n{tail}"

    if ready_url:
        err = await _wait_for_ready(ready_url, ready_timeout, proc)
        if err:
            tail = _tail_file(log_path, 40)
            return f"{label} started (pid {proc.pid}) but {err}. Log: {log_path}\n--- tail ---\n{tail}"

    return proc.pid


def _tail_file(path: Path, max_lines: int) -> str:
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError as e:
        return f"(could not read log: {e})"
    return "\n".join(lines[-max_lines:])


async def _run_make_phase(
    tool_label: str,
    make_target: str,
    log_filename: str,
    timeout_seconds: int,
    env_extra: dict[str, str] | None = None,
) -> str:
    """Run ``make <target>``; log to ``sippy-dev-logs/<log_filename>``."""
    _ensure_dev_log_dir()
    log_path = DEV_LOG_DIR / log_filename
    run_env = os.environ.copy()
    if env_extra:
        run_env.update(env_extra)
    tout = None if timeout_seconds <= 0 else timeout_seconds
    logf = open(log_path, "w", encoding="utf-8")
    try:
        proc = await asyncio.create_subprocess_exec(
            "make", make_target,
            cwd=REPO_ROOT,
            env=run_env,
            stdout=logf,
            stderr=asyncio.subprocess.STDOUT,
            stdin=asyncio.subprocess.DEVNULL,
        )
        try:
            returncode = await asyncio.wait_for(proc.wait(), timeout=tout)
        except asyncio.TimeoutError:
            proc.terminate()
            try:
                await asyncio.wait_for(proc.wait(), timeout=5)
            except asyncio.TimeoutError:
                proc.kill()
                await proc.wait()
            tail = _tail_file(log_path, 80)
            return (
                f"{tool_label} timed out after {timeout_seconds}s. log: {log_path}\n"
                f"--- tail ---\n{tail}"
            )
    finally:
        logf.close()
    if returncode != 0:
        tail = _tail_file(log_path, 80)
        return (
            f"{tool_label} failed (exit {returncode}). log: {log_path}\n"
            f"--- tail ---\n{tail}"
        )
    tail = _tail_file(log_path, 40)
    return f"{tool_label} succeeded (exit 0). log: {log_path}\n--- last lines ---\n{tail}"


@mcp.tool()
async def restore_prodlike_db(
    backup_file: str,
    timeout_seconds: int = 14400,
) -> str:
    """Drop and recreate the ``prodlike`` database from a backup under the repo.

    ``backup_file`` is resolved with the same path rules as other MCP tools (repo-relative,
    no ``..``). Supports custom-format ``.dump`` (``pg_restore``) or ``.sql`` (``psql -f``).

    Uses ``SIPPY_PRODLIKE_DATABASE_DSN`` (must end with ``/prodlike``; host must be
    ``localhost`` or ``sippy-postgres``). Stop ``sippy serve`` (and anything else using
    ``prodlike``) first. Large restores: set ``timeout_seconds=0`` for no limit.

    Log: ``sippy-dev-logs/restore_prodlike_db.log``. After success, run
    ``go run ./cmd/sippy migrate`` against ``SIPPY_PRODLIKE_DATABASE_DSN`` if the schema
    may trail the dump.
    """
    try:
        backup_p = _repo_path(backup_file)
    except ValueError as e:
        return str(e)
    if not backup_p.is_file():
        return f"backup file not found: {backup_p}"

    script = REPO_ROOT / "scripts" / "restore_prodlike_db.sh"
    if not script.is_file():
        return f"restore script missing: {script}"

    rel = str(backup_p.relative_to(REPO_ROOT.resolve()))
    _ensure_dev_log_dir()
    log_path = DEV_LOG_DIR / "restore_prodlike_db.log"
    logf = open(log_path, "w", encoding="utf-8")
    try:
        proc = await asyncio.create_subprocess_exec(
            "bash",
            str(script),
            rel,
            cwd=str(REPO_ROOT),
            env=os.environ.copy(),
            stdout=logf,
            stderr=asyncio.subprocess.STDOUT,
            stdin=asyncio.subprocess.DEVNULL,
        )
        tout = None if timeout_seconds <= 0 else float(timeout_seconds)
        try:
            returncode = await asyncio.wait_for(proc.wait(), timeout=tout)
        except asyncio.TimeoutError:
            proc.terminate()
            try:
                await asyncio.wait_for(proc.wait(), timeout=15)
            except asyncio.TimeoutError:
                proc.kill()
                await proc.wait()
            tail = _tail_file(log_path, 60)
            return (
                f"restore_prodlike_db timed out after {timeout_seconds}s. log: {log_path}\n"
                f"--- tail ---\n{tail}"
            )
    finally:
        logf.close()

    tail = _tail_file(log_path, 60)
    status = "succeeded" if returncode == 0 else "failed"
    return (
        f"restore_prodlike_db {status} (exit {returncode}). log: {log_path}\n"
        f"--- tail ---\n{tail}"
    )


@mcp.tool()
async def run_e2e(
    bigquery_credentials_file: str | None = None,
    timeout_seconds: int = 7200,
) -> str:
    """Run ``make e2e`` (sets ``GCS_SA_JSON_PATH`` from the same SA JSON as other MCP tools).

    Log: ``sippy-dev-logs/run_e2e.log``. E2e is slow and uses BigQuery; use ``timeout_seconds=0``
    for no limit.
    """
    creds_path, err = _resolve_bigquery_creds(bigquery_credentials_file)
    if err:
        return err
    return await _run_make_phase(
        "run_e2e",
        "e2e",
        "run_e2e.log",
        timeout_seconds,
        {"GCS_SA_JSON_PATH": str(creds_path)},
    )


@mcp.tool()
async def check_services() -> str:
    """Check that required services are healthy and data is loaded.

    Validates postgres connectivity, redis connectivity, sippy binary
    existence, and seed data presence. Useful after initial setup or
    to diagnose issues during development.
    """
    results: list[str] = []

    dsn = os.environ.get("SIPPY_DATABASE_DSN", _default_database_dsn())
    redis = os.environ.get("REDIS_URL", _default_redis_url())

    # Check postgres
    try:
        proc = await asyncio.create_subprocess_exec(
            "psql", dsn, "-c", "SELECT 1",
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        _, stderr = await asyncio.wait_for(proc.communicate(), timeout=10)
        if proc.returncode == 0:
            results.append("postgres: OK")
        else:
            results.append(f"postgres: FAILED ({stderr.decode().strip()})")
    except Exception as e:
        results.append(f"postgres: FAILED ({e})")

    # Check redis
    redis_host = redis.replace("redis://", "").replace("rediss://", "").split("/")[0]
    host, _, port = redis_host.partition(":")
    try:
        proc = await asyncio.create_subprocess_exec(
            "redis-cli", "-h", host or "localhost", "-p", port or "6379", "PING",
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=10)
        if b"PONG" in stdout:
            results.append("redis: OK")
        else:
            results.append(f"redis: FAILED (no PONG, got: {stdout.decode().strip()})")
    except Exception as e:
        results.append(f"redis: FAILED ({e})")

    # Check sippy binary
    sippy_bin = REPO_ROOT / "sippy"
    if sippy_bin.is_file():
        results.append("sippy binary: OK")
    else:
        results.append("sippy binary: NOT FOUND (run 'make sippy')")

    # Check seed data
    try:
        proc = await asyncio.create_subprocess_exec(
            "psql", dsn, "-t", "-c", "SELECT count(*) FROM prow_job_runs",
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=10)
        if proc.returncode != 0:
            hint = stderr.decode().strip()[:200] if stderr else "unknown error"
            results.append(f"seed data: FAILED (psql exit {proc.returncode}: {hint})")
        else:
            count = stdout.decode().strip()
            if int(count) > 0:
                results.append(f"seed data: OK ({count} job runs)")
            else:
                results.append("seed data: EMPTY (run './sippy seed-data --init-database')")
    except Exception as e:
        results.append(f"seed data: FAILED ({e})")

    status = "READY" if all("OK" in r for r in results) else "NOT READY"
    return f"Environment: {status}\n" + "\n".join(f"  {r}" for r in results)


if __name__ == "__main__":
    mcp.run()
