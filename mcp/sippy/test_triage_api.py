"""Unit tests for the Sippy MCP triage client using a client-level fake transport."""

from __future__ import annotations

import copy
import json
import os
import re
from collections.abc import Callable

import pytest

from triage_api import (
    API_URL_ENV,
    TOKEN_FILE_ENV,
    ConfigurationError,
    HTTPResponse,
    SippyAPIConfig,
    SippyClient,
    TransportError,
    TriageService,
    read_token,
    validate_api_url,
)

TOKEN = "sa-token-secret-value"
API_URL = "https://sippy-auth.example.com"
_TRIAGE_PATH = re.compile(r"^/api/component_readiness/triages/(\d+)$")


def _json(status: int, payload) -> HTTPResponse:
    return HTTPResponse(status, json.dumps(payload).encode())


def _failure(status: int, message: str) -> HTTPResponse:
    return _json(status, {"code": status, "message": message})


def _regression(rid: int) -> dict:
    return {
        "id": rid,
        "test_id": f"test-{rid}",
        "test_name": f"test name {rid}",
        "release": "4.22",
        "base_release": "4.21",
        "component": "comp",
        "capability": "cap",
        "variants": ["Platform:aws"],
        "opened": "2026-11-01T00:00:00Z",
        "closed": {"Time": "0001-01-01T00:00:00Z", "Valid": False},
        "job_runs": [{"id": 1, "prowjob_run_id": "123"}],
    }


class FakeSippy:
    """In-memory model of Sippy's triage endpoints with PUT replacement semantics."""

    def __init__(self) -> None:
        self.triages: dict[int, dict] = {}
        self.regressions = {rid: _regression(rid) for rid in (11, 12, 13)}
        self.next_id = 100
        self.calls: list[dict] = []
        # Overrides: (method, path) -> list of responses/exceptions consumed in order.
        self.scripted: dict[tuple[str, str], list] = {}
        self.audit: list[tuple[str, int, str]] = []

    def add_triage(self, **fields) -> dict:
        tid = fields.pop("id", None) or self.next_id
        self.next_id = max(self.next_id, tid) + 1
        record = {
            "id": tid,
            "created_at": "2026-11-01T00:00:00Z",
            "updated_at": "2026-11-01T00:00:00Z",
            "url": "https://redhat.atlassian.net/browse/OCPBUGS-1",
            "description": "original",
            "type": "product",
            "bug_id": 7,
            "bug": {"id": 7, "key": "OCPBUGS-1", "status": "New", "summary": "s", "url": "u"},
            "regressions": [],
            "resolved": {"Time": "0001-01-01T00:00:00Z", "Valid": False},
            "resolution_reason": "",
        }
        record.update(fields)
        record["regressions"] = [self.regressions[r] for r in record["regressions"]]
        self.triages[tid] = record
        return record

    def _out(self, record: dict) -> dict:
        out = copy.deepcopy(record)
        out["links"] = {"self": f"{API_URL}/api/component_readiness/triages/{record['id']}"}
        return out

    def __call__(self, method: str, url: str, headers: dict, body: bytes | None, timeout: float) -> HTTPResponse:
        assert url.startswith(API_URL)
        path_and_query = url[len(API_URL):]
        path = path_and_query.split("?", 1)[0]
        parsed = json.loads(body) if body else None
        self.calls.append({"method": method, "url": url, "path": path, "headers": dict(headers), "body": parsed})
        script = self.scripted.get((method, path))
        if script:
            result = script.pop(0)
            if isinstance(result, Exception):
                raise result
            if callable(result):
                return result()
            return result
        user = headers.get("X-Acting-For")
        # Like Sippy for a trusted delegator: mutations must name a user, reads need not.
        if method != "GET" and not user:
            return _failure(400, "X-Acting-For is required for requests from a trusted delegator")
        if path == "/api/component_readiness/triages":
            if method == "GET":
                return _json(200, [self._out(t) for t in self.triages.values()])
            if method == "POST":
                if not parsed.get("url"):
                    return _failure(400, "url is required for a triage record")
                missing = [r["id"] for r in parsed.get("regressions", []) if r["id"] not in self.regressions]
                if missing:
                    return _failure(400, f"some of the requested regression IDs were not found: {missing}")
                record = self.add_triage(
                    url=parsed["url"],
                    type=parsed["type"],
                    description=parsed.get("description", ""),
                    regressions=[r["id"] for r in parsed.get("regressions", [])],
                    bug_id=None,
                    bug=None,
                )
                self.audit.append(("create", record["id"], user))
                return _json(200, self._out(record))
        match = _TRIAGE_PATH.match(path)
        if match:
            tid = int(match.group(1))
            record = self.triages.get(tid)
            if record is None:
                return _failure(404, f"triage {tid} not found")
            if method == "GET":
                return _json(200, self._out(record))
            if method == "DELETE":
                del self.triages[tid]
                self.audit.append(("delete", tid, user))
                return _json(200, None)
            if method == "PUT":
                # Full replacement: anything missing from the body is cleared, like gorm Save.
                replacement = {
                    "id": tid,
                    "created_at": record["created_at"],
                    "updated_at": "2026-11-19T00:00:00Z",
                    "url": parsed.get("url", ""),
                    "description": parsed.get("description", ""),
                    "type": parsed.get("type", ""),
                    "bug_id": parsed.get("bug_id"),
                    "bug": record["bug"] if parsed.get("bug_id") == record["bug_id"] else None,
                    "regressions": [self.regressions[r["id"]] for r in parsed.get("regressions", [])],
                    "resolved": parsed.get("resolved", {"Time": "0001-01-01T00:00:00Z", "Valid": False}),
                    "resolution_reason": parsed.get("resolution_reason", ""),
                }
                self.triages[tid] = replacement
                self.audit.append(("update", tid, user))
                return _json(200, self._out(replacement))
        return _failure(404, "no route")

    def mutations(self) -> list[dict]:
        return [c for c in self.calls if c["method"] in ("POST", "PUT", "DELETE")]


