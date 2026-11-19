"""Sippy triage REST client and domain operations for the Sippy MCP server.

Every request goes to the authenticated Sippy API with the server's service-account bearer token.
Mutating tools also send the delegated user in X-Acting-For. Sippy resolves that header only for
configured trusted delegators, so authorization and triage audit records name the requesting user
rather than this server. Read tools need no delegated user.
"""

from __future__ import annotations

import http.client
import ipaddress
import json
import logging
import os
import re
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

API_URL_ENV = "SIPPY_MCP_API_URL"
TOKEN_FILE_ENV = "SIPPY_MCP_AUTH_TOKEN_FILE"
TIMEOUT_ENV = "SIPPY_MCP_REQUEST_TIMEOUT"

TRIAGES_PATH = "/api/component_readiness/triages"
TRIAGE_TYPES = ("ci-infra", "product-infra", "product", "test")
TRIAGE_STATUSES = ("open", "resolved")

DEFAULT_TIMEOUT_SECONDS = 30.0
DEFAULT_LIST_LIMIT = 50
MAX_LIST_LIMIT = 200
MAX_ACTING_FOR_CHARS = 256
MAX_ERROR_MESSAGE_CHARS = 500
MAX_ID = 2**63 - 1
# Bounds memory for a single response. The triage list endpoint is unpaginated, so this is sized well
# above a realistic full listing while still stopping a runaway body.
MAX_RESPONSE_BYTES = 32 * 1024 * 1024
_READ_CHUNK_BYTES = 64 * 1024

# Go's zero time; Sippy decodes {"Valid": false} with this value as an unresolved triage.
_UNRESOLVED = {"Time": "0001-01-01T00:00:00Z", "Valid": False}
# Presentation-only fields a GET returns that must not be echoed into a PUT replacement body.
_READ_ONLY_FIELDS = ("links", "bug")
# Scalar fields update_triage can replace directly from tool arguments.
_EDITABLE_FIELDS = ("url", "description", "type")
_LOOPBACK_HOSTNAMES = ("localhost",)
# RFC 6750 b64token, the only form a bearer credential may take in an Authorization header.
_BEARER_TOKEN = re.compile(r"[A-Za-z0-9\-._~+/]+=*")


class ConfigurationError(Exception):
    """Required runtime configuration is missing or invalid. Messages never contain secrets."""


class TransportError(Exception):
    """The request could not be completed, so its outcome on the server is unknown."""


class ToolFailure(Exception):
    """A tool-level failure with a stable code that MCP callers can act on."""

    def __init__(self, code: str, message: str, **details: Any) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.details = details

    def to_dict(self) -> dict[str, Any]:
        error: dict[str, Any] = {"code": self.code, "message": self.message}
        error.update({k: v for k, v in self.details.items() if v is not None})
        return {"error": error}


@dataclass(frozen=True)
class HTTPResponse:
    status: int
    body: bytes


# (method, url, headers, body, timeout_seconds) -> HTTPResponse. Raises TransportError when no
# HTTP response was received. Tests substitute a fake transport at this seam.
Transport = Callable[[str, str, dict[str, str], bytes | None, float], HTTPResponse]


def _is_loopback(host: str) -> bool:
    if host.lower() in _LOOPBACK_HOSTNAMES:
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def validate_api_url(raw: str) -> str:
    """Return the normalized Sippy API base URL, rejecting values that could leak the bearer token.

    HTTPS is required except for loopback hosts, which tests and same-pod proxies use.
    """
    value = (raw or "").strip()
    if not value:
        raise ConfigurationError(f"{API_URL_ENV} is required; there is no default API endpoint")
    # urlsplit silently drops tabs and newlines and keeps spaces, so a malformed value would otherwise
    # be accepted as a different URL than the one configured.
    if any(ch.isspace() or not ch.isprintable() for ch in value):
        raise ConfigurationError(f"{API_URL_ENV} must not contain whitespace or control characters")
    # Request paths are appended to this value, so even an empty query or fragment would swallow them.
    if "?" in value or "#" in value:
        raise ConfigurationError(f"{API_URL_ENV} must not contain a query or fragment")
    try:
        parts = urllib.parse.urlsplit(value)
        hostname = parts.hostname
        parts.port  # noqa: B018 - raises ValueError for a non-numeric or out-of-range port
    except ValueError:
        raise ConfigurationError(f"{API_URL_ENV} must be a valid absolute http(s) URL") from None
    if parts.scheme not in ("https", "http") or not hostname:
        raise ConfigurationError(f"{API_URL_ENV} must be an absolute http(s) URL")
    if parts.username is not None or parts.password is not None:
        raise ConfigurationError(f"{API_URL_ENV} must not contain credentials")
    if parts.scheme == "http" and not _is_loopback(hostname):
        raise ConfigurationError(f"{API_URL_ENV} must use https unless it targets a loopback host")
    return value.rstrip("/")


