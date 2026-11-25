# Sippy MCP server

The `sippy_mcp` package is a [FastMCP](https://github.com/jlowin/fastmcp) server named **`sippy`** for
working with production Sippy data through Sippy's authenticated REST API. It is separate from the
**`sippy-dev`** development server in [`mcp/server.py`](../server.py) and shares no code with it.
Triage management is its first toolset.

| File | Purpose |
| --- | --- |
| `sippy_mcp/server.py` | Entry point (`python -m sippy_mcp`), toolset registry, transport settings |
| `sippy_mcp/client.py` | Configuration, authenticated HTTP client, error mapping, shared validation |
| `sippy_mcp/triage.py` | Triage toolset: operations, tool docstrings, and registration |
| `Dockerfile`, `requirements.txt`, `requirements.lock` | Image and locked runtime dependencies |

## Authentication and identity

The server runs behind an authenticating proxy and calls Sippy with a service-account bearer token.
Mutating tools take a required `acting_for` argument, sent as `X-Acting-For`. Sippy honors that
header only when the authenticated principal is listed with `--trusted-delegator` (see
[config/README.md](../../config/README.md#trusted-delegators)), so changes are attributed to and
audited as the requesting user. Read tools take no identity.

Consumers are other services (for example a chat bot or a Jira automation) that authenticate to the
proxy in front of this server with their own service account. The proxy must admit only those trusted
consumer service accounts, never end users directly: anything that reaches this server's listener can
act with its service account's write access.

A consumer must set `acting_for` in its own code from the user it authenticated, and must never let
the model choose the value. A model reading a Jira description or a chat message can be talked into
naming someone else, and Sippy would record that name as the author of the change. The value must be
non-blank printable ASCII of at most 256 characters.

## Configuration

Missing or invalid values make the server exit with status 2 before serving.

| Variable | Default | Purpose |
| --- | --- | --- |
| `SIPPY_MCP_API_URL` | required | Authenticated Sippy API base URL. HTTPS unless loopback; no credentials, query, or fragment. |
| `SIPPY_MCP_AUTH_TOKEN_FILE` | required | Bearer token file, re-read on every request so rotation applies. |
| `SIPPY_MCP_REQUEST_TIMEOUT` | `30` | Seconds (greater than 0, up to 300) bounding each whole Sippy request. |
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
| `list_triages` | `GET /api/component_readiness/triages` | Filters: `view`, `include_archived`, `type`, `status`, `url_contains`, `regression_id`, `limit` (1 to 200). With `include_archived`, the listing uses Sippy's `fields` selection to omit linked regressions (the full archived listing exceeds the response size cap), so `regression_ids` is `null` and `regression_id` is rejected. |
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
| `upstream_error` | Unexpected Sippy failure; for a mutation, read current state before retrying. |
| `upstream_unavailable` | A read received no response. |
| `outcome_unknown` | A mutation received no complete response; read current state before retrying. |

## Adding a toolset

Add a module to `sippy_mcp/` with async tool methods (build requests with `SippyClient.send`, wrap
each tool with `tool_result`, validate identities with `validate_acting_for`). Each method's docstring
and signature become the tool's description and schema. Give the module a register function that
passes the bound methods to `server.tool`, and add a `Toolset` entry to `TOOLSETS` in
`sippy_mcp/server.py`. The image copies the whole package, so the `Dockerfile` needs no change.

## Running and testing

```bash
export SIPPY_MCP_API_URL=https://sippy-auth.example.com
export SIPPY_MCP_AUTH_TOKEN_FILE=/path/to/token
cd mcp/sippy && ../.venv/bin/python -m sippy_mcp   # stdio; set SIPPY_MCP_TRANSPORT for HTTP
```

`make test` runs the unit tests (`pytest sippy/` from `mcp/`). They use a fake transport and never
contact a real Sippy. Server-side delegation is covered by
`test/e2e/componentreadiness/triage/delegation_test.go`.

`make sippy-mcp-image` builds the image (override the tag with `SIPPY_MCP_IMAGE`). It contains only
the runtime dependencies and the `sippy_mcp` package, runs as UID 1001, and holds no credentials or
delegator configuration. Deployment is managed outside this repository.
