"""Production Sippy MCP server exposing component readiness triage management.

This entry point is separate from the development-workflow server (mcp/server.py, "sippy-dev") and
exposes no development commands. It is meant to run behind an authenticating proxy, holds a
service-account token for the authenticated Sippy API, and forwards each tool's acting_for value as
X-Acting-For so Sippy attributes changes to the requesting user. See README.md in this directory.
"""

from __future__ import annotations

import logging
import os
import sys
from collections.abc import Mapping
from dataclasses import dataclass

from fastmcp import FastMCP
from starlette.requests import Request
from starlette.responses import JSONResponse

from triage_api import (
    DEFAULT_LIST_LIMIT,
    ConfigurationError,
    SippyAPIConfig,
    SippyClient,
    TriageService,
    read_token,
)

logger = logging.getLogger(__name__)

SERVER_NAME = "sippy-production"
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
Manage Sippy component readiness triage records in production.

Every tool requires acting_for: the identity of the human the change is made for, taken from the
consumer's authenticated context (never from free text typed by a user). Sippy records that identity
in triage audit logs. Read a triage with get_triage before changing it, and confirm destructive
changes with the user. Mutations are not retried automatically; an outcome_unknown error means you
must read current state before trying again.
"""

_ACTING_FOR_DOC = (
    "acting_for: identity of the requesting user from the trusted consumer's authenticated context; "
    "sent as X-Acting-For and recorded in Sippy's audit log."
)


def _document_acting_for(fn):
    fn.__doc__ = f"{fn.__doc__.rstrip()}\n\n{_ACTING_FOR_DOC}"
    return fn


def register_triage_tools(server: FastMCP, service: TriageService) -> None:
    @server.tool()
    @_document_acting_for
    def list_triages(
        acting_for: str,
        view: str | None = None,
        include_archived: bool = False,
        type: str | None = None,
        status: str | None = None,
        url_contains: str | None = None,
        regression_id: int | None = None,
        limit: int = DEFAULT_LIST_LIMIT,
    ) -> dict:
        """List or search triage records, newest first (GET /api/component_readiness/triages).

        view and include_archived are applied by Sippy; type (ci-infra, product-infra, product, test),
        status (open or resolved), url_contains (case-insensitive), and regression_id (linked
        regression) filter the result. limit is 1-200 (default 50). Returns triages (summaries with
        regression_ids), count, total_matches, and truncated. No matches returns an empty list.
        """
        return service.list_triages(
            acting_for,
            view=view,
            include_archived=include_archived,
            type=type,
            status=status,
            url_contains=url_contains,
            regression_id=regression_id,
            limit=limit,
        )

    @server.tool()
    @_document_acting_for
    def get_triage(acting_for: str, triage_id: int) -> dict:
        """Fetch one triage with its linked regressions (GET /api/component_readiness/triages/{id}).

        Returns {"triage": ...} or an error with code not_found.
        """
        return service.get_triage(acting_for, triage_id)

    @server.tool()
    @_document_acting_for
    def create_triage(
        acting_for: str,
        url: str,
        type: str,
        description: str | None = None,
        regression_ids: list[int] | None = None,
    ) -> dict:
        """Create a triage (POST /api/component_readiness/triages) and verify it by reading it back.

        url is required (typically a Jira issue). type is ci-infra, product-infra, product, or test.
        regression_ids links existing regressions (omit for none). Returns {"triage": ..., "verified": true}.
        """
        return service.create_triage(acting_for, url, type, description=description, regression_ids=regression_ids)

    @server.tool()
    @_document_acting_for
    def update_triage(
        acting_for: str,
        triage_id: int,
        url: str | None = None,
        description: str | None = None,
        type: str | None = None,
        resolved: str | None = None,
        clear_resolved: bool = False,
        add_regression_ids: list[int] | None = None,
        remove_regression_ids: list[int] | None = None,
    ) -> dict:
        """Partially update a triage; omitted (or null) fields and regression links are preserved.

        Reads the full record, applies only the supplied changes, replaces it with
        PUT /api/component_readiness/triages/{id}, then verifies by reading it back.
        description="" clears the description. resolved is an RFC 3339 timestamp that marks the
        triage resolved; clear_resolved=true unresolves it (not both). add_regression_ids links
        regressions that are not yet linked; remove_regression_ids unlinks currently linked ones.
        Empty lists, overlapping ids, and requests with no changes are rejected. Not atomic: a
        concurrent update between the read and the write can be overwritten (reported as a warning
        when detected).
        """
        return service.update_triage(
            acting_for,
            triage_id,
            url=url,
            description=description,
            type=type,
            resolved=resolved,
            clear_resolved=clear_resolved,
            add_regression_ids=add_regression_ids,
            remove_regression_ids=remove_regression_ids,
        )

    @server.tool()
    @_document_acting_for
    def delete_triage(acting_for: str, triage_id: int) -> dict:
        """Permanently delete a triage (DELETE /api/component_readiness/triages/{id}).

        Verifies removal by confirming a subsequent read returns not found. Returns the deleted
        record's summary. Prefer resolving a triage (update_triage with resolved) over deleting it.
        """
        return service.delete_triage(acting_for, triage_id)



def build_server(service: TriageService) -> FastMCP:
    server = FastMCP(SERVER_NAME, instructions=_INSTRUCTIONS)
    register_triage_tools(server, service)

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


def load_service(environ: Mapping[str, str] | None = None) -> TriageService:
    """Build the service from the environment and confirm the token is readable before serving."""
    config = SippyAPIConfig.from_env(environ)
    read_token(config.token_file)
    return TriageService(SippyClient(config))


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    try:
        service = load_service()
        settings = runtime_settings_from_env()
    except ConfigurationError as e:
        print(f"{SERVER_NAME}: configuration error: {e}", file=sys.stderr)
        return 2

    server = build_server(service)
    logger.info("starting %s (transport=%s, api=%s)", SERVER_NAME, settings.transport, service.client.config.api_url)
    if settings.transport == "stdio":
        server.run(transport="stdio", show_banner=False)
    else:
        server.run(
            transport="streamable-http",
            host=settings.host,
            port=settings.port,
            show_banner=False,
            allowed_hosts=settings.allowed_hosts,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