@pytest.fixture
def token_file(tmp_path):
    path = tmp_path / "token"
    path.write_text(TOKEN + "\n", encoding="utf-8")
    return str(path)


@pytest.fixture
def fake() -> FakeSippy:
    return FakeSippy()


@pytest.fixture
def service(fake, token_file) -> TriageService:
    return TriageService(SippyClient(SippyAPIConfig(api_url=API_URL, token_file=token_file), transport=fake))


def _error_code(result: dict) -> str:
    assert "error" in result, result
    return result["error"]["code"]


def _assert_no_secret(result) -> None:
    assert TOKEN not in json.dumps(result)


# --- configuration --------------------------------------------------------------------------


@pytest.mark.parametrize(
    "env, message",
    [
        ({}, API_URL_ENV),
        ({API_URL_ENV: "   "}, API_URL_ENV),
        ({API_URL_ENV: API_URL}, TOKEN_FILE_ENV),
        ({API_URL_ENV: "http://sippy.example.com", TOKEN_FILE_ENV: "/t"}, "https"),
        ({API_URL_ENV: "https://user:pw@sippy.example.com", TOKEN_FILE_ENV: "/t"}, "credentials"),
        ({API_URL_ENV: "https://sippy.example.com?x=1", TOKEN_FILE_ENV: "/t"}, "query"),
        ({API_URL_ENV: "https://sippy.example.com?", TOKEN_FILE_ENV: "/t"}, "query"),
        ({API_URL_ENV: "https://sippy.example.com/#", TOKEN_FILE_ENV: "/t"}, "query"),
        ({API_URL_ENV: "https://sippy.example.com:abc", TOKEN_FILE_ENV: "/t"}, "valid"),
        ({API_URL_ENV: "https://sippy.example.com:70000", TOKEN_FILE_ENV: "/t"}, "valid"),
        ({API_URL_ENV: "https://[::1", TOKEN_FILE_ENV: "/t"}, "valid"),
        ({API_URL_ENV: "ftp://sippy.example.com", TOKEN_FILE_ENV: "/t"}, "absolute"),
        ({API_URL_ENV: "sippy.example.com", TOKEN_FILE_ENV: "/t"}, "absolute"),
        ({API_URL_ENV: API_URL, TOKEN_FILE_ENV: "/t", "SIPPY_MCP_REQUEST_TIMEOUT": "soon"}, "number"),
        ({API_URL_ENV: API_URL, TOKEN_FILE_ENV: "/t", "SIPPY_MCP_REQUEST_TIMEOUT": "0"}, "between"),
    ],
)
def test_config_fails_closed(env, message):
    with pytest.raises(ConfigurationError, match=message):
        SippyAPIConfig.from_env(env)


