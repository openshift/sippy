"""Authenticated Sippy API client shared by every Sippy MCP toolset.

Every request goes to the authenticated Sippy API with the server's service-account bearer token.
Mutating tools also send the delegated user in X-Acting-For. Sippy resolves that header only for
configured trusted delegators, so authorization and audit records name the requesting user rather
than this server. Toolset modules (for example triage.py) build on SippyClient.send, the tool
result wrapper, and the shared validators here.
"""

from __future__ import annotations

import dataclasses
import functools
import ipaddress
import json
import os
import re
import ssl
import urllib.parse
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import anyio
import httpx2

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
# Read-only methods, matching safeMethods in Sippy's delegation resolver. Every other method is a
# mutation, which is never retried and reports a missing response as an unknown outcome.
_SAFE_METHODS = frozenset({"GET", "HEAD", "OPTIONS"})

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
    # The request this answers, as "METHOD /path", so error messages can say which call failed.
    request: str = ""


# (method, url, headers, body, timeout_seconds) -> HTTPResponse. Raises TransportError when no
# complete HTTP response was received. Tests substitute a fake transport at this seam.
Transport = Callable[[str, str, dict[str, str], bytes | None, float], Awaitable[HTTPResponse]]


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
    # HTTP libraries can echo an invalid header value in their exceptions, so never let one reach a request.
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
                raise ConfigurationError(f"{TIMEOUT_ENV} must be greater than 0 and at most 300 seconds")
        return cls(api_url=api_url, token_file=token_file, timeout=timeout)


# The system trust store rather than a bundled CA set, so CAs installed in the image apply.
_SSL_CONTEXT = ssl.create_default_context()


def _http_client(url: str) -> httpx2.AsyncClient:
    """A client for one request. Redirects are not followed: they would resend Authorization to the new
    location, and an auth proxy redirect to a login page means the token was not accepted. Plain HTTP
    is only allowed for loopback, so loopback requests ignore any inherited proxy, which would carry the
    bearer token in cleartext off the host."""
    direct = _is_loopback(urllib.parse.urlsplit(url).hostname or "")
    return httpx2.AsyncClient(verify=_SSL_CONTEXT, trust_env=not direct, follow_redirects=False, timeout=None)


async def _read_body(response: httpx2.Response) -> bytes:
    chunks: list[bytes] = []
    size = 0
    async for chunk in response.aiter_bytes():
        size += len(chunk)
        if size > MAX_RESPONSE_BYTES:
            raise TransportError(f"response body exceeded {MAX_RESPONSE_BYTES} bytes")
        chunks.append(chunk)
    return b"".join(chunks)


async def httpx_transport(method: str, url: str, headers: dict[str, str], body: bytes | None, timeout: float) -> HTTPResponse:
    """Send one request within a total deadline of timeout seconds.

    The deadline covers everything, including an inherited proxy's CONNECT response, the TLS
    handshake, headers, and the body, so a peer trickling bytes cannot hold a request open.
    """
    try:
        with anyio.fail_after(timeout):
            async with _http_client(url) as client, client.stream(method, url, headers=headers, content=body) as response:
                return HTTPResponse(response.status_code, await _read_body(response))
    except (httpx2.HTTPError, httpx2.StreamError, OSError) as e:
        # TimeoutError (the deadline) is an OSError. Any failure before the whole body arrived means no
        # complete response, so the outcome is unknown.
        raise TransportError(type(e).__name__) from None


class SippyClient:
    """Sends authenticated, delegated requests to the Sippy API."""

    def __init__(self, config: SippyAPIConfig, transport: Transport = httpx_transport) -> None:
        self.config = config
        self._transport = transport

    async def request(
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
        response = await self._transport(method, url, headers, data, self.config.timeout)
        return dataclasses.replace(response, request=f"{method} {path}")

    async def send(
        self,
        method: str,
        path: str,
        acting_for: str | None = None,
        *,
        subject: str,
        not_found: str | None = None,
        details: Mapping[str, Any] | None = None,
        body: dict[str, Any] | None = None,
        query: dict[str, str] | None = None,
    ) -> HTTPResponse:
        """Send a request and convert transport failures and non-200 statuses into ToolFailure.

        subject names the record in unknown-outcome hints (for example "triage"), not_found replaces
        Sippy's 404 message, and details are added to every error result (for example a record ID).
        Mutations (any method other than GET, HEAD, or OPTIONS) are never retried; a mutation without
        a complete response is reported as outcome_unknown.
        """
        mutation = method not in _SAFE_METHODS
        extra = dict(details or {})
        try:
            response = await self.request(method, path, acting_for, body=body, query=query)
        except TransportError as e:
            message = f"No response from Sippy to {method} {path} ({e})"
            if mutation:
                raise ToolFailure("outcome_unknown", f"{message}. {unknown_outcome_hint(subject)}", **extra) from None
            raise ToolFailure("upstream_unavailable", message, **extra) from None
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
        raise ToolFailure("upstream_error", f"Sippy returned a response to {response.request} that is not valid JSON") from None


# --- shared input validation and tool results -------------------------------------------------


def invalid(message: str) -> ToolFailure:
    return ToolFailure("invalid_input", message)


def validate_acting_for(acting_for: Any) -> str:
    """The delegated identity must be supplied by a trusted consumer from its authenticated context."""
    value = acting_for.strip() if isinstance(acting_for, str) else ""
    if not value:
        raise invalid("acting_for is required and must identify the requesting user")
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


def tool_result(fn: Callable[..., Awaitable[dict[str, Any]]]) -> Callable[..., Awaitable[dict[str, Any]]]:
    """Convert expected failures into structured error results; never include credentials.

    functools.wraps keeps the name, docstring, and (through __wrapped__) the signature, so a decorated
    method can be registered with FastMCP directly.
    """

    @functools.wraps(fn)
    async def wrapper(*args: Any, **kwargs: Any) -> dict[str, Any]:
        try:
            return await fn(*args, **kwargs)
        except ToolFailure as e:
            return e.to_dict()
        except ConfigurationError as e:
            return ToolFailure("configuration_error", str(e)).to_dict()

    return wrapper

