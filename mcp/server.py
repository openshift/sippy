import os
import signal
import subprocess
import sys
import time
from pathlib import Path

from fastmcp import FastMCP

mcp = FastMCP("sippy-dev")

REPO_ROOT = Path(__file__).resolve().parent.parent
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
def regression_cache(
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

    ``bigquery_credentials_file`` should be a JSON key with BigQuery job permissions (e.g.
    ``sippy-bigquery-job-importer-key.json``). Relative paths are resolved from the repo root.
    If omitted, ``SIPPY_BIGQUERY_CREDENTIALS_FILE`` or ``GOOGLE_APPLICATION_CREDENTIALS`` must
    point to an existing file.

    Other settings default from arguments or ``SIPPY_DATABASE_DSN`` / ``REDIS_URL`` environment
    variables with sensible dev fallbacks.
    """
    creds_path, err = _resolve_bigquery_creds(bigquery_credentials_file)
    if err:
        return err

    dsn = database_dsn or _default_database_dsn()
    redis = redis_url or os.environ.get("REDIS_URL", "redis://localhost:6379")
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
    try:
        with open(log_path, "w", encoding="utf-8") as logf:
            r = subprocess.run(
                args,
                cwd=REPO_ROOT,
                env=os.environ.copy(),
                stdout=logf,
                stderr=subprocess.STDOUT,
                timeout=timeout_seconds if timeout_seconds > 0 else None,
            )
    except subprocess.TimeoutExpired:
        return (
            f"regression_cache timed out after {timeout_seconds}s. "
            f"Partial log: {log_path}\n"
            "Increase timeout_seconds or check BigQuery / network."
        )

    try:
        log_text = log_path.read_text(encoding="utf-8", errors="replace")
    except OSError as e:
        log_text = f"(could not read log file: {e})"

    tail_lines = log_text.splitlines()[-40:]
    tail = "\n".join(tail_lines)
    status = "succeeded" if r.returncode == 0 else "failed"
    return (
        f"regression_cache {status} (exit {r.returncode}). "
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


def _pids_sippy_serve() -> list[int]:
    """Processes that look like ``go run ./cmd/sippy serve`` or ``.../sippy serve`` from this repo."""
    root = REPO_ROOT.resolve()
    found: list[int] = []
    if sys.platform.startswith("linux"):
        for pid_dir in Path("/proc").iterdir():
            if not pid_dir.name.isdigit():
                continue
            try:
                if _proc_cwd(pid_dir) != root:
                    continue
                cmd = _proc_cmdline(pid_dir)
            except OSError:
                continue
            if " migrate" in cmd or " load" in cmd:
                continue
            if " serve" not in cmd and not cmd.rstrip().endswith(" serve"):
                continue
            if "cmd/sippy" in cmd or "exe/sippy" in cmd or "/sippy serve" in cmd:
                found.append(int(pid_dir.name))
        if found:
            return sorted(set(found))
    for pat in ("./cmd/sippy serve", "cmd/sippy serve", "exe/sippy serve"):
        p = _filter_pids_by_cwd(_pgrep_pids(pat), root)
        if p:
            return sorted(set(p))
    return []


def _pids_sippy_ng_dev() -> list[int]:
    """Processes running CRA dev server from ``sippy-ng`` (this repo)."""
    ng = (REPO_ROOT / "sippy-ng").resolve()
    found: list[int] = []
    if sys.platform.startswith("linux"):
        for pid_dir in Path("/proc").iterdir():
            if not pid_dir.name.isdigit():
                continue
            try:
                if _proc_cwd(pid_dir) != ng:
                    continue
                cmd = _proc_cmdline(pid_dir)
            except OSError:
                continue
            if "react-scripts" in cmd or "npm start" in cmd:
                found.append(int(pid_dir.name))
        if found:
            return sorted(set(found))
    p = _filter_pids_by_cwd(_pgrep_pids("react-scripts/scripts/start.js"), ng)
    return sorted(set(p))


@mcp.tool()
def sippy_serve(
    bigquery_credentials_file: str | None = None,
    database_dsn: str | None = None,
    redis_url: str | None = None,
    views_file: str = "config/views.yaml",
    config_file: str | None = None,
    log_file: str = "sippy-dev-logs/sippy_serve.log",
    log_level: str = "debug",
    mode: str = "ocp",
    listen: str = ":8080",
    enable_write_endpoints: bool = True,
) -> str:
    """Start the Sippy HTTP server (``go run ./cmd/sippy serve``) in the background.

    Long-running: returns after spawn with PID, log path, and listen address. Uses the same
    credential and DSN conventions as ``regression_cache``. Skips starting if a matching
    ``sippy serve`` process is already running (cwd + cmdline on Linux, ``pgrep -f`` fallback).
    """
    creds_path, err = _resolve_bigquery_creds(bigquery_credentials_file)
    if err:
        return err

    dsn = database_dsn or _default_database_dsn()
    redis = redis_url or os.environ.get("REDIS_URL", "redis://localhost:6379")
    try:
        views = _repo_path(views_file)
        log_path = _repo_path(log_file)
    except ValueError as e:
        return str(e)

    if not views.is_file():
        return f"views file not found: {views}"

    existing = _pids_sippy_serve()
    if existing:
        host_hint = f"http://127.0.0.1{listen}" if listen.startswith(":") else listen
        pids = ", ".join(str(p) for p in existing)
        return (
            f"sippy_serve already running (pid(s) {pids}). Listen: {host_hint} "
            f"log: {log_path}"
        )

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
        "--google-service-account-credential-file",
        str(creds_path),
        "--redis-url",
        redis,
        "--listen",
        listen,
    ]
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

    _ensure_dev_log_dir()
    log_path.parent.mkdir(parents=True, exist_ok=True)
    logf = open(log_path, "a", encoding="utf-8")
    try:
        proc = subprocess.Popen(
            args,
            cwd=REPO_ROOT,
            env=os.environ.copy(),
            stdout=logf,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
        )
    except OSError as e:
        logf.close()
        return f"sippy_serve failed to start: {e}"

    logf.close()
    time.sleep(0.75)
    code = proc.poll()
    if code is not None:
        try:
            tail = log_path.read_text(encoding="utf-8", errors="replace")[-4000:]
        except OSError:
            tail = "(no log output)"
        return f"sippy_serve exited immediately (exit {code}). Log: {log_path}\n--- tail ---\n{tail}"

    host_hint = f"http://127.0.0.1{listen}" if listen.startswith(":") else listen
    return f"sippy_serve started (pid {proc.pid}). Listen: {host_hint} log: {log_path}"


@mcp.tool()
def sippy_ng_start(
    log_file: str = "sippy-dev-logs/sippy_ng_start.log",
    open_browser: bool = False,
) -> str:
    """Start the React dev server (``npm start`` in ``sippy-ng``) in the background.

    CRA defaults to port 3000. ``log_file`` is resolved relative to the repo root;
    absolute paths outside the repo are rejected. Skips starting if a matching
    ``npm start`` / react-scripts process is already running for this ``sippy-ng`` tree.
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
        pids = ", ".join(str(p) for p in existing)
        return (
            f"sippy_ng_start already running (pid(s) {pids}). "
            f"Typical URL: http://127.0.0.1:3000 log: {log_path}"
        )

    env = os.environ.copy()
    if not open_browser:
        env["BROWSER"] = "none"

    _ensure_dev_log_dir()
    log_path.parent.mkdir(parents=True, exist_ok=True)
    logf = open(log_path, "a", encoding="utf-8")
    try:
        proc = subprocess.Popen(
            ["stdbuf", "-oL", "-eL", "npm", "start"],
            cwd=ng_dir,
            env=env,
            stdout=logf,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
        )
    except OSError as e:
        logf.close()
        return f"sippy_ng_start failed to start: {e}"

    logf.close()
    time.sleep(0.75)
    code = proc.poll()
    if code is not None:
        try:
            tail = log_path.read_text(encoding="utf-8", errors="replace")[-4000:]
        except OSError:
            tail = "(no log output)"
        return f"sippy_ng_start exited immediately (exit {code}). Log: {log_path}\n--- tail ---\n{tail}"

    return (
        f"sippy_ng_start started (pid {proc.pid}). Typical URL: http://127.0.0.1:3000 "
        f"log: {log_path}"
    )


def _tail_file(path: Path, max_lines: int) -> str:
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError as e:
        return f"(could not read log: {e})"
    return "\n".join(lines[-max_lines:])


def _run_make_phase(
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
    with open(log_path, "w", encoding="utf-8") as logf:
        proc = subprocess.Popen(
            ["make", make_target],
            cwd=REPO_ROOT,
            env=run_env,
            stdout=logf,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
        )
        try:
            returncode = proc.wait(timeout=tout)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
            tail = _tail_file(log_path, 80)
            return (
                f"{tool_label} timed out after {timeout_seconds}s. log: {log_path}\n"
                f"--- tail ---\n{tail}"
            )
    if returncode != 0:
        tail = _tail_file(log_path, 80)
        return (
            f"{tool_label} failed (exit {returncode}). log: {log_path}\n"
            f"--- tail ---\n{tail}"
        )
    tail = _tail_file(log_path, 40)
    return f"{tool_label} succeeded (exit 0). log: {log_path}\n--- last lines ---\n{tail}"


@mcp.tool()
def run_e2e(
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
    return _run_make_phase(
        "run_e2e",
        "e2e",
        "run_e2e.log",
        timeout_seconds,
        {"GCS_SA_JSON_PATH": str(creds_path)},
    )


if __name__ == "__main__":
    mcp.run()