def test_config_ignores_development_settings():
    """Development settings never select an endpoint: the API URL has no default."""
    env = {"SIPPY_DATABASE_DSN": "postgresql://localhost/postgres", "DEV_MODE": "1", TOKEN_FILE_ENV: "/t"}
    with pytest.raises(ConfigurationError, match="no default"):
        SippyAPIConfig.from_env(env)


@pytest.mark.parametrize(
    "raw, expected",
    [
        ("https://sippy-auth.example.com/", "https://sippy-auth.example.com"),
        ("http://127.0.0.1:18080", "http://127.0.0.1:18080"),
        ("http://localhost:8080", "http://localhost:8080"),
        ("http://[::1]:8080", "http://[::1]:8080"),
    ],
)
def test_validate_api_url_accepts(raw, expected):
    assert validate_api_url(raw) == expected


def test_read_token_errors_do_not_expose_contents(tmp_path):
    empty = tmp_path / "empty"
    empty.write_text("  \n", encoding="utf-8")
    with pytest.raises(ConfigurationError, match="empty"):
        read_token(str(empty))
    with pytest.raises(ConfigurationError, match="could not be read"):
        read_token(str(tmp_path / "missing"))


@pytest.mark.parametrize(
    "contents",
    [f"{TOKEN}\nsecond-line", f"{TOKEN} trailing", f"{TOKEN}\r\nX-Injected: 1", f"tok\u00e9n-{TOKEN}"],
)
def test_read_token_rejects_malformed_tokens_without_exposing_them(tmp_path, contents):
    path = tmp_path / "token"
    path.write_text(contents, encoding="utf-8")
    with pytest.raises(ConfigurationError, match="single bearer token") as excinfo:
        read_token(str(path))
    assert TOKEN not in str(excinfo.value)


