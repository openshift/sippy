"""Tests for the production MCP server entry point: tool registration, separation from the
development server, runtime configuration, and the real HTTP transport."""

from __future__ import annotations

import ast
import asyncio
import http.server
import json
import threading
from pathlib import Path

import pytest
from fastmcp import Client

import sippy_mcp_server
from sippy_mcp_server import (
    DEFAULT_HTTP_HOST,
    DEFAULT_HTTP_PORT,
    build_server,
    load_service,
    runtime_settings_from_env,
)
from test_triage_api import API_URL, TOKEN, FakeSippy
from triage_api import (
    API_URL_ENV,
    TOKEN_FILE_ENV,
    ConfigurationError,
    SippyAPIConfig,
    SippyClient,
    TransportError,
    TriageService,
    urllib_transport,
)

PRODUCTION_DIR = Path(__file__).resolve().parent
_TRIAGE_TOOLS = {"list_triages", "get_triage", "create_triage", "update_triage", "delete_triage"}
_MUTATION_TOOLS = {"create_triage", "update_triage", "delete_triage"}
# Tools registered by the development-workflow server (mcp/server.py); none may appear here.
_DEV_TOOLS = {
    "regression_cache",
    "restore_prodlike_db",
    "sippy_serve",
    "sippy_ng_start",
    "sippy_stop",
    "run_tests",
    "run_lint",
    "run_e2e",
    "get_make_result",
    "cancel_make_run",
    "check_services",
}


@pytest.fixture
def token_file(tmp_path):
    path = tmp_path / "token"
    path.write_text(TOKEN, encoding="utf-8")
    return str(path)


@pytest.fixture
def fake() -> FakeSippy:
    return FakeSippy()


@pytest.fixture
def server(fake, token_file):
    service = TriageService(SippyClient(SippyAPIConfig(api_url=API_URL, token_file=token_file), transport=fake))
    return build_server(service)


def _tools(server) -> dict:
    return {t.name: t for t in asyncio.run(server.list_tools())}


def test_registers_only_triage_tools(server):
    assert set(_tools(server)) == _TRIAGE_TOOLS


def test_no_development_tools(server):
    assert not set(_tools(server)) & _DEV_TOOLS


def test_every_tool_requires_acting_for(server):
    for name, tool in _tools(server).items():
        assert "acting_for" in tool.parameters["properties"], name
        assert "acting_for" in tool.parameters["required"], name
        assert "acting_for" in tool.description, name


def test_mutation_tools_document_rest_mapping(server):
    tools = _tools(server)
    assert "POST /api/component_readiness/triages" in tools["create_triage"].description
    assert "PUT /api/component_readiness/triages/{id}" in tools["update_triage"].description
    assert "DELETE /api/component_readiness/triages/{id}" in tools["delete_triage"].description


def test_update_schema_distinguishes_omitted_fields(server):
    params = _tools(server)["update_triage"].parameters
    assert set(params["required"]) == {"acting_for", "triage_id"}
    for field in ("url", "description", "type", "resolved", "add_regression_ids", "remove_regression_ids"):
        assert params["properties"][field]["default"] is None, field
    assert params["properties"]["clear_resolved"]["default"] is False
    assert params["additionalProperties"] is False


def _imported_modules(path: Path) -> set[str]:
    tree = ast.parse(path.read_text(encoding="utf-8"))
    names = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            names.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            names.add(node.module.split(".")[0])
    return names


@pytest.mark.parametrize("module", ["sippy_mcp_server.py", "triage_api.py"])
def test_production_modules_do_not_import_development_code(module):
    imported = _imported_modules(PRODUCTION_DIR / module)
    assert not imported & {"server", "testenv", "serviceurl", "dotenv", "subprocess"}


def test_calls_flow_through_mcp_with_delegated_identity(server, fake):
    async def run():
        async with Client(server) as client:
            created = await client.call_tool(
                "create_triage",
                {"acting_for": "alice", "url": "https://redhat.atlassian.net/browse/OCPBUGS-9", "type": "test"},
            )
            tid = created.structured_content["triage"]["id"]
            updated = await client.call_tool("update_triage", {"acting_for": "alice", "triage_id": tid, "description": "d"})
            deleted = await client.call_tool("delete_triage", {"acting_for": "alice", "triage_id": tid})
            return created, updated, deleted

    created, updated, deleted = asyncio.run(run())
    assert created.structured_content["verified"] is True
    assert updated.structured_content["triage"]["description"] == "d"
    assert deleted.structured_content["deleted"] is True
    assert {c["headers"]["X-Acting-For"] for c in fake.calls} == {"alice"}


