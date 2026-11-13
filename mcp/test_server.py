import asyncio
import fcntl
import inspect
import os
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path
from unittest import mock

import pytest
from fastmcp import Client
from fastmcp.client.transports import StdioTransport

import server
import serviceurl
import testenv
from server import (
    REPO_ROOT,
    _data_mode,
    _default_database_dsn,
    _default_redis_url,
    _dsn_for_mode,
    _pid_alive,
    _repo_path,
    _resolve_bigquery_creds,
    _validate_dsn,
    _validate_prodlike_backup,
    _validate_redis_url,
    _trim,
    _wait_for_url,
    mcp,
    run_e2e,
)

E2E_SCRIPT = REPO_ROOT / "scripts" / "e2e.sh"


class FakeProcess:
    """Stand-in for an asyncio subprocess that exits successfully at once."""

    returncode = 0

    async def wait(self) -> int:
        return 0

    async def terminate(self) -> None:
        return None

    async def kill(self) -> None:
        return None


class TestRepoPath:
    def test_relative_path(self):
        result = _repo_path("config/views.yaml")
        assert result == (REPO_ROOT / "config" / "views.yaml").resolve()

    def test_nested_relative_path(self):
        result = _repo_path("a/b/c")
        assert result == (REPO_ROOT / "a" / "b" / "c").resolve()

    def test_dot_relative(self):
        result = _repo_path("./config/views.yaml")
        assert result == (REPO_ROOT / "config" / "views.yaml").resolve()

    def test_rejects_dotdot_escape(self):
        with pytest.raises(ValueError, match="path escapes repo root"):
            _repo_path("../etc/passwd")

    def test_rejects_nested_dotdot_escape(self):
        with pytest.raises(ValueError, match="path escapes repo root"):
            _repo_path("foo/../../..")

    def test_rejects_absolute_outside_repo(self):
        with pytest.raises(ValueError, match="path escapes repo root"):
            _repo_path("/etc/passwd")

    def test_absolute_inside_repo(self):
        inner = str(REPO_ROOT / "config" / "views.yaml")
        result = _repo_path(inner)
        assert result == Path(inner).resolve()

    def test_tilde_expansion_outside_repo(self):
        with pytest.raises(ValueError, match="path escapes repo root"):
            _repo_path("~/something")

    def test_repo_root_itself(self):
        result = _repo_path(str(REPO_ROOT))
        assert result == REPO_ROOT.resolve()

    def test_normalizes_redundant_slashes(self):
        result = _repo_path("config///views.yaml")
        assert result == (REPO_ROOT / "config" / "views.yaml").resolve()

    def test_normalizes_inner_dotdot(self):
        result = _repo_path("config/../config/views.yaml")
        assert result == (REPO_ROOT / "config" / "views.yaml").resolve()


class TestValidateProdlikeBackup:
    def test_ordinary_directory_rejected(self):
        error = _validate_prodlike_backup(REPO_ROOT / "mcp")
        assert error is not None
        assert "toc.dat not found" in error

    def test_directory_backup_with_toc_accepted(self, tmp_path: Path):
        backup = tmp_path / "backup.dump"
        backup.mkdir()
        (backup / "toc.dat").write_text("", encoding="utf-8")
        assert _validate_prodlike_backup(backup) is None

    def test_sql_file_accepted(self, tmp_path: Path):
        backup = tmp_path / "backup.sql"
        backup.write_text("", encoding="utf-8")
        assert _validate_prodlike_backup(backup) is None

    def test_dump_file_accepted(self, tmp_path: Path):
        backup = tmp_path / "backup.dump"
        backup.write_text("", encoding="utf-8")
        assert _validate_prodlike_backup(backup) is None

    def test_missing_path_rejected(self, tmp_path: Path):
        error = _validate_prodlike_backup(tmp_path / "missing")
        assert error is not None
        assert "not found" in error


def _restore_with_stub_subprocess(backup_path: Path | str):
    """Call restore_prodlike_db without spawning the restore script.

    Returns the tool result and the subprocess mock, so tests can assert the
    script was never launched.
    """
    create_subprocess = mock.AsyncMock(return_value=FakeProcess())
    with mock.patch("server.asyncio.create_subprocess_exec", create_subprocess):
        result = asyncio.run(server.restore_prodlike_db(str(backup_path)))
    return result, create_subprocess