@pytest.mark.parametrize("contents", [f"{TOKEN}\n", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.c2ln-_", "dG9rZW4="])
def test_read_token_accepts_bearer_tokens(tmp_path, contents):
    path = tmp_path / "token"
    path.write_text(contents, encoding="utf-8")
    assert read_token(str(path)) == contents.strip()


# --- request construction ---------------------------------------------------------------------


def test_reads_carry_bearer_token_without_delegated_user(service, fake):
    fake.add_triage(id=5)
    result = service.get_triage(5)
    assert result["triage"]["id"] == 5
    service.list_triages()
    for call in fake.calls:
        assert call["headers"]["Authorization"] == f"Bearer {TOKEN}"
        assert "X-Acting-For" not in call["headers"]
    _assert_no_secret(result)


def test_mutations_carry_bearer_token_and_delegated_user(service, fake):
    fake.add_triage(id=5)
    result = service.update_triage("  alice@example.com ", 5, description="d")
    assert result["verified"] is True
    for call in fake.calls:
        assert call["headers"]["Authorization"] == f"Bearer {TOKEN}"
        assert call["headers"]["X-Acting-For"] == "alice@example.com"
    _assert_no_secret(result)


def test_token_is_reread_for_rotation(service, fake, token_file):
    fake.add_triage(id=5)
    service.get_triage(5)
    with open(token_file, "w", encoding="utf-8") as f:
        f.write("rotated-token")
    service.get_triage(5)
    assert fake.calls[1]["headers"]["Authorization"] == "Bearer rotated-token"


def test_missing_token_rejects_without_request(fake, tmp_path):
    svc = TriageService(SippyClient(SippyAPIConfig(api_url=API_URL, token_file=str(tmp_path / "none")), transport=fake))
    result = svc.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product")
    assert _error_code(result) == "configuration_error"
    assert fake.calls == []


@pytest.mark.parametrize(
    "acting_for", ["", "   ", None, "a" * 257, "alice\nX-Forwarded-User: admin", "bob\x00", "\u674e", "jos\u00e9@example.com"]
)
@pytest.mark.parametrize(
    "call",
    [
        lambda s, a: s.create_triage(a, "https://redhat.atlassian.net/browse/OCPBUGS-9", "product"),
        lambda s, a: s.update_triage(a, 1, description="x"),
        lambda s, a: s.delete_triage(a, 1),
    ],
)
def test_invalid_acting_for_is_rejected_before_any_request(service, fake, call: Callable, acting_for):
    assert _error_code(call(service, acting_for)) == "invalid_input"
    assert fake.calls == []


# --- list / get -------------------------------------------------------------------------------


def test_list_empty_result(service, fake):
    result = service.list_triages()
    assert result == {"triages": [], "count": 0, "total_matches": 0, "truncated": False}


def test_list_null_response_is_empty(service, fake):
    fake.scripted[("GET", "/api/component_readiness/triages")] = [_json(200, None)]
    assert service.list_triages()["triages"] == []


def test_list_filters_and_limits(service, fake):
    fake.add_triage(id=1, type="product", url="https://redhat.atlassian.net/browse/OCPBUGS-100", regressions=[11])
    fake.add_triage(id=2, type="test", url="https://redhat.atlassian.net/browse/OCPBUGS-200",
                    resolved={"Time": "2026-11-02T00:00:00Z", "Valid": True}, resolution_reason="user")
    fake.add_triage(id=3, type="product", url="https://redhat.atlassian.net/browse/TRT-1", regressions=[12])

    result = service.list_triages(view="4.22-main", include_archived=True, limit=2)
    assert [t["id"] for t in result["triages"]] == [3, 2]
    assert result["total_matches"] == 3 and result["truncated"] is True
    assert "view=4.22-main" in fake.calls[0]["url"] and "include_archived=true" in fake.calls[0]["url"]
    assert "regressions" not in result["triages"][0]

    assert [t["id"] for t in service.list_triages(type="product")["triages"]] == [3, 1]
    assert [t["id"] for t in service.list_triages(status="resolved")["triages"]] == [2]
    assert [t["id"] for t in service.list_triages(status="open")["triages"]] == [3, 1]
    assert [t["id"] for t in service.list_triages(url_contains="ocpbugs")["triages"]] == [2, 1]
    assert [t["id"] for t in service.list_triages(regression_id=12)["triages"]] == [3]
    resolved = service.list_triages(status="resolved")["triages"][0]
    assert resolved["resolved"] == "2026-11-02T00:00:00Z" and resolved["resolution_reason"] == "user"


@pytest.mark.parametrize(
    "kwargs",
    [
        {"type": "bogus"},
        {"status": "closed"},
        {"limit": 0},
        {"limit": 201},
        {"limit": True},
        {"regression_id": -1},
        {"view": " "},
        {"url_contains": ""},
        {"include_archived": "yes"},
    ],
)
def test_list_invalid_inputs(service, fake, kwargs):
    assert _error_code(service.list_triages(**kwargs)) == "invalid_input"
    assert fake.calls == []


def test_get_returns_regression_summaries(service, fake):
    fake.add_triage(id=5, regressions=[11, 12])
    triage = service.get_triage(5)["triage"]
    assert triage["regression_ids"] == [11, 12]
    assert triage["regressions"][0]["test_name"] == "test name 11"
    assert "job_runs" not in triage["regressions"][0]
    assert triage["resolved"] is None
    assert triage["bug"]["key"] == "OCPBUGS-1"


def test_get_not_found(service):
    result = service.get_triage(404404)
    assert _error_code(result) == "not_found"
    assert result["error"]["triage_id"] == 404404


@pytest.mark.parametrize("triage_id", [0, -3, True, "5", 2**63])
def test_get_invalid_id(service, fake, triage_id):
    assert _error_code(service.get_triage(triage_id)) == "invalid_input"
    assert fake.calls == []


# --- create -----------------------------------------------------------------------------------


def test_create_verifies_by_readback(service, fake):
    result = service.create_triage(
        "alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product", description="d", regression_ids=[11, 12]
    )
    assert result["verified"] is True
    assert result["triage"]["regression_ids"] == [11, 12]
    assert [(c["method"], c["path"]) for c in fake.calls] == [
        ("POST", "/api/component_readiness/triages"),
        ("GET", f"/api/component_readiness/triages/{result['triage']['id']}"),
    ]
    assert fake.calls[0]["body"] == {
        "url": "https://redhat.atlassian.net/browse/OCPBUGS-9",
        "type": "product",
        "description": "d",
        "regressions": [{"id": 11}, {"id": 12}],
    }
    assert fake.audit == [("create", result["triage"]["id"], "alice")]


@pytest.mark.parametrize(
    "kwargs",
    [
        {"url": "", "type": "product"},
        {"url": "OCPBUGS-1", "type": "product"},
        {"url": "https://[::1", "type": "product"},
        {"url": "https://:443", "type": "product"},
        {"url": "https://redhat.atlassian.net:abc/browse/OCPBUGS-1", "type": "product"},
        {"url": "https://redhat.atlassian.net/browse/OCPBUGS-1", "type": "bogus"},
        {"url": "https://redhat.atlassian.net/browse/OCPBUGS-1", "type": "product", "regression_ids": []},
        {"url": "https://redhat.atlassian.net/browse/OCPBUGS-1", "type": "product", "regression_ids": [11, 11]},
        {"url": "https://redhat.atlassian.net/browse/OCPBUGS-1", "type": "product", "regression_ids": [0]},
        {"url": "https://redhat.atlassian.net/browse/OCPBUGS-1", "type": "product", "description": 5},
    ],
)
def test_create_invalid_inputs(service, fake, kwargs):
    assert _error_code(service.create_triage("alice", **kwargs)) == "invalid_input"
    assert fake.calls == []


def test_create_upstream_validation_error(service, fake):
    result = service.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product", regression_ids=[99])
    assert _error_code(result) == "invalid_input"
    assert "99" in result["error"]["message"]
    assert fake.triages == {}


def test_create_verification_failure_is_reported(service, fake):
    def accept_without_regressions():
        record = fake.add_triage(url="https://redhat.atlassian.net/browse/OCPBUGS-9", regressions=[])
        return _json(200, record)

    fake.scripted[("POST", "/api/component_readiness/triages")] = [accept_without_regressions]
    result = service.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product", regression_ids=[11])
    assert _error_code(result) == "verification_failed"
    assert {m["field"] for m in result["error"]["mismatches"]} >= {"regression_ids"}


@pytest.mark.parametrize(
    "failure, code",
    [
        (TransportError("TimeoutError"), "outcome_unknown"),
        (_failure(500, "database exploded"), "upstream_error"),
        (_failure(401, "authentication is required"), "unauthenticated"),
        (_failure(403, "forbidden"), "forbidden"),
        (_failure(400, "X-Acting-For is required for requests from a trusted delegator"), "invalid_input"),
        (_failure(501, "not capable"), "unsupported"),
        (HTTPResponse(302, b"<html>login</html>"), "unauthenticated"),
        (HTTPResponse(502, b"<html>bad gateway</html>"), "upstream_error"),
    ],
)
def test_create_failures_are_not_replayed(service, fake, failure, code):
    fake.scripted[("POST", "/api/component_readiness/triages")] = [failure]
    result = service.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product")
    assert _error_code(result) == code
    assert len(fake.mutations()) == 1, "mutations must never be retried automatically"
    assert "<html>" not in json.dumps(result)
    _assert_no_secret(result)


def test_create_server_error_says_outcome_may_be_unknown(service, fake):
    fake.scripted[("POST", "/api/component_readiness/triages")] = [_failure(500, "boom")]
    assert "read the triage before retrying" in service.create_triage(
        "alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product"
    )["error"]["message"]


@pytest.mark.parametrize("body", [b"<html>ok</html>", b"{}", b"null"])
def test_create_success_without_id_is_outcome_unknown(service, fake, body):
    fake.scripted[("POST", "/api/component_readiness/triages")] = [HTTPResponse(200, body)]
    result = service.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product")
    assert _error_code(result) == "outcome_unknown"
    assert len(fake.mutations()) == 1


def _break_token_after_first_write(fake, token_file, method):
    """Make the token unreadable as soon as the first mutation succeeds, so only the readback fails."""
    original = fake.__call__

    def transport(m, url, headers, body, timeout):
        response = original(m, url, headers, body, timeout)
        if m == method:
            os.remove(token_file)
        return response

    return transport


@pytest.mark.parametrize(
    "method, call",
    [
        ("POST", lambda s: s.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product")),
        ("PUT", lambda s: s.update_triage("alice", 5, description="x")),
        ("DELETE", lambda s: s.delete_triage("alice", 5)),
    ],
)
def test_readback_credential_failure_is_verification_unavailable(fake, token_file, method, call):
    fake.add_triage(id=5)
    transport = _break_token_after_first_write(fake, token_file, method)
    svc = TriageService(SippyClient(SippyAPIConfig(api_url=API_URL, token_file=token_file), transport=transport))
    result = call(svc)
    assert _error_code(result) == "verification_unavailable", result
    assert isinstance(result["error"]["triage_id"], int)
    assert len(fake.mutations()) == 1


def test_create_readback_failure(service, fake):
    def created():
        record = fake.add_triage(id=321, url="https://redhat.atlassian.net/browse/OCPBUGS-9")
        return _json(200, record)

    fake.scripted[("POST", "/api/component_readiness/triages")] = [created]
    fake.scripted[("GET", "/api/component_readiness/triages/321")] = [TransportError("ConnectionResetError")]
    result = service.create_triage("alice", "https://redhat.atlassian.net/browse/OCPBUGS-9", "product")
    assert _error_code(result) == "verification_unavailable"
    assert result["error"]["triage_id"] == 321


# --- update -----------------------------------------------------------------------------------


def test_update_preserves_omitted_fields_and_regressions(service, fake):
    fake.add_triage(
        id=5,
        regressions=[11, 12],
        resolved={"Time": "2026-11-02T03:04:05.123456Z", "Valid": True},
        resolution_reason="jira-progression",
    )
    result = service.update_triage("alice", 5, description="new description")
    assert result["verified"] is True and result["changed_fields"] == ["description"]
    assert "warnings" not in result

    put = next(c for c in fake.calls if c["method"] == "PUT")
    assert [(c["method"]) for c in fake.calls] == ["GET", "PUT", "GET"]
    body = put["body"]
    assert body["description"] == "new description"
    assert body["url"] == "https://redhat.atlassian.net/browse/OCPBUGS-1"
    assert body["type"] == "product"
    assert body["bug_id"] == 7
    assert body["resolved"] == {"Time": "2026-11-02T03:04:05.123456Z", "Valid": True}
    assert body["resolution_reason"] == "jira-progression"
    assert body["regressions"] == [{"id": 11}, {"id": 12}]
    assert "links" not in body and "bug" not in body

    persisted = fake.triages[5]
    assert [r["id"] for r in persisted["regressions"]] == [11, 12]
    assert persisted["resolution_reason"] == "jira-progression"
    assert fake.audit == [("update", 5, "alice")]


def test_update_clears_description_explicitly(service, fake):
    fake.add_triage(id=5)
    result = service.update_triage("alice", 5, description="")
    assert result["triage"]["description"] == ""


def test_update_adds_and_removes_regressions(service, fake):
    fake.add_triage(id=5, regressions=[11, 12])
    result = service.update_triage("alice", 5, add_regression_ids=[13], remove_regression_ids=[11])
    assert result["triage"]["regression_ids"] == [12, 13]
    assert result["changed_fields"] == ["regression_ids"]


def test_update_can_remove_all_regressions_explicitly(service, fake):
    fake.add_triage(id=5, regressions=[11, 12])
    assert service.update_triage("alice", 5, remove_regression_ids=[11, 12])["triage"]["regression_ids"] == []


def test_update_resolve_and_unresolve(service, fake):
    fake.add_triage(id=5)
    resolved = service.update_triage("alice", 5, resolved="2026-11-03T10:00:00+02:00")
    assert resolved["triage"]["resolved"] == "2026-11-03T08:00:00Z"
    put = [c for c in fake.calls if c["method"] == "PUT"][-1]
    assert put["body"]["resolved"] == {"Time": "2026-11-03T08:00:00Z", "Valid": True}

    cleared = service.update_triage("alice", 5, clear_resolved=True)
    assert cleared["triage"]["resolved"] is None
    put = [c for c in fake.calls if c["method"] == "PUT"][-1]
    assert put["body"]["resolved"]["Valid"] is False


@pytest.mark.parametrize(
    "kwargs, message",
    [
        ({}, "no changes"),
        ({"clear_resolved": False}, "no changes"),
        ({"resolved": "2026-11-03T10:00:00Z", "clear_resolved": True}, "mutually exclusive"),
        ({"resolved": "2026-11-03T10:00:00"}, "timezone"),
        ({"resolved": "yesterday"}, "RFC 3339"),
        ({"resolved": ""}, "clear_resolved"),
        ({"add_regression_ids": []}, "must not be empty"),
        ({"remove_regression_ids": []}, "must not be empty"),
        ({"add_regression_ids": [13], "remove_regression_ids": [13]}, "both added and removed"),
        ({"add_regression_ids": [13, 13]}, "duplicates"),
        ({"url": ""}, "url"),
        ({"type": "bogus"}, "type"),
        ({"clear_resolved": "true"}, "boolean"),
    ],
)
def test_update_rejects_ambiguous_or_invalid_input(service, fake, kwargs, message):
    fake.add_triage(id=5)
    result = service.update_triage("alice", 5, **kwargs)
    assert _error_code(result) == "invalid_input"
    assert message in result["error"]["message"]
    assert fake.mutations() == []


@pytest.mark.parametrize(
    "kwargs, message",
    [
        ({"add_regression_ids": [11]}, "already linked"),
        ({"remove_regression_ids": [13]}, "not linked"),
    ],
)
def test_update_rejects_association_changes_that_do_not_apply(service, fake, kwargs, message):
    fake.add_triage(id=5, regressions=[11])
    result = service.update_triage("alice", 5, **kwargs)
    assert _error_code(result) == "invalid_input"
    assert message in result["error"]["message"]
    assert fake.mutations() == []


def test_update_not_found(service, fake):
    assert _error_code(service.update_triage("alice", 77, description="x")) == "not_found"
    assert fake.mutations() == []


def test_update_unknown_outcome_is_not_replayed(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("PUT", "/api/component_readiness/triages/5")] = [TransportError("TimeoutError")]
    result = service.update_triage("alice", 5, description="x")
    assert _error_code(result) == "outcome_unknown"
    assert len(fake.mutations()) == 1


def test_update_authorization_rejection_makes_no_change(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("PUT", "/api/component_readiness/triages/5")] = [_failure(401, "authentication is required")]
    assert _error_code(service.update_triage("alice", 5, description="x")) == "unauthenticated"
    assert fake.triages[5]["description"] == "original"


def test_update_verification_failure(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("PUT", "/api/component_readiness/triages/5")] = [_json(200, {})]  # accepted but not applied
    result = service.update_triage("alice", 5, description="x")
    assert _error_code(result) == "verification_failed"
    assert result["error"]["mismatches"] == [{"field": "description", "expected": "x", "actual": "original"}]


def test_update_warns_about_concurrent_changes(service, fake):
    fake.add_triage(id=5, regressions=[11])
    original_put = fake.__call__

    def concurrent_put():
        response = original_put("PUT", f"{API_URL}/api/component_readiness/triages/5", {"X-Acting-For": "alice"},
                                json.dumps(fake.calls[-1]["body"]).encode(), 1)
        fake.triages[5]["url"] = "https://redhat.atlassian.net/browse/OCPBUGS-777"
        return response

    fake.scripted[("PUT", "/api/component_readiness/triages/5")] = [concurrent_put]
    result = service.update_triage("alice", 5, description="x")
    assert result["verified"] is True
    assert "url" in result["warnings"][0]


# --- delete -----------------------------------------------------------------------------------


def test_delete_verifies_not_found(service, fake):
    fake.add_triage(id=5, regressions=[11])
    result = service.delete_triage("alice", 5)
    assert result["deleted"] is True and result["verified"] is True
    assert result["deleted_triage"]["id"] == 5
    assert [c["method"] for c in fake.calls] == ["GET", "DELETE", "GET"]
    assert fake.audit == [("delete", 5, "alice")]


def test_delete_nonexistent(service, fake):
    assert _error_code(service.delete_triage("alice", 5)) == "not_found"
    assert fake.mutations() == []


def test_delete_still_present_is_reported(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("DELETE", "/api/component_readiness/triages/5")] = [_json(200, None)]
    result = service.delete_triage("alice", 5)
    assert _error_code(result) == "verification_failed"


def test_delete_unknown_outcome(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("DELETE", "/api/component_readiness/triages/5")] = [TransportError("TimeoutError")]
    assert _error_code(service.delete_triage("alice", 5)) == "outcome_unknown"
    assert len(fake.mutations()) == 1


def test_delete_forbidden_makes_no_change(service, fake):
    fake.add_triage(id=5)
    fake.scripted[("DELETE", "/api/component_readiness/triages/5")] = [_failure(403, "forbidden")]
    assert _error_code(service.delete_triage("alice", 5)) == "forbidden"
    assert 5 in fake.triages


def test_read_transport_failure(service, fake):
    fake.scripted[("GET", "/api/component_readiness/triages/5")] = [TransportError("ConnectionRefusedError")]
    assert _error_code(service.get_triage(5)) == "upstream_unavailable"
