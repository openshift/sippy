import asyncio
import inspect
import os
import secrets
import shutil
import socket
import threading
import subprocess
import tempfile
from pathlib import Path
from unittest import mock

import pytest

import server
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
        assert "refusing to operate on non-local database host" in result.stderr
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
    def test_writable_fallback_preserves_auth_and_artifacts(self, tmp_path):
        from testenv import prepare_environment

        artifacts = tmp_path / "collected"
        parent = {
            "GOCACHE": "/proc/sippy-read-only-cache",
            "TMPDIR": "/proc/sippy-read-only-tmp",
            "NPM_CONFIG_CACHE": "/proc/sippy-read-only-npm",
            "ARTIFACT_DIR": str(artifacts),
            "SIPPY_SEED_DATABASE_DSN": "postgresql://user:secret@localhost/postgres?sslmode=disable",
            "REDIS_URL": "redis://user:secret@localhost:6379/0",
        }
        env = prepare_environment(parent, tmp_path / "logs")
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
        from testenv import prepare_environment

        parent = {"GOCACHE": str(tmp_path / "cache"), "SIPPY_E2E_REDIS_URL": "redis://localhost/5"}
        env = prepare_environment(parent, tmp_path / "logs")
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
        import testenv

        with mock.patch("testenv.socket.socket", side_effect=PermissionError(1, "Operation not permitted")):
            with pytest.raises(RuntimeError, match="MCP server needs local socket permissions"):
                testenv.check_prerequisites()

    @pytest.mark.parametrize("failure", ["postgres", "redis", "timeout", None])
    def test_service_checks_use_inherited_auth_without_logging_secrets(self, failure, capsys):
        import testenv

        env = {
            "SIPPY_DATABASE_DSN": "postgresql://user:private@localhost/postgres",
            "SIPPY_E2E_REDIS_URL": "redis://:private@localhost/1",
        }
        calls = []

        def run(command, **kwargs):
            calls.append((command, kwargs))
            if failure == "timeout":
                raise subprocess.TimeoutExpired(command, 15)
            failed = failure == "postgres"
            return subprocess.CompletedProcess(command, int(failed), "private" if failed else "t\n", "private")

        with (
            mock.patch.dict(os.environ, env),
            mock.patch("testenv.socket.socket"),
            mock.patch("testenv.Path.exists", return_value=True),
            mock.patch("testenv.subprocess.run", side_effect=run),
            mock.patch("testenv.redis_ping", side_effect=RuntimeError("private") if failure == "redis" else None) as ping,
        ):
            if failure:
                with pytest.raises(RuntimeError) as error:
                    testenv.check_prerequisites()
                assert "private" not in str(error.value)
            else:
                testenv.check_prerequisites()
        assert env["SIPPY_DATABASE_DSN"] in calls[0][0]
        assert "-w" in calls[0][0]
        if failure in ("redis", None):
            ping.assert_called_once_with(env["SIPPY_E2E_REDIS_URL"])
        assert "private" not in capsys.readouterr().out

    @pytest.mark.skipif(not Path("/run/.containerenv").exists(), reason="exercises container-only service preflight")
    def test_make_runs_in_the_validated_child_environment(self, tmp_path, monkeypatch):
        bin_dir = tmp_path / "bin"
        bin_dir.mkdir()
        for name, body in {
            "psql": '[ "$1" = "$SIPPY_DATABASE_DSN" ] || exit 90\nprintf "t\\n"',
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
            stub.write_text("#!/bin/sh\n" + body)
            stub.chmod(0o755)
        monkeypatch.setenv("PATH", f"{bin_dir}:{os.environ['PATH']}")
        monkeypatch.setenv("ARTIFACT_DIR", str(tmp_path / "artifacts"))
        for key in ("GOCACHE", "GOMODCACHE", "GOTMPDIR", "TMPDIR", "XDG_CACHE_HOME",
                    "npm_config_cache", "PLAYWRIGHT_BROWSERS_PATH"):
            monkeypatch.setenv(key, str(tmp_path / "cache" / key))
        monkeypatch.setenv("SIPPY_SEED_DATABASE_DSN", "postgresql://test:private@localhost/postgres")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            listener.settimeout(5)
            port = listener.getsockname()[1]
            url = f"redis://localhost:{port}/7"
            monkeypatch.setenv("SIPPY_E2E_REDIS_URL", url)
            monkeypatch.setenv("EXPECTED_REDIS_URL", url)
            monkeypatch.setattr(server, "DEV_LOG_DIR", tmp_path / "logs")
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
        assert "private" not in result
        assert (tmp_path / "artifacts/e2e/child-writable").exists()


class TestE2eFailurePaths:
    @pytest.mark.parametrize(
        "stage,status,expected,forbidden",
        [
            ("drop", 31, "Database setup failed", "Building sippy"),
            ("create", 32, "Database setup failed", "Building sippy"),
            ("seed", 33, "Database seeding failed (exit 33)", "Waiting for sippy API"),
            ("serve", 34, "API exited before readiness (exit 34)", "Priming component"),
            ("serve", 0, "API exited before readiness (exit 0)", "Priming component"),
            ("health", 1, "Timeout waiting for sippy API", "Priming component"),
            ("tests", 35, "Cache priming complete", "API exited before readiness"),
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
    echo "DONE test stub"
    if [[ "$FAIL_STAGE" == tests ]]; then return "$FAIL_STATUS"; fi
}
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
exec /bin/sleep 60
''')
        sippy.chmod(0o755)
        output_dir = tmp_path / "artifacts"
        result = subprocess.run(
            ["bash", str(E2E_SCRIPT)], cwd=tmp_path,
            env={**os.environ, "BASH_ENV": str(bash_env), "FAIL_STAGE": stage,
                 "FAIL_STATUS": str(status), "GCS_SA_JSON_PATH": "",
                 "SIPPY_TEST_OUTPUT_DIR": str(output_dir),
                 "SIPPY_DATABASE_DSN": "postgresql://test:secret@localhost:5432/postgres?sslmode=disable"},
            capture_output=True, text=True, timeout=10,
        )
        output = result.stdout + result.stderr
        assert result.returncode == (status or (1 if stage == "serve" else 0)), output
        assert expected in output
        assert forbidden not in output
        assert "secret" not in output
        for log in output_dir.glob("*.log"):
            assert "secret" not in log.read_text()
        if stage == "serve":
            assert "API startup failed" in output

    def test_dsn_connection_options_survive_database_replacement(self):
        result = TestE2eScriptSafety._run_helper('''
psql() { return 0; }
e2e_configure_existing_services
[ "$SIPPY_E2E_DSN" = 'postgresql://user:private@localhost:5432/sippy_e2e?sslmode=disable&connect_timeout=5' ]
''', {"SIPPY_DATABASE_DSN": "postgresql://user:private@localhost:5432/postgres?sslmode=disable&connect_timeout=5"})
        assert result.returncode == 0, result.stderr


class TestRedisProbe:
    @pytest.mark.parametrize(
        "url,responses,expected",
        [
            ("redis://localhost", b"+PONG\r\n", [[b"PING"]]),
            ("redis://:a%40b@localhost/1", b"+OK\r\n+OK\r\n+PONG\r\n",
             [[b"AUTH", b"a@b"], [b"SELECT", b"1"], [b"PING"]]),
            ("rediss://alice:private@localhost/2", b"+OK\r\n+OK\r\n+PONG\r\n",
             [[b"AUTH", b"alice", b"private"], [b"SELECT", b"2"], [b"PING"]]),
        ],
    )
    def test_auth_database_and_tls(self, url, responses, expected):
        import testenv

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
        import testenv

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
        import testenv

        with pytest.raises(ValueError):
            testenv.redis_ping(url)


class TestMakeTools:
    @pytest.mark.parametrize("name,target", [("run_tests", "test"), ("run_lint", "lint")])
    def test_tools_use_shared_runner_and_publish_timeout(self, name, target):
        with mock.patch("server._run_make_phase", new=mock.AsyncMock(return_value="completed")) as phase:
            result = asyncio.run(getattr(server, name)(timeout_seconds=123))
        assert result == "completed"
        phase.assert_awaited_once_with(name, target, f"{name}.log", 123)
        tool = asyncio.run(mcp.get_tool(name))
        assert tool.parameters["properties"]["timeout_seconds"] == {"default": 7200, "type": "integer"}

    @pytest.mark.parametrize("target,listeners", [("test", 1), ("lint", 0)])
    def test_unit_and_lint_do_not_require_database_services(self, target, listeners):
        import testenv

        with (
            mock.patch("testenv.socket.socket") as listener,
            mock.patch("testenv.subprocess.run") as command,
            mock.patch("testenv.redis_ping") as ping,
        ):
            testenv.check_prerequisites(target)
        assert listener.call_count == listeners
        command.assert_not_called()
        ping.assert_not_called()