class TestRestoreProdlikeDb:
    def test_ordinary_directory_rejected_before_restore_script(self):
        result, create_subprocess = _restore_with_stub_subprocess(
            REPO_ROOT / "mcp"
        )
        assert "toc.dat not found" in result
        create_subprocess.assert_not_awaited()

    def test_missing_path_rejected_before_restore_script(self):
        result, create_subprocess = _restore_with_stub_subprocess(
            REPO_ROOT / "missing-backup"
        )
        assert "not found" in result
        create_subprocess.assert_not_awaited()

    def test_empty_path_rejected_before_restore_script(self):
        result, create_subprocess = _restore_with_stub_subprocess("")
        assert result == "backup path is empty"
        create_subprocess.assert_not_awaited()

    def test_repository_root_rejected_before_restore_script(self):
        result, create_subprocess = _restore_with_stub_subprocess(".")
        assert "repository root" in result
        create_subprocess.assert_not_awaited()

    def _run_restore_script(
        self,
        tmp_path: Path,
        backup_name: str,
        dsn: str = "postgresql://postgres:password@localhost:5432/prodlike",
        pg_restore_list_exit: int = 0,
    ) -> tuple[subprocess.CompletedProcess, str]:
        """Run a copy of restore_prodlike_db.sh with logging psql/pg_restore stubs.

        ``pg_restore_list_exit`` sets the stub's exit code for ``pg_restore --list``
        (archive validation). Returns the process result and the logged commands.
        """
        repo = tmp_path / "repo"
        script_dir = repo / "scripts"
        script_dir.mkdir(parents=True)
        shutil.copy2(
            REPO_ROOT / "scripts" / "restore_prodlike_db.sh",
            script_dir / "restore_prodlike_db.sh",
        )

        bin_dir = tmp_path / "bin"
        bin_dir.mkdir()
        for command in ("bash", "dirname", "realpath", "sed"):
            source = shutil.which(command)
            assert source is not None
            os.symlink(source, bin_dir / command)
        command_log = tmp_path / "commands.log"
        command_log.write_text("", encoding="utf-8")
        for command in ("psql", "pg_restore"):
            stub = bin_dir / command
            stub.write_text(
                f'#!/bin/sh\necho "{command} $*" >> "$COMMAND_LOG"\n'
                'if [ "$1" = "--list" ]; then exit "$PG_RESTORE_LIST_EXIT"; fi\n',
                encoding="utf-8",
            )
            stub.chmod(0o755)

        result = subprocess.run(
            ["bash", str(script_dir / "restore_prodlike_db.sh"), backup_name],
            cwd=repo,
            env={
                "PATH": str(bin_dir),
                "HOME": str(tmp_path),
                "COMMAND_LOG": str(command_log),
                "PG_RESTORE_LIST_EXIT": str(pg_restore_list_exit),
                "SIPPY_PRODLIKE_DATABASE_DSN": dsn,
            },
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        return result, command_log.read_text(encoding="utf-8")

    def test_valid_directory_backup_reaches_pg_restore(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "valid.dump"
        backup.mkdir(parents=True)
        (backup / "toc.dat").write_text("", encoding="utf-8")
        result, commands = self._run_restore_script(tmp_path, "backups/valid.dump")
        assert result.returncode == 0, result.stderr
        assert "pg_restore --no-owner" in commands

    def test_sql_backup_reaches_psql(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "backup.sql"
        backup.parent.mkdir(parents=True)
        backup.write_text("", encoding="utf-8")
        result, commands = self._run_restore_script(tmp_path, "backups/backup.sql")
        assert result.returncode == 0, result.stderr
        assert "psql " in commands
        assert "backup.sql" in commands

    def test_script_rejects_ordinary_directory_before_drop(self, tmp_path: Path):
        (tmp_path / "repo" / "config").mkdir(parents=True)
        result, commands = self._run_restore_script(tmp_path, "config")
        assert result.returncode != 0
        assert "toc.dat not found" in result.stderr
        assert "psql" not in commands

    def test_script_rejects_invalid_archive_before_drop(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "broken.dump"
        backup.parent.mkdir(parents=True)
        backup.write_text("not an archive", encoding="utf-8")
        result, commands = self._run_restore_script(
            tmp_path, "backups/broken.dump", pg_restore_list_exit=1
        )
        assert result.returncode != 0
        assert "not a valid pg_restore archive" in result.stderr
        assert "psql" not in commands

    def test_script_logs_no_dsn_credentials(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "backup.sql"
        backup.parent.mkdir(parents=True)
        backup.write_text("", encoding="utf-8")
        user_password = secrets.token_urlsafe(24)
        query_password = secrets.token_urlsafe(24)
        result, _ = self._run_restore_script(
            tmp_path,
            "backups/backup.sql",
            dsn=(
                f"postgresql://postgres:{user_password}@localhost:5432/prodlike"
                f"?sslmode=disable&password={query_password}"
            ),
        )
        assert result.returncode == 0, result.stderr
        assert user_password not in result.stdout
        assert query_password not in result.stdout
        assert "prodlike on localhost:5432" in result.stdout

    def test_admin_dsn_keeps_query_only_password_and_port(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "valid.dump"
        backup.mkdir(parents=True)
        (backup / "toc.dat").write_text("", encoding="utf-8")
        password = secrets.token_urlsafe(24)
        query = f"user=postgres&password={password}&port=5433"
        result, commands = self._run_restore_script(
            tmp_path,
            "backups/valid.dump",
            dsn=f"postgresql://localhost/prodlike?{query}",
        )
        assert result.returncode == 0, result.stderr
        assert f"psql postgresql://localhost/postgres?{query} " in commands
        assert (
            f"pg_restore --no-owner --exit-on-error -d postgresql://localhost/prodlike?{query} "
            in commands
        )
        assert password not in result.stdout

    def test_script_accepts_encoded_credential_values(self, tmp_path: Path):
        backup = tmp_path / "repo" / "backups" / "valid.dump"
        backup.mkdir(parents=True)
        (backup / "toc.dat").write_text("", encoding="utf-8")
        result, commands = self._run_restore_script(
            tmp_path,
            "backups/valid.dump",
            dsn=(
                "postgresql://postgres:p%40ss@localhost:5432/prodlike"
                "?sslmode=disable&password=p%26ss"
            ),
        )
        assert result.returncode == 0, result.stderr
        assert "pg_restore --no-owner" in commands

    @pytest.mark.parametrize(
        "dsn",
        [
            pytest.param(
                "postgresql://postgres:password@localhost:5432,db.internal:5432/prodlike",
                id="multi-host",
            ),
            pytest.param(
                "postgresql://db.internal/postgres?x=@localhost:5432/prodlike",
                id="userinfo-smuggled-host",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost:5432/prodlike?host=db.internal",
                id="host-query-parameter",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost.internal:5432/prodlike",
                id="host-suffix",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost:5432/prodlike?ho%73t=db.internal",
                id="encoded-host-parameter",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost:5432/prodlike?%68ostaddr=db.internal",
                id="encoded-hostaddr-parameter",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost:5432/prodlike?serv%69ce=db.internal",
                id="encoded-service-parameter",
            ),
            pytest.param(
                "postgresql://postgres:password@localhost:5432/prodlike"
                "?sslmode=disable&db%6Eame=db.internal",
                id="encoded-dbname-parameter",
            ),
        ],
    )
    def test_script_rejects_non_local_dsn_before_drop(self, tmp_path: Path, dsn: str):
        backup = tmp_path / "repo" / "backups" / "valid.dump"
        backup.mkdir(parents=True)
        (backup / "toc.dat").write_text("", encoding="utf-8")
        result, commands = self._run_restore_script(
            tmp_path, "backups/valid.dump", dsn=dsn
        )
        assert result.returncode != 0
        assert "SIPPY_PRODLIKE_DATABASE_DSN" in result.stderr
        assert "db.internal" not in result.stdout + result.stderr
        assert commands == ""


class TestTrim:
    def test_short_string_unchanged(self):
        assert _trim("hello") == "hello"

    def test_at_limit_unchanged(self):
        s = "x" * 100
        assert _trim(s, max_len=100) == s

    def test_over_limit_truncated(self):
        s = "A" * 30000
        result = _trim(s, max_len=1000)
        assert len(result) < len(s)
        assert "characters omitted" in result
        head = 500
        tail = 1000 - 500 - 120
        assert result.startswith("A" * head)
        assert result.endswith("A" * tail)

    def test_omission_count_correct(self):
        s = "x" * 30000
        result = _trim(s, max_len=1000)
        head = 500
        tail = 1000 - 500 - 120
        expected_omitted = 30000 - head - tail
        assert str(expected_omitted) in result


class TestResolveBigqueryCreds:
    def test_explicit_path_exists(self):
        with tempfile.NamedTemporaryFile(suffix=".json") as f:
            path, err = _resolve_bigquery_creds(f.name)
            assert err is None
            assert path == Path(f.name).resolve()

    def test_explicit_path_missing(self):
        path, err = _resolve_bigquery_creds("/nonexistent/creds.json")
        assert path is None
        assert "not found" in err

    def test_env_var_fallback(self):
        with tempfile.NamedTemporaryFile(suffix=".json") as f:
            with mock.patch.dict(
                os.environ,
                {"SIPPY_BIGQUERY_CREDENTIALS_FILE": f.name},
                clear=False,
            ):
                path, err = _resolve_bigquery_creds(None)
                assert err is None
                assert path == Path(f.name).resolve()

    def test_env_var_second_fallback(self):
        with tempfile.NamedTemporaryFile(suffix=".json") as f:
            env = {
                "GOOGLE_APPLICATION_CREDENTIALS": f.name,
            }
            with mock.patch.dict(os.environ, env, clear=False):
                os.environ.pop("SIPPY_BIGQUERY_CREDENTIALS_FILE", None)
                path, err = _resolve_bigquery_creds(None)
                assert err is None
                assert path == Path(f.name).resolve()

    def test_env_var_file_missing(self):
        with mock.patch.dict(
            os.environ,
            {"SIPPY_BIGQUERY_CREDENTIALS_FILE": "/nonexistent/sa.json"},
            clear=False,
        ):
            path, err = _resolve_bigquery_creds(None)
            assert path is None
            assert "SIPPY_BIGQUERY_CREDENTIALS_FILE" in err
            assert "not found" in err

    def test_no_creds_anywhere(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("SIPPY_BIGQUERY_CREDENTIALS_FILE", None)
            os.environ.pop("GOOGLE_APPLICATION_CREDENTIALS", None)
            path, err = _resolve_bigquery_creds(None)
            assert path is None
            assert "Set bigquery_credentials_file" in err

    def test_explicit_overrides_env(self):
        with tempfile.NamedTemporaryFile(suffix=".json") as explicit:
            with tempfile.NamedTemporaryFile(suffix=".json") as env_file:
                with mock.patch.dict(
                    os.environ,
                    {"SIPPY_BIGQUERY_CREDENTIALS_FILE": env_file.name},
                    clear=False,
                ):
                    path, err = _resolve_bigquery_creds(explicit.name)
                    assert err is None
                    assert path == Path(explicit.name).resolve()

    def test_first_env_var_takes_priority(self):
        with tempfile.NamedTemporaryFile(suffix=".json") as first:
            with tempfile.NamedTemporaryFile(suffix=".json") as second:
                env = {
                    "SIPPY_BIGQUERY_CREDENTIALS_FILE": first.name,
                    "GOOGLE_APPLICATION_CREDENTIALS": second.name,
                }
                with mock.patch.dict(os.environ, env, clear=False):
                    path, err = _resolve_bigquery_creds(None)
                    assert err is None
                    assert path == Path(first.name).resolve()


class TestRunE2e:
    """Verify the run_e2e BigQuery opt-in behavior and published tool schema."""

    @pytest.fixture(autouse=True)
    def isolated_artifacts(self, tmp_path, monkeypatch):
        monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))

    @staticmethod
    def _successful_subprocess():
        """Return a successful async subprocess double for the make-based consumer path."""
        process = mock.Mock()
        process.wait = mock.AsyncMock(return_value=0)
        return process

    def test_defaults_to_no_bigquery_without_credentials(self, tmp_path):
        """Run without credentials by default and suppress BigQuery in the child environment."""
        process = self._successful_subprocess()
        env = {
            key: value
            for key, value in os.environ.items()
            if key
            not in (
                "GCS_SA_JSON_PATH",
                "SIPPY_BIGQUERY_CREDENTIALS_FILE",
                "GOOGLE_APPLICATION_CREDENTIALS",
            )
        }
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch("server.DEV_LOG_DIR", tmp_path),
            mock.patch(
                "server.asyncio.create_subprocess_exec",
                new=mock.AsyncMock(return_value=process),
            ) as create_subprocess,
            mock.patch("server._resolve_bigquery_creds") as resolve_creds,
        ):
            result = asyncio.run(run_e2e())

        assert "run_e2e succeeded" in result
        resolve_creds.assert_not_called()
        assert create_subprocess.await_args.kwargs["env"]["GCS_SA_JSON_PATH"] == ""

    def test_disabled_ignores_all_credentials_and_preserves_parent_env(self, tmp_path):
        """Ignore every credential source when disabled without mutating files or parent state."""
        process = self._successful_subprocess()
        with (
            tempfile.NamedTemporaryFile(suffix=".json") as explicit,
            tempfile.NamedTemporaryFile(suffix=".json") as sippy_env,
            tempfile.NamedTemporaryFile(suffix=".json") as google_env,
            mock.patch.dict(
                os.environ,
                {
                    "GCS_SA_JSON_PATH": "/inherited/credentials.json",
                    "SIPPY_BIGQUERY_CREDENTIALS_FILE": sippy_env.name,
                    "GOOGLE_APPLICATION_CREDENTIALS": google_env.name,
                },
                clear=False,
            ),
            mock.patch("server.DEV_LOG_DIR", tmp_path),
            mock.patch(
                "server.asyncio.create_subprocess_exec",
                new=mock.AsyncMock(return_value=process),
            ) as create_subprocess,
            mock.patch("server._resolve_bigquery_creds") as resolve_creds,
        ):
            credential_contents = {
                explicit.name: secrets.token_bytes(24),
                sippy_env.name: secrets.token_bytes(24),
                google_env.name: secrets.token_bytes(24),
            }
            for path, contents in credential_contents.items():
                Path(path).write_bytes(contents)

            result = asyncio.run(run_e2e(explicit.name, include_bigquery=False))

            assert os.environ["GCS_SA_JSON_PATH"] == "/inherited/credentials.json"
            for path, contents in credential_contents.items():
                assert Path(path).read_bytes() == contents

        assert "run_e2e succeeded" in result
        resolve_creds.assert_not_called()
        assert create_subprocess.await_args.kwargs["env"]["GCS_SA_JSON_PATH"] == ""

    def test_bigquery_opt_in_resolves_explicit_credentials(self, tmp_path):
        """Resolve an explicit credential file only when BigQuery coverage is requested."""
        process = self._successful_subprocess()
        with (
            tempfile.NamedTemporaryFile(suffix=".json") as explicit,
            mock.patch("server.DEV_LOG_DIR", tmp_path),
            mock.patch(
                "server.asyncio.create_subprocess_exec",
                new=mock.AsyncMock(return_value=process),
            ) as create_subprocess,
        ):
            result = asyncio.run(run_e2e(explicit.name, include_bigquery=True))

        assert "run_e2e succeeded" in result
        assert create_subprocess.await_args.kwargs["env"]["GCS_SA_JSON_PATH"] == str(
            Path(explicit.name).resolve()
        )

    @pytest.mark.parametrize(
        ("credentials_file", "expected_error"),
        [
            (None, "Set bigquery_credentials_file"),
            ("/nonexistent/creds.json", "BigQuery credentials file not found"),
        ],
    )
    def test_bigquery_opt_in_requires_valid_credentials(
        self, credentials_file, expected_error
    ):
        """Return existing resolver errors and skip make when opt-in credentials are invalid."""
        env = {
            key: value
            for key, value in os.environ.items()
            if key not in ("SIPPY_BIGQUERY_CREDENTIALS_FILE", "GOOGLE_APPLICATION_CREDENTIALS")
        }
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch("server._run_make_phase", new=mock.AsyncMock()) as run_make_phase,
        ):
            result = asyncio.run(
                run_e2e(credentials_file, include_bigquery=True)
            )

        assert expected_error in result
        run_make_phase.assert_not_awaited()

    def test_include_bigquery_is_appended_and_defaults_false_in_schema(self):
        """Expose the appended include_bigquery parameter as a false-defaulting boolean."""
        parameters = inspect.signature(run_e2e).parameters

        assert list(parameters) == [
            "bigquery_credentials_file",
            "timeout_seconds",
            "include_bigquery",
            "wait_seconds",
        ]
        assert parameters["include_bigquery"].annotation is bool
        assert parameters["include_bigquery"].default is False

        tool = asyncio.run(mcp.get_tool("run_e2e"))
        include_bigquery_schema = tool.parameters["properties"]["include_bigquery"]
        assert include_bigquery_schema == {"default": False, "type": "boolean"}


class TestE2eScriptSafety:
    """Exercise the e2e shell helpers without contacting external services."""

    @staticmethod
    def _run_helper(body, env=None):
        """Source the production script and run a helper body in an isolated Bash process."""
        run_env = os.environ.copy()
        if env:
            run_env.update(env)
        return subprocess.run(
            ["bash", "-c", f'source "$1"\n{body}', "bash", str(E2E_SCRIPT)],
            cwd=REPO_ROOT,
            env=run_env,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_existing_service_configuration_does_not_log_connection_secrets(self):
        """Keep credential-bearing URLs in command arguments and out of script output."""
        database_password = secrets.token_urlsafe(24)
        redis_password = secrets.token_urlsafe(24)
        admin_dsn = (
            f"postgresql://e2e-user:{database_password}@sippy-postgres:5432/postgres"
        )
        e2e_dsn = (
            f"postgresql://e2e-user:{database_password}@sippy-postgres:5432/sippy_e2e"
        )
        redis_url = f"redis://:{redis_password}@sippy-redis:6379/1"
        result = self._run_helper(
            """
psql_calls=0
psql() {
    psql_calls=$((psql_calls + 1))
    [ "$1" = "$EXPECTED_ADMIN_DSN" ] || exit 90
}
e2e_configure_existing_services
[ "$psql_calls" -eq 2 ] || exit 91
[ "$SIPPY_E2E_DSN" = "$EXPECTED_E2E_DSN" ] || exit 92
[ "$REDIS_URL" = "$EXPECTED_REDIS_URL" ] || exit 93
""",
            {
                "SIPPY_DATABASE_DSN": admin_dsn,
                "SIPPY_E2E_REDIS_URL": redis_url,
                "EXPECTED_ADMIN_DSN": admin_dsn,
                "EXPECTED_E2E_DSN": e2e_dsn,
                "EXPECTED_REDIS_URL": redis_url,
            },
        )

        output = result.stdout + result.stderr
        assert result.returncode == 0, output
        assert database_password not in output
        assert redis_password not in output
        assert admin_dsn not in output
        assert e2e_dsn not in output
        assert redis_url not in output

    def test_existing_service_configuration_fails_on_unexpected_dsn(self):
        """Propagate a psql argument mismatch as exit 90 instead of masking it later."""
        actual_password = secrets.token_urlsafe(24)
        expected_password = secrets.token_urlsafe(24)
        actual_dsn = (
            f"postgresql://e2e-user:{actual_password}@sippy-postgres:5432/postgres"
        )
        expected_dsn = (
            f"postgresql://e2e-user:{expected_password}@sippy-postgres:5432/postgres"
        )
        result = self._run_helper(
            """
psql() {
    [ "$1" = "$EXPECTED_ADMIN_DSN" ] || exit 90
}
e2e_configure_existing_services
""",
            {
                "SIPPY_DATABASE_DSN": actual_dsn,
                "EXPECTED_ADMIN_DSN": expected_dsn,
            },
        )

        output = result.stdout + result.stderr
        assert result.returncode == 90
        assert actual_password not in output
        assert expected_password not in output
        assert actual_dsn not in output
        assert expected_dsn not in output

    @pytest.mark.parametrize(
        "host",
        ["localhost", "sippy-postgres"],
    )
    def test_safe_dsn_host_still_accepts_local_services(self, host):
        """Preserve the localhost and sippy-postgres allowlist used by destructive helpers."""
        password = secrets.token_urlsafe(24)
        dsn = f"postgresql://user:{password}@{host}:5432/postgres"
        result = self._run_helper('e2e_safe_dsn_host "$SAFE_DSN"', {"SAFE_DSN": dsn})

        assert result.returncode == 0, result.stderr
        assert result.stdout == ""
        assert result.stderr == ""
        assert password not in result.stdout + result.stderr

    def test_safe_dsn_host_rejection_does_not_echo_credentials(self):
        """Reject remote database hosts without copying their credential-bearing DSN to stderr."""
        password = secrets.token_urlsafe(24)
        rejected_dsn = f"postgresql://user:{password}@database.example.com:5432/postgres"
        result = self._run_helper(
            'e2e_safe_dsn_host "$REJECTED_DSN"',
            {"REJECTED_DSN": rejected_dsn},
        )

        output = result.stdout + result.stderr
        assert result.returncode == 1
        assert "refusing unsafe e2e database connection settings" in result.stderr
        assert password not in output
        assert rejected_dsn not in output

    def test_devcontainer_entry_uses_safe_arguments_without_logging_secrets(
        self, tmp_path
    ):
        """Exercise the real devcontainer entry path with service and build commands stubbed."""
        database_password = secrets.token_urlsafe(24)
        redis_password = secrets.token_urlsafe(24)
        admin_dsn = (
            f"postgresql://e2e-user:{database_password}@sippy-postgres:5432/postgres"
        )
        e2e_dsn = (
            f"postgresql://e2e-user:{database_password}@sippy-postgres:5432/sippy_e2e"
        )
        redis_url = f"redis://:{redis_password}@sippy-redis:6379/1"
        call_log = tmp_path / "psql-calls.log"
        bash_env = tmp_path / "bash-env"
        bash_env.write_text(
            """
function [ {
    if [[ "$1" == "-f" && "$2" == "/run/.containerenv" ]]; then
        return 0
    fi
    builtin [ "$@"
}
psql() {
    [ "$1" = "$EXPECTED_ADMIN_DSN" ] || exit 90
    printf '%s\\n' "${*: -2}" >> "$PSQL_CALL_LOG"
}
go() {
    [ "$1" = "build" ] || exit 94
    return 95
}
"""
        )
        env = os.environ.copy()
        env.update(
            {
                "BASH_ENV": str(bash_env),
                "GCS_SA_JSON_PATH": "",
                "SIPPY_DATABASE_DSN": admin_dsn,
                "SIPPY_E2E_REDIS_URL": redis_url,
                "EXPECTED_ADMIN_DSN": admin_dsn,
                "PSQL_CALL_LOG": str(call_log),
            }
        )

        result = subprocess.run(
            ["bash", str(E2E_SCRIPT)],
            cwd=tmp_path,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )

        output = result.stdout + result.stderr
        assert result.returncode == 1, output
        assert "Detected devcontainer" in result.stdout
        assert "Building sippy with coverage instrumentation" in result.stdout
        assert "Build failed" in result.stdout
        assert call_log.read_text().splitlines() == [
            "-c DROP DATABASE IF EXISTS sippy_e2e",
            "-c CREATE DATABASE sippy_e2e",
            "-c DROP DATABASE IF EXISTS sippy_e2e",
        ]
        for secret in (
            database_password,
            redis_password,
            admin_dsn,
            e2e_dsn,
            redis_url,
        ):
            assert secret not in output

    @pytest.mark.parametrize(
        ("database_name", "expected_returncode"),
        [("sippy_e2e", 0), ("production", 1)],
    )
    def test_safe_database_name_guard_is_preserved(
        self, database_name, expected_returncode
    ):
        """Continue allowing only database names that contain the required _e2e marker."""
        result = self._run_helper(
            'e2e_safe_db_name "$DATABASE_NAME"',
            {"DATABASE_NAME": database_name},
        )

        assert result.returncode == expected_returncode
        if expected_returncode:
            assert "name must contain '_e2e'" in result.stderr


class TestDataMode:
    def test_default_is_seed(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("SIPPY_DATA_MODE", None)
            assert _data_mode() == "seed"

    def test_seed_mode(self):
        with mock.patch.dict(os.environ, {"SIPPY_DATA_MODE": "seed"}, clear=False):
            assert _data_mode() == "seed"

    def test_prod_like_mode(self):
        with mock.patch.dict(os.environ, {"SIPPY_DATA_MODE": "prod-like"}, clear=False):
            assert _data_mode() == "prod-like"

    def test_invalid_mode_falls_back_to_seed(self):
        with mock.patch.dict(os.environ, {"SIPPY_DATA_MODE": "invalid"}, clear=False):
            assert _data_mode() == "seed"

    def test_case_insensitive(self):
        with mock.patch.dict(os.environ, {"SIPPY_DATA_MODE": "PROD-LIKE"}, clear=False):
            assert _data_mode() == "prod-like"


class TestDsnForMode:
    def test_seed_mode_default(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("SIPPY_SEED_DATABASE_DSN", None)
            os.environ.pop("SIPPY_DATABASE_DSN", None)
            assert _dsn_for_mode("seed") == "postgresql://postgres:password@localhost:5432/postgres"

    def test_seed_mode_from_env(self):
        with mock.patch.dict(
            os.environ, {"SIPPY_SEED_DATABASE_DSN": "postgresql://custom/seed"}, clear=False
        ):
            assert _dsn_for_mode("seed") == "postgresql://custom/seed"

    def test_seed_mode_falls_back_to_sippy_database_dsn(self):
        with mock.patch.dict(
            os.environ, {"SIPPY_DATABASE_DSN": "postgresql://legacy/db"}, clear=False
        ):
            os.environ.pop("SIPPY_SEED_DATABASE_DSN", None)
            assert _dsn_for_mode("seed") == "postgresql://legacy/db"

    def test_prod_like_mode_default(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("SIPPY_PRODLIKE_DATABASE_DSN", None)
            assert (
                _dsn_for_mode("prod-like")
                == "postgresql://postgres:password@localhost:5432/prodlike"
            )

    def test_prod_like_mode_from_env(self):
        with mock.patch.dict(
            os.environ,
            {"SIPPY_PRODLIKE_DATABASE_DSN": "postgresql://custom/prodlike"},
            clear=False,
        ):
            assert _dsn_for_mode("prod-like") == "postgresql://custom/prodlike"


class TestValidateDsn:
    def test_valid_dsn(self):
        assert _validate_dsn("postgresql://postgres:password@localhost:5432/postgres") is None

    def test_invalid_scheme(self):
        assert _validate_dsn("mysql://localhost/db") is not None

    def test_empty_string(self):
        assert _validate_dsn("") is not None

    def test_shell_metacharacters(self):
        assert _validate_dsn("postgresql://x; rm -rf /") is not None

    def test_whitespace_rejected(self):
        assert _validate_dsn("postgresql://ok\nnewline") is not None


class TestValidateRedisUrl:
    def test_valid_redis(self):
        assert _validate_redis_url("redis://localhost:6379") is None

    def test_valid_rediss(self):
        assert _validate_redis_url("rediss://secure:6380/0") is None

    def test_invalid_scheme(self):
        assert _validate_redis_url("http://localhost:6379") is not None

    def test_empty_string(self):
        assert _validate_redis_url("") is not None

    def test_shell_metacharacters(self):
        assert _validate_redis_url("redis://x$(whoami)") is None  # no whitespace, exec-safe

    def test_whitespace_rejected(self):
        assert _validate_redis_url("redis://ok \t") is not None


class TestDefaults:
    def test_database_dsn_default(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("SIPPY_DATABASE_DSN", None)
            assert _default_database_dsn() == "postgresql://postgres:password@localhost:5432/postgres"

    def test_database_dsn_from_env(self):
        with mock.patch.dict(
            os.environ, {"SIPPY_DATABASE_DSN": "postgresql://other:5433/db"}, clear=False
        ):
            assert _default_database_dsn() == "postgresql://other:5433/db"

    def test_redis_url_default(self):
        with mock.patch.dict(os.environ, clear=False):
            os.environ.pop("REDIS_URL", None)
            assert _default_redis_url() == "redis://localhost:6379"

    def test_redis_url_from_env(self):
        with mock.patch.dict(
            os.environ, {"REDIS_URL": "redis://other:6380"}, clear=False
        ):
            assert _default_redis_url() == "redis://other:6380"


class TestPidAlive:
    def test_current_process_alive(self):
        assert _pid_alive(os.getpid()) is True

    def test_nonexistent_pid(self):
        assert _pid_alive(99999999) is False


class TestWaitForUrl:
    def test_returns_none_when_url_responds(self):
        with mock.patch("server.urllib.request.urlopen"):
            result = asyncio.run(
                _wait_for_url("http://127.0.0.1:8080", 5, [os.getpid()])
            )
            assert result is None

    def test_returns_error_on_timeout(self):
        with mock.patch(
            "server.urllib.request.urlopen", side_effect=ConnectionRefusedError
        ):
            result = asyncio.run(
                _wait_for_url("http://127.0.0.1:8080", 2, [os.getpid()])
            )
            assert result is not None
            assert "not ready after 2s" in result

    def test_returns_error_when_pid_dies(self):
        with mock.patch(
            "server.urllib.request.urlopen", side_effect=ConnectionRefusedError
        ):
            result = asyncio.run(
                _wait_for_url("http://127.0.0.1:8080", 5, [99999999])
            )
            assert result is not None
            assert "exited" in result


class TestMcpTestEnvironment:
    @pytest.mark.parametrize("target", ["lint", "test"])
    def test_non_e2e_environment_preserves_service_settings(self, tmp_path, target):
        parent = {
            "HOME": str(tmp_path),
            "SIPPY_DATABASE_DSN": "postgresql://legacy/db",
            "SIPPY_SEED_DATABASE_DSN": "postgresql://seed/db",
            "REDIS_URL": "redis://[malformed",
        }
        if target == "lint":
            parent["ARTIFACT_DIR"] = "/proc/sippy-read-only-artifacts"
            parent["PLAYWRIGHT_BROWSERS_PATH"] = "/proc/sippy-read-only-browsers"
        env = testenv.prepare_environment(parent, tmp_path / "logs", target)
        for key, value in parent.items():
            assert env[key] == value
        assert "SIPPY_E2E_REDIS_URL" not in env
        assert "SIPPY_TEST_OUTPUT_DIR" not in env
        if target == "test":
            assert Path(env["ARTIFACT_DIR"]).is_dir()

    @pytest.mark.parametrize("target", ["", "tests", "E2E", "unknown"])
    def test_unknown_targets_fail_before_side_effects(self, tmp_path, target):
        with mock.patch("testenv.socket.socket") as listener:
            with pytest.raises(ValueError, match="expected lint, test or e2e"):
                testenv.check_prerequisites(target)
            listener.assert_not_called()
        with pytest.raises(ValueError, match="expected lint, test or e2e"):
            testenv.prepare_environment({}, tmp_path / "unused", target)
        assert not (tmp_path / "unused").exists()

    @pytest.mark.parametrize("port", [23433, 23479])
    def test_host_launcher_can_replace_containers_holding_service_ports(self, port):
        with socket.socket() as occupied:
            occupied.bind(("127.0.0.1", port))
            occupied.listen()
            with mock.patch("testenv.Path.exists", return_value=False):
                testenv.check_prerequisites("e2e")

    @pytest.mark.parametrize("suffix", ["user:@localhost/db", "localhost/db?password="])
    def test_empty_uri_password_replaces_inherited_password(self, suffix):
        env = testenv.postgres_environment({
            "SIPPY_DATABASE_DSN": f"postgresql://{suffix}",
            "PGPASSWORD": "inherited",
        })
        assert env["PGPASSWORD"] == ""

    def test_validation_error_identifies_redis_setting_before_make(self, tmp_path, monkeypatch):
        monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
        monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))
        monkeypatch.delenv("SIPPY_E2E_REDIS_URL", raising=False)
        monkeypatch.setenv("REDIS_URL", "redis://private@[malformed")
        create = mock.AsyncMock()
        monkeypatch.setattr(server.asyncio, "create_subprocess_exec", create)
        result = asyncio.run(run_e2e())
        assert "REDIS_URL has a malformed URI authority" in result
        assert "ARTIFACT_DIR" not in result
        assert "private" not in result
        create.assert_not_awaited()

    def test_filesystem_error_is_distinct_from_setting_validation(self, monkeypatch):
        monkeypatch.setattr(server, "prepare_environment", mock.Mock(side_effect=PermissionError("private")))
        result = asyncio.run(server.run_lint())
        assert "cannot prepare test filesystem paths" in result
        assert "private" not in result

    @pytest.mark.parametrize("parent", [
        {}, {"SIPPY_DATABASE_DSN": "postgresql://legacy/db"},
        {"SIPPY_SEED_DATABASE_DSN": "postgresql://seed/db", "SIPPY_DATABASE_DSN": "postgresql://legacy/db"},
        {"SIPPY_SEED_DATABASE_DSN": "", "SIPPY_DATABASE_DSN": "postgresql://legacy/db"},
    ])
    def test_seed_selection_matches_server(self, parent, tmp_path):
        with mock.patch.dict(os.environ, parent, clear=True):
            assert testenv.prepare_environment(parent, tmp_path)["SIPPY_DATABASE_DSN"] == _dsn_for_mode("seed")

    def test_derived_redis_database_discards_query_options(self, tmp_path):
        env = testenv.prepare_environment({"REDIS_URL": "redis://:test@localhost:6379/0?db=0"}, tmp_path)
        assert env["SIPPY_E2E_REDIS_URL"] == "redis://:test@localhost:6379/1"

    @pytest.mark.parametrize(
        "dsn,expected",
        [
            (
                "postgresql://user%40name:p%2Bss%3Dword@localhost:5433/test%20db?sslmode=require&application_name=preflight",
                {"PGHOST": "localhost", "PGPORT": "5433", "PGUSER": "user@name", "PGPASSWORD": "p+ss=word",
                 "PGDATABASE": "test db", "PGSSLMODE": "require", "PGAPPNAME": "preflight"},
            ),
            (
                "postgres://user:old@localhost:5433/original?host=%2Ftmp&dbname=selected&password=new+secret&options=-c%20search_path%3Dpublic",
                {"PGHOST": "localhost", "PGPORT": "5433", "PGUSER": "user", "PGPASSWORD": "new+secret",
                 "PGDATABASE": "original", "PGOPTIONS": "-c search_path=public"},
            ),
            ("postgresql://localhost", {"PGHOST": "localhost", "PGDATABASE": "inherited", "PGUSER": "inherited"}),
        ],
    )
    def test_postgres_parameters_use_libpq_environment(self, dsn, expected):
        parent = {"SIPPY_DATABASE_DSN": dsn, "PGDATABASE": "inherited", "PGUSER": "inherited"}
        env = testenv.postgres_environment(parent)
        assert {key: env[key] for key in expected} == expected
        assert env["PGCONNECT_TIMEOUT"] == "10"
        assert "PGHOST" not in parent

    @pytest.mark.parametrize("option", ["sslpassword=private", "invalid=private", "private"])
    def test_unsupported_postgres_options_fail_without_values(self, option):
        with pytest.raises(ValueError) as error:
            testenv.postgres_environment({"SIPPY_DATABASE_DSN": f"postgresql://localhost/postgres?{option}"})
        assert "private" not in str(error.value)

    @pytest.mark.parametrize("platform", ["linux", "darwin", "win32"])
    @pytest.mark.parametrize("browser_path", [None, "0", "explicit"])
    def test_usable_browser_locations_are_preserved(self, tmp_path, monkeypatch, platform, browser_path):
        monkeypatch.setattr(testenv.sys, "platform", platform)
        parent = {"HOME": str(tmp_path), "LOCALAPPDATA": str(tmp_path / "AppData/Local")}
        if browser_path is not None:
            parent["PLAYWRIGHT_BROWSERS_PATH"] = (
                str(tmp_path / "browsers") if browser_path == "explicit" else browser_path
            )
        env = testenv.prepare_environment(parent, tmp_path / "logs")
        assert env.get("PLAYWRIGHT_BROWSERS_PATH") == parent.get("PLAYWRIGHT_BROWSERS_PATH")
        if platform == "linux" and browser_path is None:
            assert Path(env.get("XDG_CACHE_HOME", tmp_path / ".cache")) == tmp_path / ".cache"

    def test_inherited_xdg_browser_cache_is_preserved(self, tmp_path):
        parent = {"XDG_CACHE_HOME": str(tmp_path / "shared-cache")}
        env = testenv.prepare_environment(parent, tmp_path / "logs")
        assert env["XDG_CACHE_HOME"] == parent["XDG_CACHE_HOME"]
        assert "PLAYWRIGHT_BROWSERS_PATH" not in env

    def test_browser_cache_survives_xdg_parent_fallback(self, tmp_path, monkeypatch):
        monkeypatch.setattr(testenv.sys, "platform", "linux")
        xdg_cache = tmp_path / "shared-cache"
        browser_cache = xdg_cache / "ms-playwright"
        browser_cache.mkdir(parents=True)
        writable_directory = testenv.writable_directory

        def probe(path):
            if path == xdg_cache:
                raise PermissionError("cache parent is not writable")
            return writable_directory(path)

        monkeypatch.setattr(testenv, "writable_directory", probe)
        env = testenv.prepare_environment({"XDG_CACHE_HOME": str(xdg_cache)}, tmp_path / "logs")
        assert env["XDG_CACHE_HOME"] != str(xdg_cache)
        assert env["PLAYWRIGHT_BROWSERS_PATH"] == str(browser_cache)

    @pytest.mark.parametrize("explicit", [False, True])
    def test_unusable_browser_location_gets_writable_fallback(self, tmp_path, monkeypatch, explicit):
        monkeypatch.setattr(testenv.sys, "platform", "linux")

        blocked = tmp_path / "blocked"
        blocked.write_text("not a directory")
        parent = {"HOME": str(tmp_path), "XDG_CACHE_HOME": str(tmp_path / "cache")}
        if explicit:
            parent["PLAYWRIGHT_BROWSERS_PATH"] = str(blocked)
        else:
            (tmp_path / "cache").mkdir()
            (tmp_path / "cache/ms-playwright").write_text("not a directory")
        env = testenv.prepare_environment(parent, tmp_path / "logs")
        assert env["PLAYWRIGHT_BROWSERS_PATH"] == str(tmp_path / "logs/test-cache/playwright_browsers_path")

    def test_writable_fallback_preserves_auth_and_artifacts(self, tmp_path):
        artifacts = tmp_path / "collected"
        parent = {
            "GOCACHE": "/proc/sippy-read-only-cache",
            "TMPDIR": "/proc/sippy-read-only-tmp",
            "NPM_CONFIG_CACHE": "/proc/sippy-read-only-npm",
            "ARTIFACT_DIR": str(artifacts),
            "SIPPY_SEED_DATABASE_DSN": "postgresql://user:secret@localhost/postgres?sslmode=disable",
            "REDIS_URL": "redis://user:secret@localhost:6379/0",
        }
        env = testenv.prepare_environment(parent, tmp_path / "logs")
        assert env["GOCACHE"] == str(tmp_path / "logs/test-cache/gocache")
        assert env["TMPDIR"] == str(tmp_path / "logs/test-cache/tmpdir")
        assert env["NPM_CONFIG_CACHE"] == env["npm_config_cache"]
        assert env["npm_config_cache"] == str(tmp_path / "logs/test-cache/npm_config_cache")
        assert env["ARTIFACT_DIR"] == str(artifacts)
        assert env["SIPPY_TEST_OUTPUT_DIR"] == str(artifacts / "e2e")
        assert env["SIPPY_DATABASE_DSN"] == parent["SIPPY_SEED_DATABASE_DSN"]
        assert env["SIPPY_E2E_REDIS_URL"] == "redis://user:secret@localhost:6379/1"
        assert parent["GOCACHE"] == "/proc/sippy-read-only-cache"

    def test_writable_cache_and_explicit_redis_are_kept(self, tmp_path):
        parent = {"GOCACHE": str(tmp_path / "cache"), "SIPPY_E2E_REDIS_URL": "redis://localhost/5"}
        env = testenv.prepare_environment(parent, tmp_path / "logs")
        assert env["GOCACHE"] == parent["GOCACHE"]
        assert env["SIPPY_E2E_REDIS_URL"] == parent["SIPPY_E2E_REDIS_URL"]

    def test_unwritable_artifacts_fail_before_make(self, tmp_path, monkeypatch):
        monkeypatch.setenv("ARTIFACT_DIR", "/proc/sippy-read-only-artifacts")
        monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path)
        create = mock.AsyncMock()
        monkeypatch.setattr(server.asyncio, "create_subprocess_exec", create)
        result = asyncio.run(run_e2e())
        assert "prerequisite failed" in result
        assert "ARTIFACT_DIR" in result
        create.assert_not_awaited()

    def test_listener_denial_has_actionable_diagnostic(self):
        with mock.patch("testenv.socket.socket", side_effect=PermissionError(1, "Operation not permitted")):
            with pytest.raises(RuntimeError, match="MCP server needs local socket permissions"):
                testenv.check_prerequisites()

    @pytest.mark.parametrize(
        "dsn,expected_host,expected_database",
        [
            pytest.param("postgresql://localhost/postgres", "localhost", "postgres", id="single-host"),
            pytest.param("postgresql://sippy-postgres/admin", "sippy-postgres", "admin", id="container-host"),
            pytest.param("postgresql://localhost", "localhost", "inherited", id="pathless"),
            pytest.param(
                "postgresql://localhost/postgres?host=first%2Csecond%2Cthird", "localhost", "postgres",
                id="query-host-override",
            ),
            pytest.param(
                "postgresql://localhost/postgres?hostaddr=127.0.0.2", "localhost", "postgres",
                id="query-host-address",
            ),
            pytest.param(
                "postgresql://localhost/postgres?dbname=other", "localhost", "postgres",
                id="query-database",
            ),
            pytest.param(
                "postgresql://localhost/postgres?service=other", "localhost", "postgres",
                id="query-service",
            ),
            pytest.param(
                "postgresql://localhost/postgres?ho%73t=other&host%61ddr=127.0.0.2&db%6eame=other&%73ervice=other",
                "localhost", "postgres", id="encoded-target-overrides",
            ),
        ],
    )
    def test_postgres_prerequisite_matches_e2e_connection_target(self, dsn, expected_host, expected_database):
        env = {
            "SIPPY_DATABASE_DSN": dsn + ("&" if "?" in dsn else "?") + "user=review&password=private&sslmode=require&application_name=preflight",
            "SIPPY_E2E_REDIS_URL": "redis://localhost/1",
            "PGHOST": "other", "PGHOSTADDR": "127.0.0.2,127.0.0.3", "PGSERVICE": "other",
            "PGDATABASE": "inherited",
        }
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch("testenv.socket.socket"),
            mock.patch("testenv.Path.exists", return_value=True),
            mock.patch("testenv.subprocess.run", return_value=subprocess.CompletedProcess([], 0, "t\n")) as run,
            mock.patch("testenv.redis_ping"),
        ):
            testenv.check_prerequisites()
        child = run.call_args.kwargs["env"]
        assert child["PGHOST"] == expected_host
        assert child["PGDATABASE"] == expected_database
        assert "PGHOSTADDR" not in child
        assert "PGSERVICE" not in child
        assert child["PGUSER"] == "review"
        assert child["PGPASSWORD"] == "private"
        assert child["PGSSLMODE"] == "require"
        assert child["PGAPPNAME"] == "preflight"
        assert child["PGCONNECT_TIMEOUT"] == "10"
        assert run.call_args.kwargs["timeout"] == 15
        assert all("private" not in argument for argument in run.call_args.args[0])
        assert env["PGHOSTADDR"] == "127.0.0.2,127.0.0.3"

    @pytest.mark.parametrize("dsn", [
        "postgresql://remote/postgres?host=localhost",
        "postgresql://localhost,remote/postgres",
        "postgresql:///postgres?host=localhost",
        "postgresql://localhost:notaport/postgres",
        "postgresql://localhost:99999/postgres",
    ])
    def test_postgres_prerequisite_rejects_unsafe_authorities_before_psql(self, dsn):
        with (
            mock.patch.dict(os.environ, {"SIPPY_DATABASE_DSN": dsn}, clear=True),
            mock.patch("testenv.socket.socket"),
            mock.patch("testenv.Path.exists", return_value=True),
            mock.patch("testenv.subprocess.run") as run,
        ):
            with pytest.raises(RuntimeError, match="PostgreSQL SIPPY_DATABASE_DSN validation failed"):
                testenv.check_prerequisites()
        run.assert_not_called()

    @pytest.mark.parametrize("failure,expected", [
        ("postgres", "PostgreSQL connection/authentication failed"),
        ("permission", "PostgreSQL CREATEDB permission denied"),
        ("query", "PostgreSQL permission query failed"),
        ("missing", "PostgreSQL psql executable not found"),
        ("startup", "PostgreSQL psql startup failed"),
        ("redis", "Redis rejected prerequisite PING"),
        ("redis-url", "Redis URL validation failed"),
        ("redis-connection", "Redis connection failed"),
        ("timeout", "PostgreSQL permission query timed out"),
        (None, None),
    ])
    def test_service_checks_use_inherited_auth_without_logging_secrets(self, failure, expected, capsys):
        env = {
            "SIPPY_DATABASE_DSN": "postgresql://user:private@localhost/postgres",
            "SIPPY_E2E_REDIS_URL": "redis://:private@localhost/1",
        }
        calls = []

        def run(command, **kwargs):
            calls.append((command, kwargs))
            if failure == "timeout":
                raise subprocess.TimeoutExpired(command, 15)
            if failure == "missing":
                raise FileNotFoundError("private")
            if failure == "startup":
                raise PermissionError("private")
            code = 2 if failure == "postgres" else 1 if failure == "query" else 0
            output = "f\n" if failure == "permission" else "t\n"
            return subprocess.CompletedProcess(command, code, output, "private")

        with (
            mock.patch.dict(os.environ, env),
            mock.patch("testenv.socket.socket"),
            mock.patch("testenv.Path.exists", return_value=True),
            mock.patch("testenv.subprocess.run", side_effect=run),
            mock.patch("testenv.redis_ping", side_effect={
                "redis": testenv.RedisCommandError("Redis rejected prerequisite PING"),
                "redis-url": ValueError("SIPPY_E2E_REDIS_URL has a malformed authority, port or database"),
                "redis-connection": testenv.RedisPrerequisiteError("Redis connection failed"),
            }.get(failure)) as ping,
        ):
            if failure:
                with pytest.raises(RuntimeError, match=expected) as error:
                    testenv.check_prerequisites()
                assert type(error.value) is RuntimeError
                assert "private" not in str(error.value)
            else:
                testenv.check_prerequisites()
        assert env["SIPPY_DATABASE_DSN"] not in calls[0][0]
        assert all("private" not in arg for arg in calls[0][0])
        assert calls[0][1]["env"]["PGPASSWORD"] == "private"
        assert calls[0][1]["env"]["PGUSER"] == "user"
        assert calls[0][1]["env"]["PGHOST"] == "localhost"
        assert calls[0][1]["env"]["PGDATABASE"] == "postgres"
        assert "-w" in calls[0][0]
        if failure in ("redis", "redis-url", "redis-connection", None):
            ping.assert_called_once_with(env["SIPPY_E2E_REDIS_URL"])
        assert "private" not in capsys.readouterr().out

    def test_make_runs_in_the_validated_child_environment(self, tmp_path, monkeypatch):
        credential = secrets.token_urlsafe(32)
        # sitecustomize also runs in the real child, independent of the host marker.
        (tmp_path / "sitecustomize.py").write_text('''
from pathlib import Path
original_exists = Path.exists
Path.exists = lambda path: str(path) == "/run/.containerenv" or original_exists(path)
''')
        monkeypatch.setenv("PYTHONPATH", str(tmp_path))
        bin_dir = tmp_path / "bin"
        bin_dir.mkdir()
        for name, body in {
            "psql": '''
[ "$1" = "-X" ] || exit 90
[ "$PGUSER" = test ] && [ "$PGPASSWORD" = private ] || exit 95
[ "$PGHOST" = localhost ] && [ "$PGDATABASE" = postgres ] || exit 96
[ -z "${PGHOSTADDR:-}${PGSERVICE:-}" ] || exit 97
printf "t\\n"
''',
            "make": '''
[ "$1" = e2e ] || exit 91
[ "$SIPPY_DATABASE_DSN" = "$SIPPY_SEED_DATABASE_DSN" ] || exit 92
[ "$SIPPY_E2E_REDIS_URL" = "$EXPECTED_REDIS_URL" ] || exit 93
for directory in "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR" "$TMPDIR" "$ARTIFACT_DIR" "$SIPPY_TEST_OUTPUT_DIR"; do
    touch "$directory/child-writable" || exit 94
done
echo "validated child ran make e2e"
''',
        }.items():
            stub = bin_dir / name
            stub.write_text("#!/bin/sh\n" + body.replace("private", credential))
            stub.chmod(0o755)
        monkeypatch.setenv("PATH", f"{bin_dir}:{os.environ['PATH']}")
        monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))
        for key in ("GOCACHE", "GOMODCACHE", "GOTMPDIR", "TMPDIR", "XDG_CACHE_HOME",
                    "npm_config_cache", "PLAYWRIGHT_BROWSERS_PATH"):
            monkeypatch.setenv(key, str(tmp_path / "cache" / key))
        monkeypatch.setenv("SIPPY_SEED_DATABASE_DSN", (
            f"postgresql://test:{credential}@localhost/postgres"
            "?ho%73t=other&hostaddr=127.0.0.2&dbname=other&service=other"
        ))
        monkeypatch.setenv("PGHOSTADDR", "127.0.0.2")
        monkeypatch.setenv("PGSERVICE", "other")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            listener.settimeout(5)
            port = listener.getsockname()[1]
            url = f"redis://localhost:{port}/7"
            monkeypatch.setenv("SIPPY_E2E_REDIS_URL", url)
            monkeypatch.setenv("EXPECTED_REDIS_URL", url)
            monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "private/var/logs")
            commands = []

            def respond():
                with listener.accept()[0] as connection, connection.makefile("rb") as reader:
                    for _ in range(2):
                        count = int(reader.readline()[1:])
                        command = []
                        for _ in range(count):
                            size = int(reader.readline()[1:])
                            command.append(reader.read(size + 2)[:-2])
                        commands.append(command)
                        connection.sendall(b"+PONG\r\n" if command == [b"PING"] else b"+OK\r\n")

            thread = threading.Thread(target=respond, daemon=True)
            thread.start()
            result = asyncio.run(run_e2e())
            thread.join(timeout=5)
            assert not thread.is_alive()
        assert commands == [[b"SELECT", b"7"], [b"PING"]]
        assert "run_e2e succeeded" in result, result
        assert "validated child ran make e2e" in result
        assert credential not in (tmp_path / "private/var/logs/run_e2e.log").read_text()
        assert (tmp_path / "artifacts/e2e/child-writable").exists()


class TestE2eFailurePaths:
    @pytest.mark.parametrize("arguments", [[], ["unknown"], ["validate", "extra"]])
    def test_uri_cli_rejects_invalid_commands(self, arguments):
        result = subprocess.run(
            [sys.executable, str(REPO_ROOT / "mcp/serviceurl.py"), *arguments],
            capture_output=True, text=True,
        )
        assert result.returncode == 2
        assert "usage:" in result.stderr
        assert "Traceback" not in result.stderr

    @pytest.mark.parametrize("uri,reason", [
        ("postgresql://user:private@remote/db", "Non-local PostgreSQL URI"),
        ("postgresql://user:private@localhost:notaport/db", "Malformed PostgreSQL URI port"),
        ("postgresql://user:private@[invalid/db", "Malformed PostgreSQL URI authority"),
        ("https://user:private@localhost/db", "Unsupported PostgreSQL URI scheme"),
        ("postgresql://localhost/db?password=private&host=other", "overrides the e2e connection target"),
    ])
    def test_uri_cli_reports_safe_validation_reason(self, uri, reason):
        with pytest.raises(ValueError, match=reason):
            serviceurl.local_database_uri(uri)
        result = subprocess.run(
            [sys.executable, str(REPO_ROOT / "mcp/serviceurl.py"), "validate"],
            env={**os.environ, "SIPPY_DATABASE_DSN": uri}, capture_output=True, text=True,
        )
        assert result.returncode == 1
        assert reason in result.stderr
        assert "private" not in result.stdout + result.stderr

    @pytest.mark.parametrize("key", ["host", "hostaddr", "dbname", "ho%73t", "db%6eame", "service"])
    def test_database_overrides_cannot_redirect_destructive_calls(self, key):
        result = TestE2eScriptSafety._run_helper('''
psql() { [ "$1" = "postgresql://user:test@localhost:5432/postgres?sslmode=disable" ] || exit 90; }
e2e_configure_existing_services || exit $?
[ "$SIPPY_E2E_DSN" = "postgresql://user:test@localhost:5432/sippy_e2e?sslmode=disable" ] || exit 91
[ -z "${PGHOSTADDR:-}${PGSERVICE:-}" ]
''', {"SIPPY_DATABASE_DSN": f"postgresql://user:test@localhost:5432/postgres?{key}=remote&sslmode=disable",
      "PGHOSTADDR": "127.0.0.2", "PGSERVICE": "remote"})
        assert result.returncode == 0, result.stderr
        result = TestE2eScriptSafety._run_helper('''
psql() { exit 90; }
e2e_create_database sippy_e2e "$SIPPY_DATABASE_DSN"
''', {"SIPPY_DATABASE_DSN": f"postgresql://user:test@localhost:5432/postgres?{key}=remote"})
        assert result.returncode == 1, result.stderr

    @pytest.mark.parametrize("key", ["pass%77ord", "%70assword", "%73slpassword", "sslpass%77ord"])
    def test_encoded_credential_keys_are_redacted(self, key):
        credential = secrets.token_urlsafe(32)
        result = TestE2eScriptSafety._run_helper(
            'printf "%s\\n" "$TEST_LOG" | e2e_redact_logs',
            {"TEST_LOG": f"postgresql://localhost/db?{key}={credential}&sslmode=disable"},
        )
        assert result.returncode == 0
        assert result.stdout.strip() == f"postgresql://localhost/db?{key}=[redacted]&sslmode=disable"

    @pytest.mark.parametrize(
        "stage,status,expected,forbidden",
        [
            ("drop", 31, "Database setup failed", "Building sippy"),
            ("create", 32, "Database setup failed", "Building sippy"),
            ("seed", 33, "Database seeding failed (exit 33)", "Waiting for sippy API"),
            ("serve", 34, "API exited before readiness (exit 34)", "Priming component"),
            ("serve", 0, "API exited before readiness (exit 0)", "Priming component"),
            ("signal-reader-pid", 143, "Injected startup signal", "Priming component"),
            ("signal-reader", 143, "Injected startup signal", "Priming component"),
            ("signal-writer-pid", 143, "Injected startup signal", "Priming component"),
            ("health", 1, "Timeout waiting for sippy API", "Priming component"),
            ("tests", 35, "Cache priming complete", "API exited before readiness"),
            ("tee", 36, "Cache priming complete", "API exited before readiness"),
            ("success", 0, "Cache priming complete", "API exited before readiness"),
        ],
    )
    def test_failure_status_survives_cleanup(self, tmp_path, stage, status, expected, forbidden):
        """Execute the entry point with failing commands, including failing cleanup."""
        bash_env = tmp_path / "bash-env"
        bash_env.write_text('''
function [ {
    if [[ "$1" == "-f" && "$2" == "/run/.containerenv" ]]; then return 0; fi
    builtin [ "$@"
}
python3() {
    if [[ "$3" == redact ]]; then /bin/sleep 0.1; fi
    command python3 "$@"
}
mkfifo() {
    # Model virtiofs: artifacts and TMPDIR can hold files, but not named pipes.
    case "$1" in
        /tmp/sippy-api-log.*/api.fifo) ;;
        *) echo "FIFO on unsupported workspace filesystem" >&2; return 1 ;;
    esac
    echo "$1" > "$SIPPY_TEST_OUTPUT_DIR/fifo-path"
    command mkfifo "$@"
}
psql() {
    if [[ "$*" == *"CREATE DATABASE"* && "$FAIL_STAGE" == create ]]; then return "$FAIL_STATUS"; fi
    if [[ "$*" == *"DROP DATABASE IF"* && "$FAIL_STAGE" == drop ]]; then return "$FAIL_STATUS"; fi
    # Fail cleanup to ensure it cannot mask the original status.
    if [[ "$*" == *"DROP DATABASE sippy_e2e"* ]]; then return 87; fi
}
go() {
    if [[ "$1" == tool ]]; then return 88; fi
    return 0
}
curl() {
    if [[ "$*" == *"/api/capabilities"* ]]; then
        if [[ "$FAIL_STAGE" == health ]]; then SECONDS=$((SECONDS + 600)); fi
        if [[ "$FAIL_STAGE" == serve || "$FAIL_STAGE" == health ]]; then return 22; fi
        return 0
    fi
    if [[ "$*" == *"/api/component_readiness/views"* ]]; then
        echo '{"views":[]}'
        return 0
    fi
    return 96
}
sleep() { if [[ "$FAIL_STAGE" != health ]]; then /bin/sleep 0.02; fi; }
gotestsum() {
    echo "test failure postgresql://test:secret@localhost/postgres?password=secret&sslmode=disable" >&2
    echo "DONE test stub"
    if [[ "$FAIL_STAGE" == tests ]]; then return "$FAIL_STATUS"; fi
}
tee() {
    if [[ "$FAIL_STAGE" == tee && "$1" == *e2e-gotestsum.log ]]; then
        cat >/dev/null
        return "$FAIL_STATUS"
    fi
    command tee "$@"
}
inject_startup_signal() {
    if [[ "$FAIL_STAGE" == signal-reader-pid && "$BASH_COMMAND" == 'API_LOG_PID=$!' ]] ||
       [[ "$FAIL_STAGE" == signal-reader && -n "${API_LOG_PID:-}" && -z "${CHILD_PID:-}" ]] ||
       [[ "$FAIL_STAGE" == signal-writer-pid && "$BASH_COMMAND" == 'CHILD_PID=$!' ]]; then
        trap - DEBUG
        echo "$!" > "$SIPPY_TEST_OUTPUT_DIR/signalled-child.pid"
        if [[ "$FAIL_STAGE" == signal-writer-pid ]]; then
            for ((attempt=0; attempt<500; attempt++)); do
                [[ -f "$SIPPY_TEST_OUTPUT_DIR/writer-started" ]] && break
                /bin/sleep 0.01
            done
        fi
        echo "Injected startup signal" >&2
        kill -TERM "$$"
    fi
}
trap 'inject_startup_signal' DEBUG
''')
        sippy = tmp_path / "sippy"
        sippy.write_text('''#!/bin/bash
if [[ "$1" == seed-data ]]; then
    touch "$GOCOVERDIR/covcounters.test"
    if [[ "$FAIL_STAGE" == seed ]]; then
        echo "seed refused postgresql://test:secret@localhost/postgres" >&2
        exit "$FAIL_STATUS"
    fi
    exit 0
fi
if [[ "$FAIL_STAGE" == serve ]]; then
    echo "API startup failed redis://:secret@localhost/1" >&2
    exit "$FAIL_STATUS"
fi
if [[ "$FAIL_STAGE" == signal-writer-pid ]]; then
    echo "API writer final line redis://:secret@localhost/1" >&2
    touch "$SIPPY_TEST_OUTPUT_DIR/writer-started"
fi
exec /bin/sleep 60
''')
        sippy.chmod(0o755)
        output_dir = tmp_path / "artifacts"
        output_dir.mkdir()
        (output_dir / "e2e-gotestsum.log").write_text("DONE previous run\n")
        proc = subprocess.Popen(
            ["bash", str(E2E_SCRIPT)], cwd=tmp_path,
            env={**os.environ, "BASH_ENV": str(bash_env), "FAIL_STAGE": stage,
                 "FAIL_STATUS": str(status), "GCS_SA_JSON_PATH": "",
                 "SIPPY_TEST_OUTPUT_DIR": str(output_dir),
                 "SIPPY_DATABASE_DSN": "postgresql://test:secret@localhost:5432/postgres?sslmode=disable"},
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True,
        )
        try:
            stdout, stderr = proc.communicate(timeout=10)
            if stage.startswith("signal-"):
                child_pid = int((output_dir / "signalled-child.pid").read_text())
                with pytest.raises(ProcessLookupError):
                    os.kill(child_pid, 0)
        finally:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.communicate()
        output = stdout + stderr
        assert proc.returncode == (status or (1 if stage == "serve" else 0)), output
        assert expected in output
        assert forbidden not in output
        assert "DONE previous run" not in output
        if stage in ("tests", "success"):
            assert "DONE test stub" in output
        assert "secret" not in output
        for log in output_dir.glob("*.log"):
            assert "secret" not in log.read_text()
        assert not list(output_dir.glob("api-log.*"))
        fifo_path = output_dir / "fifo-path"
        if fifo_path.exists():
            assert not Path(fifo_path.read_text().strip()).parent.exists()
        if stage == "serve":
            assert "API startup failed" in output
        if stage == "signal-writer-pid":
            assert (output_dir / "writer-started").exists()
            assert "API writer final line redis://[redacted]@localhost/1" in (output_dir / "e2e.log").read_text()

    @pytest.mark.parametrize("path", ["", "/", "/postgres"])
    @pytest.mark.parametrize("query", ["", "?sslmode=disable&connect_timeout=5"])
    def test_dsn_connection_options_survive_database_replacement(self, path, query):
        result = TestE2eScriptSafety._run_helper('''
psql() { return 0; }
e2e_configure_existing_services
[ "$SIPPY_E2E_DSN" = "$EXPECTED_DSN" ]
''', {"SIPPY_DATABASE_DSN": f"postgresql://user:private@localhost:5432{path}{query}",
      "EXPECTED_DSN": f"postgresql://user:private@localhost:5432/sippy_e2e{query}"})
        assert result.returncode == 0, result.stderr

    @pytest.mark.parametrize("scheme", ["postgres", "postgresql", "redis", "rediss"])
    def test_log_redaction_covers_userinfo_and_query_credentials(self, scheme):
        source = f'{scheme}://user:private@localhost/db?password=query-secret&sslmode=disable&sslpassword=key-secret&password=second-secret'
        result = TestE2eScriptSafety._run_helper(
            'printf "%s\\n" "$TEST_LOG" | e2e_redact_logs', {"TEST_LOG": source}
        )
        assert result.returncode == 0
        assert result.stdout.strip() == (
            f'{scheme}://[redacted]@localhost/db?password=[redacted]&sslmode=disable&sslpassword=[redacted]&password=[redacted]'
        )

    def test_readiness_deadline_includes_request_time(self):
        result = TestE2eScriptSafety._run_helper('''
SECONDS=0
CHILD_PID=$$
SIPPY_API_PORT=18080
calls=0
curl() {
    calls=$((calls + 1))
    [ "$calls" -le 150 ] || exit 99
    SECONDS=$((SECONDS + 2))
    return 28
}
sleep() { SECONDS=$((SECONDS + $1)); }
e2e_log_tail() { :; }
wait_for_sippy
status=$?
[ "$status" -eq 1 ] && [ "$SECONDS" -eq 600 ] && [ "$calls" -eq 150 ]
''')
        assert result.returncode == 0, result.stdout + result.stderr
        assert "after 600s" in result.stderr

    def test_readiness_sleep_is_limited_to_remaining_time(self):
        result = TestE2eScriptSafety._run_helper('''
SECONDS=0
CHILD_PID=$$
SIPPY_API_PORT=18080
curl() { SECONDS=599; return 28; }
sleep() {
    [ "$1" -eq 1 ] || exit 99
    SECONDS=$((SECONDS + $1))
}
e2e_log_tail() { :; }
wait_for_sippy
status=$?
[ "$status" -eq 1 ] && [ "$SECONDS" -eq 600 ]
''')
        assert result.returncode == 0, result.stdout + result.stderr


class TestRedisProbe:
    @pytest.mark.parametrize("error,expected", [
        (ConnectionRefusedError("private"), "Redis connection failed"),
        (TimeoutError("private"), "Redis connection timed out"),
    ])
    def test_connection_diagnostics_do_not_expose_socket_errors(self, error, expected):
        with mock.patch("testenv.socket.create_connection", side_effect=error):
            with pytest.raises(testenv.RedisPrerequisiteError, match=expected) as caught:
                testenv.redis_ping("redis://:private@localhost/1")
        assert "private" not in str(caught.value)

    @pytest.mark.parametrize("command", ["AUTH", "SELECT", "PING"])
    @pytest.mark.parametrize("error,suffix", [(TimeoutError("private"), "timed out"), (OSError("private"), "transport failed")])
    def test_transport_diagnostics_name_the_failed_operation(self, command, error, suffix):
        connection = mock.MagicMock()
        connection.sendall.side_effect = [None] * ["AUTH", "SELECT", "PING"].index(command) + [error]
        connection.makefile.return_value.__enter__.return_value.readline.return_value = b"+OK\r\n"
        with mock.patch("testenv.socket.create_connection", return_value=connection):
            with pytest.raises(testenv.RedisPrerequisiteError, match=f"{command} {suffix}") as caught:
                testenv.redis_ping("redis://:private@localhost/1")
        assert "private" not in str(caught.value)
        connection.close.assert_called_once()

    def test_tls_diagnostics_preserve_cleanup_and_hide_socket_error(self):
        connection = mock.Mock()
        with (
            mock.patch("testenv.socket.create_connection", return_value=connection),
            mock.patch("testenv.ssl.create_default_context") as context,
        ):
            context.return_value.wrap_socket.side_effect = OSError("private")
            with pytest.raises(testenv.RedisPrerequisiteError, match="TLS handshake failed") as caught:
                testenv.redis_ping("rediss://:private@localhost/1")
        assert "private" not in str(caught.value)
        connection.close.assert_called_once()

    @pytest.mark.parametrize("url", ["redis://localhost/1?db=0", "redis://localhost/1?%64b=0"])
    def test_explicit_query_options_fail_before_connecting(self, url):
        with mock.patch("testenv.socket.create_connection") as connect:
            with pytest.raises(ValueError, match="query options"):
                testenv.redis_ping(url)
            connect.assert_not_called()

    @pytest.mark.parametrize("failed_command", ["AUTH", "SELECT", "PING"])
    def test_rejected_operation_is_named_without_redis_reply(self, failed_command):
        credential = secrets.token_urlsafe(32)
        commands = ["AUTH", "SELECT", "PING"]
        replies = b"+OK\r\n" * commands.index(failed_command) + f"-ERR {credential}\r\n".encode()
        client, peer = socket.socketpair()
        with peer, mock.patch("testenv.socket.create_connection", return_value=client):
            peer.sendall(replies)
            with pytest.raises(testenv.RedisCommandError, match=failed_command) as error:
                testenv.redis_ping(f"redis://:{credential}@localhost/1")
            assert credential not in str(error.value)

    @pytest.mark.parametrize(
        "url,responses,expected",
        [
            ("redis://localhost", b"+PONG\r\n", [[b"PING"]]),
            ("redis://:a%40b@localhost/1", b"+OK\r\n+OK\r\n+PONG\r\n",
             [[b"AUTH", b"a@b"], [b"SELECT", b"1"], [b"PING"]]),
            ("rediss://alice:private@localhost/2", b"+OK\r\n+OK\r\n+PONG\r\n",
             [[b"AUTH", b"private"], [b"SELECT", b"2"], [b"PING"]]),
        ],
    )
    def test_auth_database_and_tls(self, url, responses, expected):
        client, peer = socket.socketpair()
        with peer, mock.patch("testenv.socket.create_connection", return_value=client):
            with mock.patch("testenv.ssl.create_default_context") as tls:
                tls.return_value.wrap_socket.return_value = client
                peer.sendall(responses)
                testenv.redis_ping(url)
                payload = peer.recv(4096)
                expected_payload = b"".join(
                    f"*{len(command)}\r\n".encode() + b"".join(
                        f"${len(part)}\r\n".encode() + part + b"\r\n" for part in command
                    ) for command in expected
                )
                assert payload == expected_payload
                if url.startswith("rediss:"):
                    tls.return_value.wrap_socket.assert_called_once_with(client, server_hostname="localhost")
                else:
                    tls.assert_not_called()
        assert client.fileno() == -1

    @pytest.mark.parametrize("response", [b"-WRONGPASS private\r\n", b"-ERR private\r\n", b""])
    def test_failure_does_not_expose_reply(self, response):
        client, peer = socket.socketpair()
        with peer, mock.patch("testenv.socket.create_connection", return_value=client):
            peer.sendall(response)
            peer.shutdown(socket.SHUT_WR)
            with pytest.raises(RuntimeError, match="Redis rejected") as error:
                testenv.redis_ping("redis://localhost")
            assert "private" not in str(error.value)
        assert client.fileno() == -1

    @pytest.mark.parametrize("url", ["https://localhost/1", "redis:///1", "redis://localhost/invalid"])
    def test_invalid_url(self, url):
        with pytest.raises(ValueError):
            testenv.redis_ping(url)


class TestMakeTools:
    @pytest.mark.parametrize("target,tool", [("lint", "run_lint"), ("test", "run_tests")])
    def test_runner_only_prepares_settings_needed_by_its_target(self, tmp_path, monkeypatch, target, tool):
        monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
        monkeypatch.setenv("REDIS_URL", "redis://[malformed")
        monkeypatch.delenv("SIPPY_E2E_REDIS_URL", raising=False)
        monkeypatch.setenv("SIPPY_DATABASE_DSN", "postgresql://legacy/db")
        monkeypatch.setenv("SIPPY_SEED_DATABASE_DSN", "postgresql://seed/db")
        monkeypatch.setenv("ARTIFACT_DIR", (
            "/proc/sippy-read-only-artifacts" if target == "lint" else str(tmp_path / "artifacts")
        ))
        create = mock.AsyncMock(return_value=FakeProcess())
        monkeypatch.setattr(server.asyncio, "create_subprocess_exec", create)
        result = asyncio.run(getattr(server, tool)())
        assert "succeeded" in result
        env = create.call_args.kwargs["env"]
        assert env["SIPPY_DATABASE_DSN"] == "postgresql://legacy/db"
        assert "SIPPY_E2E_REDIS_URL" not in env
        assert create.call_args.args[2] == target

    @pytest.mark.parametrize("target,grace", [("e2e", 60), ("lint", 5), ("test", 5)])
    def test_cancellation_uses_target_cleanup_grace(self, tmp_path, monkeypatch, target, grace):
        async def run():
            started = asyncio.Event()

            async def wait():
                started.set()
                await asyncio.Event().wait()

            proc = mock.Mock()
            proc.wait = wait
            create = mock.AsyncMock(return_value=proc)
            cleanup = mock.AsyncMock()
            monkeypatch.setattr(server.asyncio, "create_subprocess_exec", create)
            monkeypatch.setattr(server, "_stop_make_process", cleanup)
            monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
            monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))
            task = asyncio.create_task(server._run_make_phase(target, target, "phase.log", 0))
            await started.wait()
            task.cancel()
            with pytest.raises(asyncio.CancelledError):
                await task
            cleanup.assert_awaited_once_with(proc, grace)

        asyncio.run(run())

    @pytest.mark.parametrize("leader_ignores_term", [False, True])
    @pytest.mark.parametrize("stop", ["timeout", "cancel"])
    def test_stopping_releases_resources_held_by_grandchildren(self, tmp_path, monkeypatch, leader_ignores_term, stop):
        lock_path = tmp_path / "grandchild.lock"
        ready_path = tmp_path / "grandchild.pid"
        child = f'''
import fcntl
import os
import signal
import time
from pathlib import Path
signal.signal(signal.SIGTERM, signal.SIG_IGN)
lock = open({str(lock_path)!r}, "w")
fcntl.flock(lock, fcntl.LOCK_EX)
Path({str(ready_path)!r}).write_text(str(os.getpid()))
while True:
    time.sleep(1)
'''
        make = tmp_path / "make"
        make.write_text(f'''#!{sys.executable}
import signal
import subprocess
import sys
if {leader_ignores_term!r}:
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
subprocess.Popen([sys.executable, "-c", {child!r}]).wait()
''')
        make.chmod(0o755)
        monkeypatch.setenv("PATH", f"{tmp_path}:{os.environ['PATH']}")
        monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))
        monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
        try:
            async def run():
                task = asyncio.create_task(server._run_make_phase(
                    "run_lint", "lint", "run_lint.log", 2 if stop == "timeout" else 30,
                ))
                if stop == "cancel":
                    deadline = time.monotonic() + 5
                    while not ready_path.exists() and time.monotonic() < deadline:
                        await asyncio.sleep(0.01)
                    task.cancel()
                    with pytest.raises(asyncio.CancelledError):
                        await task
                else:
                    assert "timed out" in await task

            asyncio.run(run())
            assert ready_path.exists(), "grandchild must acquire its resource before the timeout"
            with lock_path.open("w") as lock:
                deadline = time.monotonic() + 2
                while True:
                    try:
                        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                        break
                    except BlockingIOError:
                        if time.monotonic() >= deadline:
                            pytest.fail("grandchild survived the runner timeout and retained its lock")
                        time.sleep(0.01)
        finally:
            if ready_path.exists():
                try:
                    os.kill(int(ready_path.read_text()), signal.SIGKILL)
                except ProcessLookupError:
                    pass

    @pytest.mark.parametrize("name,target", [("run_tests", "test"), ("run_lint", "lint")])
    def test_tools_use_shared_runner_and_publish_timeout(self, name, target):
        with mock.patch("server._run_make_phase", new=mock.AsyncMock(return_value="completed")) as phase:
            result = asyncio.run(getattr(server, name)(timeout_seconds=123))
        assert result == "completed"
        phase.assert_awaited_once_with(name, target, f"{name}.log", 123, None)
        tool = asyncio.run(mcp.get_tool(name))
        assert tool.parameters["properties"]["timeout_seconds"] == {"default": 7200, "type": "integer"}

    @pytest.mark.parametrize("target,listeners", [("test", 1), ("lint", 0)])
    def test_unit_and_lint_do_not_require_database_services(self, target, listeners):
        with (
            mock.patch("testenv.socket.socket") as listener,
            mock.patch("testenv.subprocess.run") as command,
            mock.patch("testenv.redis_ping") as ping,
        ):
            testenv.check_prerequisites(target)
        assert listener.call_count == listeners
        command.assert_not_called()
        ping.assert_not_called()


