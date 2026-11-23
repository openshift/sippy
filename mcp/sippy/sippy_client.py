"""Authenticated Sippy API client shared by every Sippy MCP toolset.

Every request goes to the authenticated Sippy API with the server's service-account bearer token.
Mutating tools also send the delegated user in X-Acting-For. Sippy resolves that header only for
configured trusted delegators, so authorization and audit records name the requesting user rather
than this server. Toolset modules (for example triage_api.py) build on SippyClient.send, the tool
result wrapper, and the shared validators here.
"""

from __future__ import annotations

import contextvars
import http.client
import ipaddress
import json
import os
import re
import socket
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

API_URL_ENV = "SIPPY_MCP_API_URL"
TOKEN_FILE_ENV = "SIPPY_MCP_AUTH_TOKEN_FILE"
TIMEOUT_ENV = "SIPPY_MCP_REQUEST_TIMEOUT"

DEFAULT_TIMEOUT_SECONDS = 30.0
MAX_ACTING_FOR_CHARS = 256
MAX_ERROR_MESSAGE_CHARS = 500
MAX_ID = 2**63 - 1
# Bounds memory for a single response. Some list endpoints (for example triages) are unpaginated, so
# this is sized well above a realistic full listing while still stopping a runaway body.
MAX_RESPONSE_BYTES = 32 * 1024 * 1024
_READ_CHUNK_BYTES = 64 * 1024

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


def _shutdown(sock: socket.socket) -> None:
    try:
        # The base-class shutdown, because SSLSocket.shutdown also discards TLS state that the
        # request thread may be reading through.
        socket.socket.shutdown(sock, socket.SHUT_RDWR)
    except OSError:
        pass  # already closed


class _RequestDeadline:
    """Shuts a request's socket down once its total deadline passes.

    urllib's timeout applies to each socket operation, and a single http.client call can make many
    of them (a proxy CONNECT response, the TLS handshake, headers, chunk sizes, or trailers), so a
    peer trickling bytes could hold a request open indefinitely. Shutting the socket down unblocks
    any read in progress.
    """

    def __init__(self, timeout: float, clock: Callable[[], float] = time.monotonic) -> None:
        # The timer starts after this, so it never fires before the deadline a reader checks.
        self.at = clock() + timeout
        self.expired = False
        self._lock = threading.Lock()
        self._sockets: list[socket.socket] = []
        self._timer = threading.Timer(timeout, self._expire)
        self._timer.daemon = True

    def __enter__(self) -> _RequestDeadline:
        self._timer.start()
        return self

    def __exit__(self, *exc_info: object) -> None:
        self._timer.cancel()
        with self._lock:
            sockets, self._sockets = self._sockets, []
            for sock in sockets:
                sock.close()

    def watch(self, sock: socket.socket) -> None:
        """Bound every later read on sock's connection, including after TLS wraps it.

        TLS wrapping detaches the original socket object and completes the handshake before
        returning the wrapped one. A duplicate descriptor still refers to the same connection, so
        shutting it down unblocks the handshake and every read on the wrapped socket.
        """
        watched = sock.dup()
        with self._lock:
            self._sockets.append(watched)
            if self.expired:
                _shutdown(watched)

    def _expire(self) -> None:
        # Shutting down under the lock keeps __exit__ from closing a descriptor mid-shutdown.
        with self._lock:
            self.expired = True
            for sock in self._sockets:
                _shutdown(sock)


# The deadline of the request being sent on this thread; connections register their sockets with it.
_active_deadline: contextvars.ContextVar[_RequestDeadline | None] = contextvars.ContextVar("_active_deadline", default=None)


class _DeadlineConnectionMixin:
    """Registers each new TCP socket with the active deadline before anything is read from it.

    connect() reads an inherited proxy's CONNECT response and completes the TLS handshake before it
    returns, so registering afterwards would leave both unbounded.
    """

    def __init__(self, *args: Any, **kwargs: Any) -> None:
        super().__init__(*args, **kwargs)
        create_connection = self._create_connection

        def create_watched_connection(*conn_args: Any, **conn_kwargs: Any) -> socket.socket:
            sock = create_connection(*conn_args, **conn_kwargs)
            deadline = _active_deadline.get()
            if deadline is not None:
                try:
                    deadline.watch(sock)
                except OSError:
                    sock.close()
                    raise
            return sock

        self._create_connection = create_watched_connection


class _DeadlineHTTPConnection(_DeadlineConnectionMixin, http.client.HTTPConnection):
    pass


class _DeadlineHTTPSConnection(_DeadlineConnectionMixin, http.client.HTTPSConnection):
    pass


class _DeadlineHTTPHandler(urllib.request.HTTPHandler):
    def http_open(self, req):  # noqa: D102
        return self.do_open(_DeadlineHTTPConnection, req)


class _DeadlineHTTPSHandler(urllib.request.HTTPSHandler):
    def https_open(self, req):  # noqa: D102
        # No context argument, so TLS verification keeps http.client's defaults.
        return self.do_open(_DeadlineHTTPSConnection, req)