def test_missing_acting_for_is_rejected_by_schema(server, fake):
    async def run():
        async with Client(server) as client:
            return await client.call_tool("delete_triage", {"triage_id": 1}, raise_on_error=False)

    assert asyncio.run(run()).is_error
    assert fake.calls == []


def test_healthz():
    from starlette.testclient import TestClient

    service = TriageService(SippyClient(SippyAPIConfig(api_url=API_URL, token_file="/nonexistent")))
    app = build_server(service).http_app()
    with TestClient(app) as client:
        response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


# --- runtime configuration --------------------------------------------------------------------


def test_runtime_defaults_bind_loopback():
    settings = runtime_settings_from_env({})
    assert settings.transport == "stdio"
    assert settings.host == DEFAULT_HTTP_HOST == "127.0.0.1"
    assert settings.port == DEFAULT_HTTP_PORT
    assert settings.allowed_hosts is None


def test_runtime_settings_parse():
    settings = runtime_settings_from_env(
        {
            "SIPPY_MCP_TRANSPORT": "streamable-http",
            "SIPPY_MCP_HTTP_HOST": "0.0.0.0",
            "SIPPY_MCP_HTTP_PORT": "9000",
            "SIPPY_MCP_ALLOWED_HOSTS": "sippy-mcp.example.com, other.example.com:443",
        }
    )
    assert (settings.transport, settings.host, settings.port) == ("streamable-http", "0.0.0.0", 9000)
    assert settings.allowed_hosts == ["sippy-mcp.example.com", "other.example.com:443"]


@pytest.mark.parametrize(
    "env, message",
    [
        ({"SIPPY_MCP_TRANSPORT": "sse"}, "TRANSPORT"),
        ({"SIPPY_MCP_HTTP_PORT": "http"}, "integer"),
        ({"SIPPY_MCP_HTTP_PORT": "70000"}, "between"),
    ],
)
def test_runtime_settings_fail_closed(env, message):
    with pytest.raises(ConfigurationError, match=message):
        runtime_settings_from_env(env)


def test_load_service_requires_readable_token(tmp_path):
    with pytest.raises(ConfigurationError, match="could not be read"):
        load_service({API_URL_ENV: API_URL, TOKEN_FILE_ENV: str(tmp_path / "missing")})


def test_main_exits_nonzero_without_configuration(monkeypatch, capsys):
    for name in (API_URL_ENV, TOKEN_FILE_ENV):
        monkeypatch.delenv(name, raising=False)
    assert sippy_mcp_server.main() == 2
    assert API_URL_ENV in capsys.readouterr().err


# --- real HTTP transport ----------------------------------------------------------------------


class _Recorder(http.server.BaseHTTPRequestHandler):
    seen: list[dict] = []

    def log_message(self, *args):
        pass

    def do_GET(self):
        _Recorder.seen.append({"path": self.path, "authorization": self.headers.get("Authorization")})
        if self.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "/login")
            self.end_headers()
            return
        status = 404 if self.path == "/missing" else 200
        body = json.dumps({"code": status, "message": "triage 1 not found"} if status == 404 else {"ok": True}).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


@pytest.fixture
def local_http():
    _Recorder.seen = []
    httpd = http.server.HTTPServer(("127.0.0.1", 0), _Recorder)
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{httpd.server_address[1]}"
    httpd.shutdown()
    httpd.server_close()


def test_urllib_transport_returns_error_responses(local_http):
    response = urllib_transport("GET", f"{local_http}/missing", {}, None, 5)
    assert response.status == 404
    assert json.loads(response.body)["message"] == "triage 1 not found"


def test_urllib_transport_does_not_follow_redirects(local_http):
    response = urllib_transport("GET", f"{local_http}/redirect", {"Authorization": "Bearer secret"}, None, 5)
    assert response.status == 302
    assert [s["path"] for s in _Recorder.seen] == ["/redirect"], "the bearer token must not be resent to a redirect target"


def test_urllib_transport_connection_failure():
    with pytest.raises(TransportError):
        urllib_transport("GET", "http://127.0.0.1:9/unreachable", {}, None, 2)
