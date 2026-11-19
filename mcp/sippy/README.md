# Sippy MCP server

`sippy_mcp_server.py` is a [FastMCP](https://github.com/jlowin/fastmcp) server named **`sippy`**
that lets a trusted agent manage Component Readiness triage records through Sippy's existing triage
REST API. It is separate from the **`sippy-dev`** development-workflow server in
[`mcp/server.py`](../server.py): it exposes no development commands, does not load
`.devcontainer/.env`, and has no default API endpoint.

| File | Purpose |
| --- | --- |
| `sippy_mcp_server.py` | Entry point, tool registration, runtime (transport) settings |
| `triage_api.py` | Configuration, HTTP client, triage operations, readback verification |
| `Dockerfile` | Multi-stage image (runtime dependencies and the two modules above only, no build tools) |
| `requirements.txt`, `requirements.lock` | Runtime dependency and hashed lockfile |
| `test_*.py` | Unit, registration, transport, and integration tests |

## Authentication and caller identity contract

The server follows the ship-status trusted-delegation pattern.

1. The MCP server runs behind an authenticating proxy, so only authenticated consumers can reach
   it. It holds a service-account bearer token (`SIPPY_MCP_AUTH_TOKEN_FILE`) for the Sippy API.
2. Every tool call sends `Authorization: Bearer <token>` to `SIPPY_MCP_API_URL`, which must be
   Sippy's **authenticated** route. Mutating tools also send `X-Acting-For: <acting_for>`, including
   on the reads they make before and after the write. The proxy in front of Sippy authenticates the
   token and sets `X-Forwarded-User` to the service account's principal.
3. Sippy compares the complete `X-Forwarded-User` value with its `--trusted-delegator` allowlist
   (exact match, no wildcards). For an allowlisted principal, Sippy uses a non-blank `X-Acting-For`
   (surrounding whitespace is trimmed) as the effective user for authorization checks and triage
   audit records. On mutating requests (anything other than `GET`, `HEAD`, or `OPTIONS`) the header
   is required: a missing or blank value returns `400 Bad Request` before any handler runs, and
   Sippy never records the service account as the user. Reads without it run as the service account.
   For any other principal, `X-Acting-For` is ignored. Unauthenticated requests cannot gain an
   identity from it.
4. Triage create, update, and delete return `401 Unauthorized` when Sippy cannot determine a user.
   Delegation changes only the effective identity; it does not grant extra endpoints or
   permissions (`--enable-write-endpoints` still controls writes).

Consumer responsibilities:

* Only trusted, authenticated consumers may call this server, and only they may supply
  `acting_for`. The consumer must take it from its own authenticated user context (for example
  the Slack or SSO identity established by the consuming agent), never from text a user typed.
  This server and Sippy vouch for nothing beyond what the consumer supplies.
* `create_triage`, `update_triage`, and `delete_triage` require `acting_for`. `list_triages` and
  `get_triage` take no identity, because Sippy only requires one for mutating requests.
* `acting_for` is trimmed, must be non-blank, at most 256 characters, and contain only printable
  ASCII characters. HTTP header values are not reliably transported beyond ASCII, and an audit
  identity is never re-encoded or altered, so other values are rejected with `invalid_input`.

Deployment requirements (supplied by the separate deployment work, not this repository):

* The Sippy instance serving `SIPPY_MCP_API_URL` runs with `--enable-write-endpoints` and one
  `--trusted-delegator <principal>` per allowed principal, including the exact principal its proxy
  reports for this server's service account (for example `system:serviceaccount:<ns>:<name>`).
  See [config/README.md](../../config/README.md#trusted-delegators).
* That proxy accepts the bearer token and sets `X-Forwarded-User`, and must replace any
  client-supplied `X-Forwarded-User`.
* This server is reachable only through an authenticating proxy. Anything that can reach its
  listener can act with the service account's write access.

## Configuration

All values come from the environment. Missing or invalid required values make the server exit
with status 2 before it serves anything.

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `SIPPY_MCP_API_URL` | yes | none | Base URL of the authenticated Sippy API, for example `https://sippy-auth.example.com`. HTTPS is required except for loopback hosts. Credentials, invalid ports, and query or fragment delimiters (even empty ones) are rejected. Loopback requests never use an inherited HTTP proxy. |
| `SIPPY_MCP_AUTH_TOKEN_FILE` | yes | none | Path to the service-account bearer token. Read on every request so rotated tokens apply. Must be readable at startup and contain a single bearer token (a trailing newline is fine). |
| `SIPPY_MCP_REQUEST_TIMEOUT` | no | `30` | Seconds per Sippy request (greater than 0, at most 300). |
| `SIPPY_MCP_TRANSPORT` | no | `stdio` | `stdio` or `streamable-http`. The image sets `streamable-http`. |
| `SIPPY_MCP_HTTP_HOST` | no | `127.0.0.1` | Listen address for `streamable-http`. Loopback keeps it reachable only by a same-pod proxy. |
| `SIPPY_MCP_HTTP_PORT` | no | `8090` | Listen port for `streamable-http`. The MCP endpoint is `/mcp`; `/healthz` returns `{"status": "ok"}`. |
| `SIPPY_MCP_ALLOWED_HOSTS` | no | none | Comma-separated extra `Host` header values the `streamable-http` listener accepts, for proxies that forward the external host name. Requests whose `Host` is not loopback, the listen address, or listed here get `421 Misdirected Request`, including `/healthz`. |

Tokens are never logged or returned. Upstream error bodies are reduced to Sippy's JSON `message`
(truncated to 500 characters); other bodies, such as HTML from a proxy, are replaced by the status.
Redirects are not followed, so the token is never resent to another location.

## Tools

All tools return a JSON object. Failures return `{"error": {"code": ..., "message": ..., ...}}`.
Unknown arguments are rejected, and arguments are validated strictly against the schema without
type coercion (for example `true` or `"5"` is not accepted as a triage ID). An omitted argument and
an explicit `null` both mean "not supplied".

| Tool | REST mapping | Inputs | Success result |
| --- | --- | --- | --- |
| `list_triages` | `GET /api/component_readiness/triages` | optional `view`, `include_archived` (bool, default false), `type`, `status` (`open` or `resolved`), `url_contains` (case-insensitive), `regression_id`, `limit` (1 to 200, default 50) | `triages` (newest first, summaries with `regression_ids`), `count`, `total_matches`, `truncated`. No matches gives an empty list. |
| `get_triage` | `GET /api/component_readiness/triages/{id}` | `triage_id` | `triage` including `regressions` summaries |
| `create_triage` | `POST /api/component_readiness/triages`, then `GET .../{id}` | `acting_for`, `url` (absolute http(s), typically Jira), `type`; optional `description`, `regression_ids` (non-empty, unique) | `triage`, `verified: true` |
| `update_triage` | `GET .../{id}`, `PUT .../{id}`, `GET .../{id}` | `acting_for`, `triage_id`; optional `url`, `description`, `type`, `resolved` (RFC 3339 with timezone), `clear_resolved` (bool), `add_regression_ids`, `remove_regression_ids` | `triage`, `verified: true`, `changed_fields`, optional `warnings` |
| `delete_triage` | `GET .../{id}`, `DELETE .../{id}`, `GET .../{id}` (expects 404) | `acting_for`, `triage_id` | `deleted: true`, `verified: true`, `triage_id`, `deleted_triage` |

`view` and `include_archived` are applied by Sippy (see
[pkg/api/README.md](../../pkg/api/README.md#component-readiness-triages)); the other list filters
are applied to Sippy's response. Triage `type` is one of `ci-infra`, `product-infra`, `product`,
`test`. Record identifiers are Sippy's numeric triage and regression IDs.

A triage summary contains `id`, `url`, `description`, `type`, `resolved` (RFC 3339 or null),
`resolution_reason`, `bug_id`, `bug` (`key`, `status`, `summary`, `url`, or null),
`regression_ids`, `created_at`, `updated_at`, and Sippy's HATEOAS `links`. Regression summaries
contain `id`, `test_id`, `test_name`, `release`, `base_release`, `component`, `capability`,
`variants`, `opened`, and `closed`.

### Update semantics

Sippy's `PUT` replaces the whole record, so `update_triage` reads the complete record, applies only
the supplied changes, and sends the full replacement:

* Omitted fields, the bug link, the resolution and its reason, and regression associations are
  preserved.
* `description: ""` clears the description. `url` and `type` cannot be cleared.
* `resolved` marks the triage resolved at that instant (Sippy sets `resolution_reason` to `user`);
  `clear_resolved: true` unresolves it. Supplying both is rejected.
* `add_regression_ids` must name regressions that are not yet linked, and `remove_regression_ids`
  must name linked ones. Removing every linked ID clears the associations. Empty lists, duplicate
  IDs, an ID in both lists, and requests with no changes are rejected.

Concurrency: Sippy has no version or precondition check, so the read and the write are not atomic.
A change another writer makes between them can be overwritten (last writer wins). After writing,
the tool compares the fields you did not change with the record it read first and adds a
`warnings` entry when they differ.

`verified: true` therefore confirms only that the fields you changed have the requested values. It
does not confirm that omitted fields are unchanged: the replacement restores the values from the
first read, so a concurrent change that the `PUT` overwrites leaves no difference for the drift
check to report.

### Verification and error codes

Create and update read the record back and compare the requested values. Delete confirms that a
subsequent read returns not found. Mutations are never retried.

| Code | Meaning |
| --- | --- |
| `invalid_input` | Rejected by local validation or Sippy (`400`), including a missing delegated identity. No change was made. |
| `not_found` | The triage does not exist (`404`). |
| `unauthenticated`, `forbidden` | Sippy or its proxy rejected the credentials (`401`, `403`, or a redirect). |
| `unsupported` | Sippy lacks the capability, for example write endpoints are disabled (`501`). |
| `configuration_error` | The token file is missing, unreadable, or malformed. No request was sent. |
| `upstream_error` | Unexpected Sippy failure. For a mutation, the message says the outcome may be unknown. |
| `upstream_unavailable` | A read received no response. |
| `outcome_unknown` | A mutation received no response. Read the triage before trying again. |
| `verification_failed` | The mutation succeeded but the persisted state differs (`mismatches` lists fields). |
| `verification_unavailable` | The mutation succeeded but the readback failed, including when the token became unreadable after the write. `triage_id` names the record. |

## Running

The dev virtualenv (`mcp/run.sh --sippy-dev-setup-only`) has the same locked dependency versions:

```bash
export SIPPY_MCP_API_URL=https://sippy-auth.example.com
export SIPPY_MCP_AUTH_TOKEN_FILE=/path/to/token
mcp/.venv/bin/python mcp/sippy/sippy_mcp_server.py            # stdio
SIPPY_MCP_TRANSPORT=streamable-http mcp/.venv/bin/python mcp/sippy/sippy_mcp_server.py
```

Do not point a local server at the production Sippy instance unless you intend to change production
records.

## Testing

`make test` (the `sippy-dev` MCP `run_tests` tool) runs the unit and registration tests together
with the development server tests. It does not collect `test_triage_integration.py`, so exported
integration variables can never make it write to a Sippy instance:

```bash
cd mcp && ./run.sh --sippy-dev-setup-only && \
  .venv/bin/pytest sippy/ --ignore=sippy/test_triage_integration.py -v
```

Client tests use a fake transport (`FakeSippy` in `test_triage_api.py`) that models `PUT`
replacement semantics, so they never contact a real Sippy. `test_triage_integration.py` is skipped
unless `SIPPY_MCP_INTEGRATION_SIPPY_URL` and `SIPPY_MCP_INTEGRATION_DELEGATOR` are set.
`make e2e` (`run_e2e`) sets them before the Go e2e suite, using the isolated e2e server and database
that `scripts/e2e.sh` starts with two `--trusted-delegator` values. A local proxy stands in for the
deployed oauth-proxy. Its output is in `e2e-mcp-sippy.log` in the e2e artifact directory.
Server-side delegation, rejection, and audit attribution are also covered by
`test/e2e/componentreadiness/triage/delegation_test.go`.

## Image

```bash
make sippy-mcp-image                       # tag: sippy-mcp:latest
make sippy-mcp-image SIPPY_MCP_IMAGE=quay.io/example/sippy-mcp:dev
```

The build context is `mcp/sippy`. A `ubi9/python-312` builder stage installs the hash-verified
`requirements.lock` into a virtualenv; the `ubi9/ubi-minimal` runtime stage adds only the
`python3.12` package, that virtualenv, `triage_api.py`, and `sippy_mcp_server.py`, so compilers,
development packages, and pip are not in the final image. It runs as UID 1001 and starts the server
with `streamable-http` on `127.0.0.1:8090`. Runtime credentials and trusted-delegator values are not
part of the image.

Isolated startup smoke test (no network, dummy token):

```bash
podman run --rm --network none sippy-mcp:latest     # exits 2: SIPPY_MCP_API_URL is required
echo dummy > /tmp/token && chmod 644 /tmp/token
podman run -d --name sippy-mcp-smoke --network none \
  -v /tmp/token:/var/run/secrets/sippy-mcp/token:ro,Z \
  -e SIPPY_MCP_API_URL=https://sippy-api.invalid \
  -e SIPPY_MCP_AUTH_TOKEN_FILE=/var/run/secrets/sippy-mcp/token \
  sippy-mcp:latest
podman exec sippy-mcp-smoke python -c \
  "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8090/healthz').read())"
podman rm -f sippy-mcp-smoke
```

Registry publication, deployment manifests, and deployment configuration are managed outside this
repository.