def read_token(path: str) -> str:
    """Read the bearer token on every call so rotated service-account tokens take effect."""
    try:
        token = Path(path).read_text(encoding="utf-8").strip()
    except (OSError, UnicodeError) as e:
        raise ConfigurationError(f"the file named by {TOKEN_FILE_ENV} could not be read ({type(e).__name__})") from None
    if not token:
        raise ConfigurationError(f"the file named by {TOKEN_FILE_ENV} is empty")
    # http.client echoes an invalid header value in its exception, so never let one reach a request.
    if not _BEARER_TOKEN.fullmatch(token):
        raise ConfigurationError(f"the file named by {TOKEN_FILE_ENV} must contain a single bearer token")
    return token


@dataclass(frozen=True)
class SippyAPIConfig:
    api_url: str
    token_file: str
    timeout: float = DEFAULT_TIMEOUT_SECONDS

    @classmethod
    def from_env(cls, environ: Mapping[str, str] | None = None) -> SippyAPIConfig:
        """Load configuration, failing closed when anything required is missing or invalid."""
        env = os.environ if environ is None else environ
        api_url = validate_api_url(env.get(API_URL_ENV, ""))
        token_file = env.get(TOKEN_FILE_ENV, "").strip()
        if not token_file:
            raise ConfigurationError(f"{TOKEN_FILE_ENV} is required")
        raw_timeout = env.get(TIMEOUT_ENV, "").strip()
        timeout = DEFAULT_TIMEOUT_SECONDS
        if raw_timeout:
            try:
                timeout = float(raw_timeout)
            except ValueError:
                raise ConfigurationError(f"{TIMEOUT_ENV} must be a number of seconds") from None
            if not 0 < timeout <= 300:
                raise ConfigurationError(f"{TIMEOUT_ENV} must be between 0 and 300 seconds")
        return cls(api_url=api_url, token_file=token_file, timeout=timeout)


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuse redirects: urllib would resend Authorization to the new location, and an auth proxy
    redirect to a login page means the token was not accepted."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: D102
        return None


_OPENER = urllib.request.build_opener(_NoRedirect)
# Plain HTTP is only allowed for loopback, so it must never be sent through an inherited proxy, which
# would carry the bearer token in cleartext off the host.
_DIRECT_OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect)


def _opener_for(url: str) -> urllib.request.OpenerDirector:
    return _DIRECT_OPENER if _is_loopback(urllib.parse.urlsplit(url).hostname or "") else _OPENER


def read_body(response: Any, deadline: float, clock: Callable[[], float] = time.monotonic) -> bytes:
    """Read a response body before deadline (a clock() value) and within MAX_RESPONSE_BYTES.

    urllib's timeout applies to each socket operation, so a body trickled a byte at a time would
    never time out. read1 performs at most one socket read, which lets the deadline be checked
    between reads; a call can overrun it by at most one per-operation timeout.
    """
    chunks: list[bytes] = []
    size = 0
    while True:
        if clock() > deadline:
            raise TimeoutError("request deadline exceeded")
        chunk = response.read1(_READ_CHUNK_BYTES)
        if not chunk:
            # Unlike read(), read1 reports a body cut short of its Content-Length as a plain EOF,
            # leaving the unread byte count in length.
            remaining = getattr(response, "length", None)
            if isinstance(remaining, int) and remaining > 0:
                raise http.client.IncompleteRead(b"".join(chunks), remaining)
            return b"".join(chunks)
        size += len(chunk)
        if size > MAX_RESPONSE_BYTES:
            raise TransportError(f"response body exceeded {MAX_RESPONSE_BYTES} bytes")
        chunks.append(chunk)


