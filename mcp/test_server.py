import asyncio
import inspect
import os
import subprocess
import tempfile
from pathlib import Path
from unittest import mock

import pytest

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
    _validate_redis_url,
    _trim,
    _wait_for_url,
    mcp,
    run_e2e,
)

E2E_SCRIPT = REPO_ROOT / "scripts" / "e2e.sh"


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
    @staticmethod
    def _successful_subprocess():
        process = mock.Mock()
        process.wait = mock.AsyncMock(return_value=0)
        return process

    def test_defaults_to_no_bigquery_without_credentials(self, tmp_path):
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
                explicit.name: b"explicit credentials",
                sippy_env.name: b"sippy environment credentials",
                google_env.name: b"google environment credentials",
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
        database_password = "synthetic-database-password"
        redis_password = "synthetic-redis-password"
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
    [ "$1" = "$EXPECTED_ADMIN_DSN" ] || return 90
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

    @pytest.mark.parametrize(
        "dsn",
        [
            "postgresql://user:local-password@localhost:5432/postgres",
            "postgresql://user:service-password@sippy-postgres:5432/postgres",
        ],
    )
    def test_safe_dsn_host_still_accepts_local_services(self, dsn):
        """Preserve the localhost and sippy-postgres allowlist used by destructive helpers."""
        result = self._run_helper('e2e_safe_dsn_host "$SAFE_DSN"', {"SAFE_DSN": dsn})

        assert result.returncode == 0, result.stderr
        assert result.stdout == ""
        assert result.stderr == ""

    def test_safe_dsn_host_rejection_does_not_echo_credentials(self):
        """Reject remote database hosts without copying their credential-bearing DSN to stderr."""
        password = "synthetic-rejected-password"
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