_HANDLERS = (_DeadlineHTTPHandler, _DeadlineHTTPSHandler, _NoRedirect)
_OPENER = urllib.request.build_opener(*_HANDLERS)
# Plain HTTP is only allowed for loopback, so it must never be sent through an inherited proxy, which
# would carry the bearer token in cleartext off the host.
_DIRECT_OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), *_HANDLERS)


def _opener_for(url: str) -> urllib.request.OpenerDirector:
    return _DIRECT_OPENER if _is_loopback(urllib.parse.urlsplit(url).hostname or "") else _OPENER


def read_body(response: Any, deadline: float, clock: Callable[[], float] = time.monotonic) -> bytes:
    """Read a response body before deadline (a clock() value) and within MAX_RESPONSE_BYTES.

    The deadline is checked after every read, so an EOF caused by _RequestDeadline shutting the
    socket down is reported as a timeout rather than as the end of the body.
    """
    chunks: list[bytes] = []
    size = 0
    while True:
        chunk = response.read1(_READ_CHUNK_BYTES)
        if clock() >= deadline:
            raise TimeoutError("request deadline exceeded")
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
    """Send one request, bounding everything after connect by a total deadline of timeout seconds."""
    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    with _RequestDeadline(timeout) as deadline:
        active = _active_deadline.set(deadline)
        # Failures while reading either a success or an error body (for example IncompleteRead or a
        # timeout) mean no complete response arrived, so they are reported as an unknown outcome.
        try:
            try:
                with _opener_for(url).open(request, timeout=timeout) as response:
                    return HTTPResponse(response.status, read_body(response, deadline.at))
            except urllib.error.HTTPError as e:
                try:
                    return HTTPResponse(e.code, read_body(e, deadline.at))
                finally:
                    e.close()
        except (urllib.error.URLError, http.client.HTTPException, OSError) as e:
            # A shut-down socket surfaces as whatever error the interrupted read produced.
            raise TransportError("TimeoutError" if deadline.expired else type(e).__name__) from None
        finally:
            _active_deadline.reset(active)


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

    def send(
        self,
        method: str,
        path: str,
        acting_for: str | None = None,
        *,
        mutation: bool,
        subject: str,
        not_found: str | None = None,
        details: Mapping[str, Any] | None = None,
        body: dict[str, Any] | None = None,
        query: dict[str, str] | None = None,
    ) -> HTTPResponse:
        """Send a request and convert transport failures and non-200 statuses into ToolFailure.

        subject names the record in unknown-outcome hints (for example "triage"), not_found replaces
        Sippy's 404 message, and details are added to every error result (for example a record ID).
        Mutations are never retried; a mutation without a complete response is reported as
        outcome_unknown.
        """
        extra = dict(details or {})
        try:
            response = self.request(method, path, acting_for, body=body, query=query)
        except TransportError as e:
            if mutation:
                raise ToolFailure("outcome_unknown", f"No response from Sippy ({e}). {unknown_outcome_hint(subject)}", **extra) from None
            raise ToolFailure("upstream_unavailable", f"No response from Sippy ({e})", **extra) from None
        raise_for_status(response, mutation=mutation, subject=subject, not_found=not_found, **extra)
        return response


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


def unknown_outcome_hint(subject: str) -> str:
    return f"The change may or may not have been applied; read the {subject} before retrying."


def raise_for_status(response: HTTPResponse, *, mutation: bool, subject: str, not_found: str | None = None, **details: Any) -> None:
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
        raise ToolFailure("invalid_input", message, status=status, **details)
    if status == 401:
        raise ToolFailure("unauthenticated", message, status=status)
    if status == 403:
        raise ToolFailure("forbidden", message, status=status)
    if status == 404:
        raise ToolFailure("not_found", not_found or message, status=status, **details)
    if status == 501:
        raise ToolFailure("unsupported", "This Sippy server does not support the request (are write endpoints enabled?)", status=status)
    if mutation and status >= 500:
        raise ToolFailure("upstream_error", f"{message}. {unknown_outcome_hint(subject)}", status=status, **details)
    raise ToolFailure("upstream_error", message, status=status, **details)


def decode_json(response: HTTPResponse) -> Any:
    try:
        return json.loads(response.body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ToolFailure("upstream_error", "Sippy returned a response that is not valid JSON") from None


# --- shared input validation and tool results -------------------------------------------------


def invalid(message: str) -> ToolFailure:
    return ToolFailure("invalid_input", message)


def validate_acting_for(acting_for: Any) -> str:
    """The delegated identity must be supplied by a trusted consumer from its authenticated context."""
    if not isinstance(acting_for, str) or not acting_for.strip():
        raise invalid("acting_for is required and must identify the requesting user")
    value = acting_for.strip()
    if len(value) > MAX_ACTING_FOR_CHARS:
        raise invalid(f"acting_for must be at most {MAX_ACTING_FOR_CHARS} characters")
    # HTTP header values are not reliably transported beyond ASCII, and an audit identity must never be
    # silently altered, so anything else is rejected rather than encoded or replaced.
    if not value.isascii() or not value.isprintable():
        raise invalid("acting_for must contain only printable ASCII characters")
    return value


def validate_id(value: Any, name: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or not 0 < value <= MAX_ID:
        raise invalid(f"{name} must be a positive integer")
    return value


def tool_result(fn: Callable[..., dict[str, Any]]) -> Callable[..., dict[str, Any]]:
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