def urllib_transport(method: str, url: str, headers: dict[str, str], body: bytes | None, timeout: float) -> HTTPResponse:
    """Send one request, bounding the body read by a total deadline of timeout seconds."""
    deadline = time.monotonic() + timeout
    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    # Failures while reading either a success or an error body (for example IncompleteRead or a
    # timeout) mean no complete response arrived, so they are reported as an unknown outcome.
    try:
        try:
            with _opener_for(url).open(request, timeout=timeout) as response:
                return HTTPResponse(response.status, read_body(response, deadline))
        except urllib.error.HTTPError as e:
            try:
                return HTTPResponse(e.code, read_body(e, deadline))
            finally:
                e.close()
    except (urllib.error.URLError, http.client.HTTPException, OSError) as e:
        raise TransportError(type(e).__name__) from None


class SippyClient:
    """Sends authenticated, delegated requests to the Sippy API."""

    def __init__(self, config: SippyAPIConfig, transport: Transport = urllib_transport) -> None:
        self.config = config
        self._transport = transport

    def request(
        self,
        method: str,
        path: str,
        acting_for: str | None = None,
        *,
        body: dict[str, Any] | None = None,
        query: dict[str, str] | None = None,
    ) -> HTTPResponse:
        """Send a request; acting_for is sent as X-Acting-For when given (required by Sippy for mutations)."""
        token = read_token(self.config.token_file)
        url = self.config.api_url + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
        }
        if acting_for is not None:
            headers["X-Acting-For"] = acting_for
        data = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            data = json.dumps(body).encode("utf-8")
        return self._transport(method, url, headers, data, self.config.timeout)


def _clean_text(text: str) -> str:
    cleaned = "".join(ch if ch.isprintable() else " " for ch in text).strip()
    if len(cleaned) > MAX_ERROR_MESSAGE_CHARS:
        cleaned = cleaned[:MAX_ERROR_MESSAGE_CHARS] + "..."
    return cleaned


