"""Integration tests for the Sippy MCP triage tools against an isolated Sippy test instance.

scripts/e2e.sh runs these against its throwaway database before the Go e2e suite, which deletes
seeded regressions. They are skipped unless SIPPY_MCP_INTEGRATION_SIPPY_URL (the e2e Sippy API) and
SIPPY_MCP_INTEGRATION_DELEGATOR (a principal passed to that server with --trusted-delegator) are
set. Never point them at production.

A local stand-in for the deployed oauth-proxy authenticates the MCP client's bearer token, discards
any client-supplied X-Forwarded-User, and sets it to the configured delegator principal.
"""

from __future__ import annotations

import hmac
import http.server
import json
import os
import threading
import urllib.parse
import uuid
from datetime import datetime

import pytest

from triage_api import TRIAGES_PATH, HTTPResponse, SippyAPIConfig, SippyClient, TriageService, urllib_transport

SIPPY_URL = os.environ.get("SIPPY_MCP_INTEGRATION_SIPPY_URL", "").rstrip("/")
DELEGATOR = os.environ.get("SIPPY_MCP_INTEGRATION_DELEGATOR", "")

pytestmark = pytest.mark.skipif(
    not SIPPY_URL or not DELEGATOR,
    reason="set SIPPY_MCP_INTEGRATION_SIPPY_URL and SIPPY_MCP_INTEGRATION_DELEGATOR to run against an isolated Sippy",
)

PROXY_TOKEN = "integration-proxy-token"
_FORWARDED_REQUEST_HEADERS = ("Accept", "Content-Type", "X-Acting-For")


def _sippy(method: str, path: str, headers: dict[str, str] | None = None, body: bytes | None = None) -> HTTPResponse:
    return urllib_transport(method, SIPPY_URL + path, headers or {}, body, 30)


def _sippy_json(path: str):
    response = _sippy("GET", path)
    assert response.status == 200, response.body
    return json.loads(response.body)


class _AuthProxy(http.server.BaseHTTPRequestHandler):
    """Simulates the authenticating proxy that fronts the deployed Sippy write API."""

    def log_message(self, *args):
        pass

    def _forward(self):
        if not hmac.compare_digest(self.headers.get("Authorization", ""), f"Bearer {PROXY_TOKEN}"):
            self._reply(HTTPResponse(401, json.dumps({"code": 401, "message": "proxy rejected token"}).encode()))
            return
        headers = {name: self.headers[name] for name in _FORWARDED_REQUEST_HEADERS if self.headers.get(name) is not None}
        headers["X-Forwarded-User"] = DELEGATOR
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else None
        self._reply(_sippy(self.command, self.path, headers, body))

    def _reply(self, response: HTTPResponse):
        self.send_response(response.status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(response.body)))
        self.end_headers()
        self.wfile.write(response.body)

    do_GET = do_POST = do_PUT = do_DELETE = _forward


@pytest.fixture(scope="module")
def proxy_url():
    httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _AuthProxy)
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{httpd.server_address[1]}"
    httpd.shutdown()
    httpd.server_close()


def _service(proxy_url: str, tmp_path, token: str = PROXY_TOKEN) -> TriageService:
    token_file = tmp_path / f"token-{uuid.uuid4().hex}"
    token_file.write_text(token, encoding="utf-8")
    return TriageService(SippyClient(SippyAPIConfig(api_url=proxy_url, token_file=str(token_file))))


@pytest.fixture
def service(proxy_url, tmp_path) -> TriageService:
    return _service(proxy_url, tmp_path)


@pytest.fixture
def unique_url() -> str:
    return f"https://redhat.atlassian.net/browse/MCPTEST-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="module")
def regression_ids() -> list[int]:
    views = _sippy_json("/api/component_readiness/views")
    releases = {v["sample_release"]["release"] for v in views if v.get("sample_release")}
    ids: set[int] = set()
    for release in sorted(releases):
        query = urllib.parse.urlencode({"release": release})
        ids.update(r["id"] for r in _sippy_json(f"/api/component_readiness/regressions?{query}") or [])
    return sorted(ids)[:3]


def _audit(triage_id: int) -> list[dict]:
    return _sippy_json(f"{TRIAGES_PATH}/{triage_id}/audit") or []


def _instant(value: str | None) -> datetime | None:
    """Parse an RFC 3339 timestamp so instants compare equal regardless of offset formatting."""
    return datetime.fromisoformat(value) if value else None


