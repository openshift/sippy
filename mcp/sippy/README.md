# Sippy MCP server

`sippy_mcp_server.py` is a [FastMCP](https://github.com/jlowin/fastmcp) server named **`sippy`** for
working with production Sippy data through Sippy's authenticated REST API. It is separate from the
**`sippy-dev`** development server in [`mcp/server.py`](../server.py) and shares no code with it.
Triage management is its first toolset.

| File | Purpose |
| --- | --- |
| `sippy_mcp_server.py` | Entry point, toolset registry, transport settings |
| `sippy_client.py` | Configuration, authenticated HTTP client, error mapping, shared validation |
| `triage_api.py` | Triage toolset operations |
| `Dockerfile`, `requirements.txt`, `requirements.lock` | Image and locked runtime dependencies |

## Authentication and identity

The server runs behind an authenticating proxy and calls Sippy with a service-account bearer token.
Mutating tools take a required `acting_for` argument, sent as `X-Acting-For`. Sippy honors that
header only when the authenticated principal is listed with `--trusted-delegator` (see
[config/README.md](../../config/README.md#trusted-delegators)), so changes are authorized and
audited as the requesting user. Read tools take no identity.

Consumers must take `acting_for` from their own authenticated user context, never from text a user
typed. It must be non-blank printable ASCII of at most 256 characters. Anything that can reach this
server's listener can act with the service account's write access, so keep it behind the proxy.

## Configuration

Missing or invalid required values make the server exit with status 2 before serving.

| Variable | Default | Purpose |
| --- | --- | --- |
| `SIPPY_MCP_API_URL` | required | Authenticated Sippy API base URL. HTTPS unless loopback; no credentials, query, or fragment. |
| `SIPPY_MCP_AUTH_TOKEN_FILE` | required | Bearer token file, re-read on every request so rotation applies. |
| `SIPPY_MCP_REQUEST_TIMEOUT` | `30` | Seconds (0 to 300) bounding each whole Sippy request. |
| `SIPPY_MCP_TRANSPORT` | `stdio` | `stdio` or `streamable-http` (the image uses `streamable-http`). |
| `SIPPY_MCP_HTTP_HOST` | `127.0.0.1` | Listen address for `streamable-http`. |
| `SIPPY_MCP_HTTP_PORT` | `8090` | Listen port. The MCP endpoint is `/mcp`; `/healthz` returns `{"status": "ok"}`. |
| `SIPPY_MCP_ALLOWED_HOSTS` | none | Extra comma-separated `Host` values to accept; others get `421`. |

Tokens are never logged or returned, redirects are not followed, and responses over 32 MiB are
rejected.

## Tools

Tools return JSON objects; failures return `{"error": {"code": ..., "message": ...}}`. Arguments are
validated strictly (no type coercion), and mutations are never retried.

### Triage

| Tool | REST mapping | Notes |
| --- | --- | --- |
| `list_triages` | `GET /api/component_readiness/triages` | Filters: `view`, `include_archived`, `type`, `status`, `url_contains`, `regression_id`, `limit` (1 to 200). |
| `get_triage` | `GET .../triages/{id}` | Includes linked regression summaries. |
| `create_triage` | `POST .../triages` | `url`, `type`, optional `description` and `regression_ids`. |
| `update_triage` | `GET` then `PUT .../triages/{id}` | Partial update; omitted fields and links are preserved. Last writer wins. |
| `delete_triage` | `DELETE .../triages/{id}` | Returns `deleted` and `triage_id`. |

### Error codes

| Code | Meaning |
| --- | --- |
| `invalid_input` | Rejected locally or by Sippy (`400`); nothing changed. |
| `not_found` | The record does not exist (`404`). |
| `unauthenticated`, `forbidden` | Credentials rejected (`401`, `403`, or a redirect). |
| `unsupported` | Sippy lacks the capability, for example write endpoints are disabled (`501`). |
| `configuration_error` | The token file is missing, unreadable, or malformed. |
| `upstream_error` | Unexpected Sippy failure. |
| `upstream_unavailable` | A read received no response. |
| `outcome_unknown` | A mutation received no complete response; read current state before retrying. |

## Adding a toolset

Write a module with the operations (build requests with `SippyClient.send`, wrap tool methods with
`tool_result`, validate identities with `validate_acting_for`), write a register function that adds
its tools to the `FastMCP` server, and add a `Toolset` entry to `TOOLSETS` in
`sippy_mcp_server.py`. Add the module to the `Dockerfile` `COPY` line.

## Running and testing

```bash
export SIPPY_MCP_API_URL=https://sippy-auth.example.com
export SIPPY_MCP_AUTH_TOKEN_FILE=/path/to/token
mcp/.venv/bin/python mcp/sippy/sippy_mcp_server.py   # stdio; set SIPPY_MCP_TRANSPORT for HTTP
```

`make test` runs the unit tests (`pytest sippy/` from `mcp/`). They use a fake transport and never
contact a real Sippy. Server-side delegation is covered by
`test/e2e/componentreadiness/triage/delegation_test.go`.

`make sippy-mcp-image` builds the image (override the tag with `SIPPY_MCP_IMAGE`). It contains only
the runtime dependencies and server modules, runs as UID 1001, and holds no credentials or
delegator configuration. Deployment is managed outside this repository.