class TestMakeRunResults:
    @pytest.fixture(autouse=True)
    def isolate_runs(self, monkeypatch):
        monkeypatch.setattr(server, "_MAKE_RUNS", {})

    def test_poll_and_recover_after_request_cancellation(self, monkeypatch):
        async def run():
            started = asyncio.Event()
            finish = asyncio.Event()

            async def phase(*args):
                started.set()
                await finish.wait()
                return "run_tests succeeded (exit 0)"

            monkeypatch.setattr(server, "_run_make_phase", phase)
            request = asyncio.create_task(server.run_tests())
            await started.wait()
            request.cancel()
            with pytest.raises(asyncio.CancelledError):
                await request
            task = next(iter(server._MAKE_RUNS.values())).task
            assert not task.done()
            status = await server.get_make_result(wait_seconds=0)
            run_id = next(iter(server._MAKE_RUNS))
            assert run_id in status
            # A retry must recover the same run instead of starting another suite.
            assert "already active" in await server.run_tests(wait_seconds=0)
            assert len(server._MAKE_RUNS) == 1
            finish.set()
            assert "succeeded" in await server.get_make_result(run_id)
            assert "succeeded" in await server.get_make_result(run_id, wait_seconds=0)

        asyncio.run(run())

    def test_explicit_cancel_waits_for_cleanup(self, monkeypatch):
        async def run():
            started = asyncio.Event()
            cleanup = asyncio.Event()

            async def phase(*args):
                started.set()
                try:
                    await asyncio.Event().wait()
                except asyncio.CancelledError:
                    await cleanup.wait()
                    raise

            monkeypatch.setattr(server, "_run_make_phase", phase)
            await server.run_tests(wait_seconds=0)
            await started.wait()
            run_id = next(iter(server._MAKE_RUNS))
            assert "Cancellation requested" in await server.cancel_make_run(run_id)
            await asyncio.sleep(0)
            assert "running" in await server.get_make_result(run_id, wait_seconds=0)
            cleanup.set()
            assert "cleanup complete" in await server.get_make_result(run_id)

        asyncio.run(run())

    def test_unknown_run_and_bounded_history(self, monkeypatch):
        async def run():
            assert "No matching" in await server.get_make_result()
            assert "No matching" in await server.cancel_make_run("unknown")
            monkeypatch.setattr(server, "_run_make_phase", mock.AsyncMock(return_value="completed"))
            for _ in range(25):
                assert await server.run_lint() == "completed"
            assert len(server._MAKE_RUNS) == 20

        asyncio.run(run())

    def test_real_mcp_request_timeout_preserves_run(self, tmp_path):
        mcp_dir = tmp_path / "mcp"
        mcp_dir.mkdir()
        for filename in ("server.py", "testenv.py", "serviceurl.py"):
            shutil.copyfile(REPO_ROOT / "mcp" / filename, mcp_dir / filename)
        finish = tmp_path / "finish"
        make = tmp_path / "make"
        make.write_text(f'''#!{sys.executable}
import time
from pathlib import Path
deadline = time.monotonic() + 10
while not Path({str(finish)!r}).exists():
    if time.monotonic() > deadline:
        raise SystemExit(1)
    time.sleep(0.01)
print('suite complete')
''')
        make.chmod(0o755)

        async def run():
            transport = StdioTransport(
                command=sys.executable,
                args=[str(mcp_dir / "server.py")],
                cwd=str(tmp_path),
                env={**os.environ, "PATH": f"{tmp_path}:{os.environ['PATH']}"},
            )
            async with Client(transport) as client:
                with pytest.raises(Exception, match="(?i)timed out"):
                    await client.call_tool("run_lint", {}, timeout=0.2)
                result = await client.call_tool("get_make_result", {"wait_seconds": 0})
                assert "running" in result.content[0].text
                with pytest.raises(Exception, match="(?i)timed out"):
                    await client.call_tool("get_make_result", {}, timeout=0.05)
                finish.touch()
                result = await client.call_tool("get_make_result", {})
                assert "succeeded" in result.content[0].text
                assert "suite complete" in result.content[0].text

        asyncio.run(run())


