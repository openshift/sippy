"""Sippy MCP server ("sippy") for working with production Sippy data.

This entry point is separate from the development-workflow server (mcp/server.py, "sippy-dev") and
exposes no development commands. It is meant to run behind an authenticating proxy, holds a
service-account token for the authenticated Sippy API, and forwards each mutating tool's acting_for
value as X-Acting-For so Sippy attributes changes to the requesting user.

Tools are grouped into toolsets (see TOOLSETS). To add a capability, write a module in this package
that registers its tools against a SippyClient and add a Toolset entry. This module only holds wiring.
See README.md in the parent directory.
"""

from __future__ import annotations

import logging
import os
import sys
from collections.abc import Callable, Mapping
from dataclasses import dataclass

from fastmcp import FastMCP
from starlette.requests import Request
from starlette.responses import JSONResponse

from sippy_mcp import triage
from sippy_mcp.client import ConfigurationError, SippyAPIConfig, SippyClient, read_token

logger = logging.getLogger(__name__)

SERVER_NAME = "sippy"
TRANSPORT_ENV = "SIPPY_MCP_TRANSPORT"
HOST_ENV = "SIPPY_MCP_HTTP_HOST"
PORT_ENV = "SIPPY_MCP_HTTP_PORT"
ALLOWED_HOSTS_ENV = "SIPPY_MCP_ALLOWED_HOSTS"

DEFAULT_TRANSPORT = "stdio"
# Loopback by default so only a same-pod authenticating proxy can reach the server; anything that can
# reach it can act with the service account's write access.
DEFAULT_HTTP_HOST = "127.0.0.1"
DEFAULT_HTTP_PORT = 8090
SUPPORTED_TRANSPORTS = ("stdio", "streamable-http")

_INSTRUCTIONS = """\
Work with production Sippy data.

Every tool that changes data requires acting_for: the identity of the human the change is made for,
taken from the consumer's authenticated context (never from free text typed by a user). Sippy records
that identity in its audit logs. Read tools need no identity. Read a record before changing it, and
confirm destructive changes with the user. Mutations are not retried automatically; an
outcome_unknown error means you must read current state before trying again.
"""


@dataclass(frozen=True)
class Toolset:
    """A group of tools for one area of Sippy data, with instructions that apply to it."""

    name: str
    instructions: str
    register: Callable[[FastMCP, SippyClient], None]


TOOLSETS: tuple[Toolset, ...] = (
    Toolset(name="triage", instructions=triage.INSTRUCTIONS, register=triage.register_tools),
)


def build_server(client: SippyClient, toolsets: tuple[Toolset, ...] = TOOLSETS) -> FastMCP:
    instructions = "\n".join([_INSTRUCTIONS, *(t.instructions for t in toolsets)])
    # Strict validation stops type coercion (for example true -> 1 for an id) before a tool runs.
    server = FastMCP(SERVER_NAME, instructions=instructions, strict_input_validation=True)
    for toolset in toolsets:
        toolset.register(server, client)

    @server.custom_route("/healthz", methods=["GET"], include_in_schema=False)
    async def healthz(_: Request) -> JSONResponse:
        return JSONResponse({"status": "ok"})

    return server


@dataclass(frozen=True)
class RuntimeSettings:
    transport: str
    host: str
    port: int
    allowed_hosts: list[str] | None


def runtime_settings_from_env(environ: Mapping[str, str] | None = None) -> RuntimeSettings:
    """Parse transport settings, rejecting invalid values instead of guessing."""
    env = os.environ if environ is None else environ
    transport = env.get(TRANSPORT_ENV, "").strip() or DEFAULT_TRANSPORT
    if transport not in SUPPORTED_TRANSPORTS:
        raise ConfigurationError(f"{TRANSPORT_ENV} must be one of: {', '.join(SUPPORTED_TRANSPORTS)}")
    host = env.get(HOST_ENV, "").strip() or DEFAULT_HTTP_HOST
    raw_port = env.get(PORT_ENV, "").strip()
    port = DEFAULT_HTTP_PORT
    if raw_port:
        try:
            port = int(raw_port)
        except ValueError:
            raise ConfigurationError(f"{PORT_ENV} must be an integer") from None
        if not 1 <= port <= 65535:
            raise ConfigurationError(f"{PORT_ENV} must be between 1 and 65535")
    allowed = [h.strip() for h in env.get(ALLOWED_HOSTS_ENV, "").split(",") if h.strip()]
    return RuntimeSettings(transport=transport, host=host, port=port, allowed_hosts=allowed or None)


def http_transport_options(settings: RuntimeSettings) -> dict:
    """Options that make the HTTP listener reject requests whose Host is not loopback, the bind address,
    or listed in SIPPY_MCP_ALLOWED_HOSTS. FastMCP ignores allowed_hosts unless protection is enabled."""
    return {"host_origin_protection": True, "allowed_hosts": settings.allowed_hosts}


def load_client(environ: Mapping[str, str] | None = None) -> SippyClient:
    """Build the Sippy client from the environment and confirm the token is readable before serving."""
    config = SippyAPIConfig.from_env(environ)
    read_token(config.token_file)
    return SippyClient(config)


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    try:
        client = load_client()
        settings = runtime_settings_from_env()
    except ConfigurationError as e:
        print(f"{SERVER_NAME}: configuration error: {e}", file=sys.stderr)
        return 2

    server = build_server(client)
    logger.info("starting %s (transport=%s, api=%s)", SERVER_NAME, settings.transport, client.config.api_url)
    if settings.transport == "stdio":
        server.run(transport="stdio", show_banner=False)
    else:
        server.run(
            transport="streamable-http",
            host=settings.host,
            port=settings.port,
            show_banner=False,
            **http_transport_options(settings),
        )
    return 0