def _upstream_message(response: HTTPResponse) -> str:
    """Extract Sippy's failureResponse message without echoing arbitrary (possibly HTML) bodies."""
    try:
        parsed = json.loads(response.body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return f"HTTP {response.status}"
    if isinstance(parsed, dict) and isinstance(parsed.get("message"), str):
        return _clean_text(parsed["message"])
    return f"HTTP {response.status}"


_UNKNOWN_OUTCOME_HINT = "The change may or may not have been applied; read the triage before retrying."


def _raise_for_status(response: HTTPResponse, *, mutation: bool, triage_id: int | None = None) -> None:
    status = response.status
    if status == 200:
        return
    message = _upstream_message(response)
    if 300 <= status < 400:
        raise ToolFailure(
            "unauthenticated",
            "Sippy redirected the request, which usually means the bearer token was not accepted by "
            f"the authentication proxy. Check {API_URL_ENV} and {TOKEN_FILE_ENV}.",
            status=status,
        )
    if status == 400:
        raise ToolFailure("invalid_input", message, status=status, triage_id=triage_id)
    if status == 401:
        raise ToolFailure("unauthenticated", message, status=status)
    if status == 403:
        raise ToolFailure("forbidden", message, status=status)
    if status == 404:
        raise ToolFailure("not_found", f"triage {triage_id} not found" if triage_id else message, status=status, triage_id=triage_id)
    if status == 501:
        raise ToolFailure("unsupported", "This Sippy server does not support the request (are write endpoints enabled?)", status=status)
    if mutation and status >= 500:
        raise ToolFailure("upstream_error", f"{message}. {_UNKNOWN_OUTCOME_HINT}", status=status, triage_id=triage_id)
    raise ToolFailure("upstream_error", message, status=status, triage_id=triage_id)


def _decode_json(response: HTTPResponse) -> Any:
    try:
        return json.loads(response.body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ToolFailure("upstream_error", "Sippy returned a response that is not valid JSON") from None


def _null_time(value: Any) -> str | None:
    """Convert Go sql.NullTime JSON ({"Time": ..., "Valid": ...}) to an RFC 3339 string or None."""
    if isinstance(value, dict):
        return value.get("Time") if value.get("Valid") else None
    return value if isinstance(value, str) else None


def _parse_time(value: str) -> datetime:
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return parsed


def _regression_ids(raw: Mapping[str, Any]) -> list[int]:
    return [r["id"] for r in raw.get("regressions") or [] if isinstance(r, dict) and isinstance(r.get("id"), int)]


def summarize_regression(raw: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "id": raw.get("id"),
        "test_id": raw.get("test_id"),
        "test_name": raw.get("test_name"),
        "release": raw.get("release"),
        "base_release": raw.get("base_release"),
        "component": raw.get("component"),
        "capability": raw.get("capability"),
        "variants": raw.get("variants") or [],
        "opened": raw.get("opened"),
        "closed": _null_time(raw.get("closed")),
    }


def summarize_triage(raw: Mapping[str, Any], *, include_regressions: bool) -> dict[str, Any]:
    """Return a stable, compact triage representation for tool output."""
    bug = raw.get("bug")
    if not isinstance(bug, dict):
        bug = None
    summary: dict[str, Any] = {
        "id": raw.get("id"),
        "url": raw.get("url"),
        "description": raw.get("description") or "",
        "type": raw.get("type"),
        "resolved": _null_time(raw.get("resolved")),
        "resolution_reason": raw.get("resolution_reason") or None,
        "bug_id": raw.get("bug_id"),
        "bug": {k: bug.get(k) for k in ("key", "status", "summary", "url")} if bug else None,
        "regression_ids": sorted(_regression_ids(raw)),
        "created_at": raw.get("created_at"),
        "updated_at": raw.get("updated_at"),
        "links": raw.get("links") or {},
    }
    if include_regressions:
        summary["regressions"] = [summarize_regression(r) for r in raw.get("regressions") or [] if isinstance(r, dict)]
    return summary


# --- input validation -------------------------------------------------------------------------


def _invalid(message: str) -> ToolFailure:
    return ToolFailure("invalid_input", message)


def validate_acting_for(acting_for: Any) -> str:
    """The delegated identity must be supplied by a trusted consumer from its authenticated context."""
    if not isinstance(acting_for, str) or not acting_for.strip():
        raise _invalid("acting_for is required and must identify the requesting user")
    value = acting_for.strip()
    if len(value) > MAX_ACTING_FOR_CHARS:
        raise _invalid(f"acting_for must be at most {MAX_ACTING_FOR_CHARS} characters")
    # HTTP header values are not reliably transported beyond ASCII, and an audit identity must never be
    # silently altered, so anything else is rejected rather than encoded or replaced.
    if not value.isascii() or not value.isprintable():
        raise _invalid("acting_for must contain only printable ASCII characters")
    return value


def validate_id(value: Any, name: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or not 0 < value <= MAX_ID:
        raise _invalid(f"{name} must be a positive integer")
    return value


def validate_id_list(values: Any, name: str) -> list[int]:
    if not isinstance(values, list):
        raise _invalid(f"{name} must be a list of positive integers")
    if not values:
        raise _invalid(f"{name} must not be empty; omit it to leave regression associations unchanged")
    ids = [validate_id(v, f"each entry in {name}") for v in values]
    if len(set(ids)) != len(ids):
        raise _invalid(f"{name} must not contain duplicates")
    return ids


def validate_url(value: Any) -> str:
    if not isinstance(value, str) or not value.strip():
        raise _invalid("url is required and must not be blank")
    url = value.strip()
    message = "url must be an absolute http(s) URL, typically a Jira issue"
    try:
        parts = urllib.parse.urlsplit(url)
        hostname = parts.hostname
        parts.port  # noqa: B018 - raises ValueError for a non-numeric or out-of-range port
    except ValueError:
        raise _invalid(message) from None
    if parts.scheme not in ("http", "https") or not hostname:
        raise _invalid(message)
    return url


def validate_type(value: Any) -> str:
    if value not in TRIAGE_TYPES:
        raise _invalid(f"type must be one of: {', '.join(TRIAGE_TYPES)}")
    return value


def validate_description(value: Any) -> str:
    if not isinstance(value, str):
        raise _invalid("description must be a string")
    return value


def validate_resolved(value: Any) -> str:
    """Normalize an RFC 3339 resolution timestamp to UTC."""
    if not isinstance(value, str) or not value.strip():
        raise _invalid("resolved must be an RFC 3339 timestamp with a timezone; use clear_resolved to unresolve")
    try:
        parsed = _parse_time(value.strip())
    except ValueError:
        raise _invalid("resolved must be an RFC 3339 timestamp with a timezone, e.g. 2026-01-02T15:04:05Z") from None
    return parsed.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


# --- domain operations ------------------------------------------------------------------------


def _tool(fn: Callable[..., dict[str, Any]]) -> Callable[..., dict[str, Any]]:
    """Convert expected failures into structured error results; never include credentials."""

    def wrapper(*args: Any, **kwargs: Any) -> dict[str, Any]:
        try:
            return fn(*args, **kwargs)
        except ToolFailure as e:
            return e.to_dict()
        except ConfigurationError as e:
            return ToolFailure("configuration_error", str(e)).to_dict()

    wrapper.__name__ = fn.__name__
    wrapper.__doc__ = fn.__doc__
    return wrapper


class TriageService:
    """Triage CRUD over Sippy's existing REST API.

    Create and update return the record Sippy persisted, which the POST and PUT responses already
    contain. Mutations are never retried: when the outcome is unknown the caller is told to read state
    first.
    """

    def __init__(self, client: SippyClient) -> None:
        self.client = client

    def _send(self, method: str, path: str, acting_for: str | None, *, mutation: bool, triage_id: int | None = None, **kwargs: Any) -> HTTPResponse:
        try:
            response = self.client.request(method, path, acting_for, **kwargs)
        except TransportError as e:
            if mutation:
                raise ToolFailure(
                    "outcome_unknown",
                    f"No response from Sippy ({e}). {_UNKNOWN_OUTCOME_HINT}",
                    triage_id=triage_id,
                ) from None
            raise ToolFailure("upstream_unavailable", f"No response from Sippy ({e})", triage_id=triage_id) from None
        _raise_for_status(response, mutation=mutation, triage_id=triage_id)
        return response

    def _fetch(self, triage_id: int, acting_for: str | None = None) -> dict[str, Any]:
        response = self._send("GET", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=False, triage_id=triage_id)
        raw = _decode_json(response)
        if not isinstance(raw, dict) or raw.get("id") != triage_id:
            raise ToolFailure("upstream_error", "Sippy returned an unexpected triage record", triage_id=triage_id)
        return raw

    @_tool
    def list_triages(
        self,
        view: str | None = None,
        include_archived: bool = False,
        type: str | None = None,
        status: str | None = None,
        url_contains: str | None = None,
        regression_id: int | None = None,
        limit: int = DEFAULT_LIST_LIMIT,
    ) -> dict[str, Any]:
        query: dict[str, str] = {}
        if view is not None:
            if not isinstance(view, str) or not view.strip():
                raise _invalid("view must be a non-blank view name, e.g. 4.22-main")
            query["view"] = view.strip()
        if not isinstance(include_archived, bool):
            raise _invalid("include_archived must be a boolean")
        if include_archived:
            query["include_archived"] = "true"
        if type is not None:
            validate_type(type)
        if status is not None and status not in TRIAGE_STATUSES:
            raise _invalid(f"status must be one of: {', '.join(TRIAGE_STATUSES)}")
        if url_contains is not None and (not isinstance(url_contains, str) or not url_contains.strip()):
            raise _invalid("url_contains must be a non-blank string")
        if regression_id is not None:
            validate_id(regression_id, "regression_id")
        if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= MAX_LIST_LIMIT:
            raise _invalid(f"limit must be an integer between 1 and {MAX_LIST_LIMIT}")

        # Sippy's list endpoint has no paging or server-side filters beyond view and include_archived,
        # so the full listing is fetched (bounded by MAX_RESPONSE_BYTES) and filtered here.
        raw = _decode_json(self._send("GET", TRIAGES_PATH, None, mutation=False, query=query or None))
        if raw is None:
            raw = []
        if not isinstance(raw, list):
            raise ToolFailure("upstream_error", "Sippy returned an unexpected triage listing")

        needle = url_contains.strip().lower() if url_contains else None
        matches = []
        for record in raw:
            if not isinstance(record, dict):
                continue
            if type is not None and record.get("type") != type:
                continue
            resolved = _null_time(record.get("resolved")) is not None
            if status == "open" and resolved or status == "resolved" and not resolved:
                continue
            if needle and needle not in str(record.get("url", "")).lower():
                continue
            if regression_id is not None and regression_id not in _regression_ids(record):
                continue
            matches.append(record)
        matches.sort(key=lambda r: r.get("id") or 0, reverse=True)
        returned = [summarize_triage(r, include_regressions=False) for r in matches[:limit]]
        return {
            "triages": returned,
            "count": len(returned),
            "total_matches": len(matches),
            "truncated": len(matches) > limit,
        }

    @_tool
    def get_triage(self, triage_id: int) -> dict[str, Any]:
        triage_id = validate_id(triage_id, "triage_id")
        return {"triage": summarize_triage(self._fetch(triage_id), include_regressions=True)}

    @_tool
    def create_triage(
        self,
        acting_for: str,
        url: str,
        type: str,
        description: str | None = None,
        regression_ids: list[int] | None = None,
    ) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        body: dict[str, Any] = {"url": validate_url(url), "type": validate_type(type)}
        if description is not None:
            body["description"] = validate_description(description)
        ids = validate_id_list(regression_ids, "regression_ids") if regression_ids is not None else []
        body["regressions"] = [{"id": i} for i in ids]

        response = self._send("POST", TRIAGES_PATH, acting_for, mutation=True, body=body)
        try:
            created = _decode_json(response)
        except ToolFailure:
            created = None
        triage_id = created.get("id") if isinstance(created, dict) else None
        if not isinstance(triage_id, int) or triage_id <= 0:
            raise ToolFailure("outcome_unknown", f"Sippy accepted the request but returned no triage id. {_UNKNOWN_OUTCOME_HINT}")
        return {"triage": summarize_triage(created, include_regressions=True)}

    @_tool
    def update_triage(
        self,
        acting_for: str,
        triage_id: int,
        url: str | None = None,
        description: str | None = None,
        type: str | None = None,
        resolved: str | None = None,
        clear_resolved: bool = False,
        add_regression_ids: list[int] | None = None,
        remove_regression_ids: list[int] | None = None,
    ) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        triage_id = validate_id(triage_id, "triage_id")
        if not isinstance(clear_resolved, bool):
            raise _invalid("clear_resolved must be a boolean")
        changes: dict[str, Any] = {}
        if url is not None:
            changes["url"] = validate_url(url)
        if description is not None:
            changes["description"] = validate_description(description)
        if type is not None:
            changes["type"] = validate_type(type)
        if resolved is not None and clear_resolved:
            raise _invalid("resolved and clear_resolved are mutually exclusive")
        if resolved is not None:
            changes["resolved"] = validate_resolved(resolved)
        add = validate_id_list(add_regression_ids, "add_regression_ids") if add_regression_ids is not None else []
        remove = validate_id_list(remove_regression_ids, "remove_regression_ids") if remove_regression_ids is not None else []
        if set(add) & set(remove):
            raise _invalid("a regression id cannot be both added and removed")
        if not changes and not clear_resolved and not add and not remove:
            raise _invalid("no changes requested; supply at least one field to update")

        # Sippy's PUT replaces the whole record, so start from the complete current record.
        current = self._fetch(triage_id, acting_for)
        current_ids = _regression_ids(current)
        already_linked = sorted(set(add) & set(current_ids))
        if already_linked:
            raise _invalid(f"regressions already linked to triage {triage_id}: {already_linked}")
        not_linked = sorted(set(remove) - set(current_ids))
        if not_linked:
            raise _invalid(f"regressions not linked to triage {triage_id}: {not_linked}")

        body = {k: v for k, v in current.items() if k not in _READ_ONLY_FIELDS}
        new_ids = [i for i in current_ids if i not in remove] + add
        body["regressions"] = [{"id": i} for i in new_ids]
        for field in _EDITABLE_FIELDS:
            if field in changes:
                body[field] = changes[field]
        if "resolved" in changes:
            body["resolved"] = {"Time": changes["resolved"], "Valid": True}
        elif clear_resolved:
            body["resolved"] = dict(_UNRESOLVED)

        response = self._send("PUT", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=True, triage_id=triage_id, body=body)
        try:
            updated = _decode_json(response)
        except ToolFailure:
            updated = None
        if not isinstance(updated, dict) or updated.get("id") != triage_id:
            raise ToolFailure(
                "upstream_error",
                "Sippy accepted the update but returned an unexpected triage record; read the triage to see its state",
                triage_id=triage_id,
            )

        changed_fields = [k for k in _EDITABLE_FIELDS if k in changes]
        if "resolved" in changes or clear_resolved:
            changed_fields.append("resolved")
        if add or remove:
            changed_fields.append("regression_ids")
        return {"triage": summarize_triage(updated, include_regressions=True), "changed_fields": sorted(changed_fields)}

    @_tool
    def delete_triage(self, acting_for: str, triage_id: int) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        triage_id = validate_id(triage_id, "triage_id")
        self._send("DELETE", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=True, triage_id=triage_id)
        return {"deleted": True, "triage_id": triage_id}