class TestFrontendPrerequisites:
    @pytest.mark.parametrize("broken", [None, "rollup", "esbuild"])
    def test_loads_native_packages_in_node(self, tmp_path, broken):
        for name in ("rollup", "esbuild"):
            package = tmp_path / "sippy-ng/node_modules" / name
            package.mkdir(parents=True)
            (package / "index.js").write_text(
                "throw new Error('wrong platform')" if name == broken
                else "module.exports.transformSync = () => ({code: ''});"
            )
        if broken:
            with pytest.raises(RuntimeError, match="another OS/architecture"):
                testenv.check_frontend_dependencies(tmp_path)
        else:
            testenv.check_frontend_dependencies(tmp_path)

    def test_missing_modules_are_left_for_make(self, tmp_path):
        with mock.patch("testenv.subprocess.run") as run:
            testenv.check_frontend_dependencies(tmp_path)
        run.assert_not_called()

    @pytest.mark.parametrize("failure", [None, 1, FileNotFoundError(), subprocess.TimeoutExpired("node", 30)])
    def test_native_dependency_diagnostic(self, tmp_path, failure):
        (tmp_path / "sippy-ng/node_modules").mkdir(parents=True)
        with mock.patch("testenv.subprocess.run") as run:
            if isinstance(failure, Exception):
                run.side_effect = failure
            else:
                run.return_value.returncode = failure or 0
            if failure is None:
                testenv.check_frontend_dependencies(tmp_path)
            else:
                with pytest.raises(RuntimeError, match="Node|node_modules"):
                    testenv.check_frontend_dependencies(tmp_path)
            assert run.call_args.kwargs["cwd"] == tmp_path / "sippy-ng"