def _triage_urls() -> set[str]:
    response = _sippy("GET", f"{TRIAGES_PATH}?include_archived=true")
    assert response.status == 200
    return {t["url"] for t in json.loads(response.body) or []}


def test_lifecycle_with_audit_attribution(service, unique_url, regression_ids):
    assert len(regression_ids) >= 2, "the isolated instance must have at least two seeded regressions"
    first, second = regression_ids[0], regression_ids[1]

    created = service.create_triage("alice@example.com", unique_url, "product", description="initial", regression_ids=[first])
    assert "error" not in created, created
    assert (created["triage"]["description"], created["triage"]["regression_ids"]) == ("initial", [first])
    tid = created["triage"]["id"]

    try:
        updated = service.update_triage("alice@example.com", tid, description="changed")
        assert "error" not in updated, updated
        triage = updated["triage"]
        assert (triage["url"], triage["type"], triage["regression_ids"]) == (unique_url, "product", [first])

        associated = service.update_triage("bob@example.com", tid, add_regression_ids=[second], remove_regression_ids=[first])
        assert associated["triage"]["regression_ids"] == [second], associated
        assert associated["triage"]["description"] == "changed"

        resolved = service.update_triage("bob@example.com", tid, resolved="2026-11-01T12:00:00Z")
        assert resolved["triage"]["resolved"] is not None and resolved["triage"]["resolution_reason"] == "user"
        # A later edit that leaves the resolution alone must not disturb it.
        kept = service.update_triage("bob@example.com", tid, type="test")
        assert _instant(kept["triage"]["resolved"]) == _instant(resolved["triage"]["resolved"]), kept
        assert kept["triage"]["regression_ids"] == [second]
        cleared = service.update_triage("bob@example.com", tid, clear_resolved=True)
        assert cleared["triage"]["resolved"] is None

        listed = service.list_triages(url_contains=unique_url)
        assert [t["id"] for t in listed["triages"]] == [tid]
    finally:
        deleted = service.delete_triage("carol@example.com", tid)
    assert deleted.get("deleted") is True, deleted
    assert service.get_triage(tid)["error"]["code"] == "not_found"

    users_by_operation = [(a["operation"], a["user"]) for a in reversed(_audit(tid))]
    assert users_by_operation[0] == ("CREATE", "alice@example.com")
    assert users_by_operation[-1] == ("DELETE", "carol@example.com")
    assert ("UPDATE", "alice@example.com") in users_by_operation
    assert ("UPDATE", "bob@example.com") in users_by_operation
    assert all(user != DELEGATOR for _, user in users_by_operation), "audit must name the requesting user"


def test_empty_list_result(service):
    result = service.list_triages(url_contains=f"no-such-triage-{uuid.uuid4().hex}")
    assert result == {"triages": [], "count": 0, "total_matches": 0, "truncated": False}


def test_nonexistent_records(service):
    missing = 2**62
    assert service.get_triage(missing)["error"]["code"] == "not_found"
    assert service.update_triage("alice@example.com", missing, description="x")["error"]["code"] == "not_found"
    assert service.delete_triage("alice@example.com", missing)["error"]["code"] == "not_found"


def test_invalid_regression_is_rejected_without_changes(service, unique_url):
    result = service.create_triage("alice@example.com", unique_url, "product", regression_ids=[2**62])
    assert result["error"]["code"] == "invalid_input"
    assert unique_url not in _triage_urls()


def test_rejected_token_makes_no_change(proxy_url, tmp_path, unique_url):
    result = _service(proxy_url, tmp_path, token="wrong-token").create_triage("alice@example.com", unique_url, "product")
    assert result["error"]["code"] == "unauthenticated"
    assert "wrong-token" not in json.dumps(result)
    assert unique_url not in _triage_urls()


def test_sippy_rejects_delegator_mutation_without_acting_for(service, unique_url):
    """Bypasses the tool's own validation to confirm the server-side delegation check."""
    for acting_for in (None, ""):
        response = service.client.request("POST", TRIAGES_PATH, acting_for, body={"url": unique_url, "type": "product"})
        assert response.status == 400
        assert "X-Acting-For" in json.loads(response.body)["message"]
    assert unique_url not in _triage_urls()


def test_sippy_allows_delegator_reads_without_acting_for(service):
    response = service.client.request("GET", TRIAGES_PATH)
    assert response.status == 200, response.body