class TestCheckServicesRedis:
    @pytest.mark.parametrize("url", ["redis://:private@localhost/1", "rediss://user:private@[::1]:6380/2"])
    @pytest.mark.parametrize("failure", [None, testenv.RedisCommandError("Redis rejected AUTH")])
    def test_uses_shared_probe_without_redis_cli(self, monkeypatch, url, failure):
        monkeypatch.setenv("REDIS_URL", url)
        process = mock.Mock(returncode=0)
        process.communicate = mock.AsyncMock(return_value=(b"42", b""))
        create = mock.AsyncMock(return_value=process)
        ping = mock.Mock(side_effect=failure)
        monkeypatch.setattr(server.asyncio, "create_subprocess_exec", create)
        monkeypatch.setattr(server, "redis_ping", ping)
        result = asyncio.run(server.check_services())
        ping.assert_called_once_with(url, setting_name="REDIS_URL")
        assert all(call.args[0] == "psql" for call in create.call_args_list)
        assert "redis: OK" in result if failure is None else "Redis rejected AUTH" in result
        assert "private" not in result


class TestReviewRegressions:
    @pytest.mark.parametrize("scheme", ["postgres", "postgresql", "redis", "rediss"])
    def test_redactor_uses_last_userinfo_separator_and_preserves_invalid_bytes(self, scheme):
        source = (f"ok\n{scheme}://user:p@ss@localhost/db?password=private\n".encode()
                  + b"\xff bad\n" + f"{scheme}://user:tail@localhost/db\n".encode())
        result = subprocess.run(
            [sys.executable, str(REPO_ROOT / "mcp/serviceurl.py"), "redact"],
            input=source, capture_output=True,
        )
        assert result.returncode == 0, result.stderr
        assert result.stdout == (
            f"ok\n{scheme}://[redacted]@localhost/db?password=[redacted]\n".encode()
            + b"\xff bad\n" + f"{scheme}://[redacted]@localhost/db\n".encode()
        )

    @pytest.mark.parametrize("password", ["p%3Fss%40word", "p@ss"])
    def test_database_selection_uses_parsed_uri(self, password):
        uri = f"postgresql://user:{password}@localhost:5433/original?sslmode=require&connect_timeout=7"
        expected = f"postgresql://user:{password}@localhost:5433/sippy_e2e?sslmode=require&connect_timeout=7"
        assert serviceurl.e2e_database_uri(uri, "sippy_e2e") == expected
        result = TestE2eScriptSafety._run_helper('''
psql() { return 0; }
e2e_configure_existing_services || exit $?
[ "$SIPPY_E2E_DSN" = "$EXPECTED_DSN" ]
''', {"SIPPY_DATABASE_DSN": uri, "EXPECTED_DSN": expected})
        assert result.returncode == 0, result.stderr

    @pytest.mark.parametrize("stage", ["prepare", "directory", "log", "spawn"])
    def test_launch_errors_identify_path_and_allow_next_run(self, tmp_path, monkeypatch, stage):
        monkeypatch.setattr(server, "_MAKE_RUNS", {})
        monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
        monkeypatch.setattr(server, "prepare_environment", mock.Mock(return_value=os.environ.copy()))
        monkeypatch.setattr(server.asyncio, "create_subprocess_exec", mock.AsyncMock(return_value=FakeProcess()))
        target = {
            "prepare": "server.prepare_environment", "directory": "server._ensure_dev_log_dir",
            "log": "builtins.open", "spawn": "server.asyncio.create_subprocess_exec",
        }[stage]
        async def run():
            with mock.patch(target, side_effect=PermissionError(13, "private diagnostic", "failed-path")):
                result = await server.run_lint()
            assert "failed-path" in result
            assert "private diagnostic" not in result
            assert ("cannot prepare test filesystem paths" if stage in ("prepare", "directory") else "failed to start") in result
            assert "succeeded" in await server.run_lint()
        asyncio.run(run())

    @pytest.mark.parametrize("stage", ["term", "poll", "kill"])
    def test_macos_exited_process_group_is_reaped(self, stage, monkeypatch):
        proc = mock.Mock(pid=1234)
        proc.wait = mock.AsyncMock(return_value=0)
        calls = []
        def killpg(pid, sig):
            calls.append(sig)
            if sig == {"term": signal.SIGTERM, "poll": 0, "kill": signal.SIGKILL}[stage]:
                raise PermissionError(1, "Operation not permitted")
            if sig == 0:
                raise ProcessLookupError
        monkeypatch.setattr(server.os, "killpg", killpg)
        asyncio.run(server._stop_make_process(proc, 1))
        proc.wait.assert_awaited_once()
        assert signal.SIGTERM in calls
        if stage != "term":
            assert 0 in calls
            assert signal.SIGKILL in calls

    @pytest.mark.parametrize("requested,expected", [(600, 30), (-5, 0)])
    def test_poll_wait_is_bounded_without_cancelling_run(self, monkeypatch, requested, expected):
        async def run():
            task = asyncio.create_task(asyncio.Event().wait())
            monkeypatch.setattr(server, "_MAKE_RUNS", {"test": server._MakeRun(task, "lint", {})})
            try:
                with mock.patch("server.asyncio.wait_for", new=mock.AsyncMock(side_effect=asyncio.TimeoutError)) as wait:
                    result = await server.get_make_result("test", wait_seconds=requested)
                assert "running" in result
                assert wait.call_args.kwargs["timeout"] == expected
                assert not task.done()
            finally:
                task.cancel()
                await asyncio.gather(task, return_exceptions=True)
        asyncio.run(run())

    @pytest.mark.parametrize("different_target", [True, False])
    def test_conflicting_target_or_bigquery_request_is_not_started(self, tmp_path, monkeypatch, different_target):
        async def run():
            monkeypatch.setattr(server, "_MAKE_RUNS", {})
            phase = mock.AsyncMock(side_effect=lambda *args: None)
            finish = asyncio.Event()
            async def active(*args):
                await finish.wait()
                return "original completed"
            phase.side_effect = active
            monkeypatch.setattr(server, "_run_make_phase", phase)
            creds = tmp_path / "credentials.json"
            creds.write_text('{}')
            await server.run_e2e(wait_seconds=0)
            original_id = next(iter(server._MAKE_RUNS))
            if different_target:
                result = await server.run_lint(wait_seconds=0)
            else:
                result = await server.run_e2e(str(creds), include_bigquery=True, wait_seconds=0)
            assert "was not started" in result
            assert original_id in result
            assert "make e2e" in result
            assert "does not validate this request" in result
            assert len(server._MAKE_RUNS) == 1
            finish.set()
            assert await server.get_make_result(original_id) == "original completed"
            assert phase.await_count == 1
            result = await server.run_lint() if different_target else await server.run_e2e(str(creds), include_bigquery=True)
            assert result == "original completed"
            assert phase.await_count == 2
            if not different_target:
                assert phase.call_args.args[-1] == {"GCS_SA_JSON_PATH": str(creds)}
        asyncio.run(run())

    @pytest.mark.parametrize("timeout", ["3", "27"])
    @pytest.mark.parametrize("source", ["uri", "environment"])
    def test_postgres_timeout_is_preserved_in_probe_and_deadline(self, source, timeout):
        parent = {"SIPPY_DATABASE_DSN": "postgresql://localhost/postgres", "SIPPY_E2E_REDIS_URL": "redis://localhost/1"}
        if source == "uri":
            parent["SIPPY_DATABASE_DSN"] += "?connect_timeout=" + timeout
            parent["PGCONNECT_TIMEOUT"] = "99"
        else:
            parent["PGCONNECT_TIMEOUT"] = timeout
        with (
            mock.patch.dict(os.environ, parent, clear=True),
            mock.patch("testenv.socket.socket"),
            mock.patch("testenv.Path.exists", return_value=True),
            mock.patch("testenv.subprocess.run", return_value=subprocess.CompletedProcess([], 0, "t\n")) as run,
            mock.patch("testenv.redis_ping"),
        ):
            testenv.check_prerequisites()
        assert run.call_args.kwargs["env"]["PGCONNECT_TIMEOUT"] == timeout
        assert run.call_args.kwargs["timeout"] == int(timeout) + 5

    @pytest.mark.parametrize("timeout", ["0", "-1", "private"])
    def test_unbounded_or_invalid_postgres_timeouts_are_rejected(self, timeout):
        with pytest.raises(ValueError, match="positive integer") as exc:
            testenv.postgres_environment({"SIPPY_DATABASE_DSN": "postgresql://localhost/db?connect_timeout=" + timeout})
        assert "private" not in str(exc.value)

    def test_default_go_and_system_caches_are_reused(self, tmp_path, monkeypatch):
        go_cache, go_mod = tmp_path / "go-build", tmp_path / "go-mod"
        temporary = tmp_path / "system-tmp"
        monkeypatch.setattr(testenv.tempfile, "gettempdir", lambda: str(temporary))
        command = mock.Mock(return_value=subprocess.CompletedProcess([], 0, f"{go_cache}\n{go_mod}\n"))
        monkeypatch.setattr(testenv.subprocess, "run", command)
        env = testenv.prepare_environment({"HOME": str(tmp_path)}, tmp_path / "logs", "lint")
        assert env["GOCACHE"] == str(go_cache)
        assert env["GOMODCACHE"] == str(go_mod)
        assert env["TMPDIR"] == env["GOTMPDIR"] == str(temporary)
        assert env["npm_config_cache"] == str(tmp_path / ".npm")
        command.assert_called_once()
        assert command.call_args.args[0] == ["go", "env", "GOCACHE", "GOMODCACHE"]

    @pytest.mark.parametrize("failure", [PermissionError(), subprocess.TimeoutExpired('go', 10), None])
    def test_unavailable_go_defaults_use_writable_fallback(self, tmp_path, monkeypatch, failure):
        command = mock.Mock(side_effect=failure, return_value=subprocess.CompletedProcess([], 1, ""))
        monkeypatch.setattr(testenv.subprocess, "run", command)
        env = testenv.prepare_environment({"HOME": str(tmp_path)}, tmp_path / "logs", "lint")
        for key in ("GOCACHE", "GOMODCACHE"):
            assert env[key] == str(tmp_path / "logs/test-cache" / key.lower())

    @pytest.mark.parametrize("setting", ["REDIS_URL", "SIPPY_E2E_REDIS_URL"])
    def test_redis_validation_names_the_callers_setting(self, setting):
        with pytest.raises(ValueError, match=setting):
            testenv.redis_ping("redis://[malformed", setting_name=setting)

    @pytest.mark.parametrize("shutdown_signal", [signal.SIGTERM, signal.SIGHUP])
    def test_server_shutdown_releases_detached_descendant_resources(self, tmp_path, shutdown_signal):
        mcp_dir = tmp_path / "mcp"
        mcp_dir.mkdir()
        for filename in ("server.py", "testenv.py", "serviceurl.py"):
            shutil.copyfile(REPO_ROOT / "mcp" / filename, mcp_dir / filename)
        pidfile = tmp_path / "server.pid"
        ready = tmp_path / "descendant.pid"
        resource = tmp_path / "resource.lock"
        wrapper = tmp_path / "serve.py"
        wrapper.write_text(f'''import os, runpy, sys
from pathlib import Path
Path({str(pidfile)!r}).write_text(str(os.getpid()))
sys.path.insert(0, {str(mcp_dir)!r})
runpy.run_path({str(mcp_dir / 'server.py')!r}, run_name='__main__')
''')
        descendant = f'''import fcntl, os, signal, time
from pathlib import Path
signal.signal(signal.SIGTERM, signal.SIG_IGN)
lock = open({str(resource)!r}, 'w')
fcntl.flock(lock, fcntl.LOCK_EX)
Path({str(ready)!r}).write_text(str(os.getpid()))
while True: time.sleep(1)
'''
        make = tmp_path / "make"
        make.write_text(f'''#!{sys.executable}
import subprocess, sys
subprocess.Popen([sys.executable, '-c', {descendant!r}]).wait()
''')
        make.chmod(0o755)
        async def run():
            transport = StdioTransport(
                command=sys.executable, args=[str(wrapper)], cwd=str(tmp_path),
                env={**os.environ, "PATH": f"{tmp_path}:{os.environ['PATH']}"},
            )
            async with Client(transport) as client:
                result = await client.call_tool("run_lint", {"wait_seconds": 0})
                assert "running" in result.content[0].text
                deadline = time.monotonic() + 10
                while not ready.exists() and time.monotonic() < deadline:
                    await asyncio.sleep(0.01)
                assert ready.exists()
                os.kill(int(pidfile.read_text()), shutdown_signal)
                with resource.open("w") as lock:
                    while True:
                        try:
                            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                            break
                        except BlockingIOError:
                            assert time.monotonic() < deadline, "server left its detached descendant alive"
                            await asyncio.sleep(0.02)
        try:
            asyncio.run(run())
        finally:
            for path in (ready, pidfile):
                if path.exists():
                    try:
                        os.kill(int(path.read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass
