# Sippy MCP triage management

## Purpose

Lets a trusted agent (for example QSE's agent through the Chai bot) create, read, update, and
delete Component Readiness triage records through the `sippy` MCP server while Sippy's authorization
checks and audit log attribute every change to the human who asked for it, not to the agent's
service account. The `sippy` server is separate from the `sippy-dev` development-workflow server.

## Data flow

```
consumer agent (authenticated user context)
  -> authenticating proxy -> sippy MCP server (mcp/sippy)
       Authorization: Bearer <service-account token>, X-Acting-For: <user> (mutations only)
  -> Sippy's authenticating proxy (sets X-Forwarded-User to the service-account principal)
  -> sippy serve: delegation middleware (trusted delegator allowlist)
       effective user = X-Acting-For  -> triage handlers -> audit_log.user
```

1. The MCP tool validates input and sends the request with the bearer token. Mutating tools
   (create, update, delete) also send `X-Acting-For`; read tools (list, get) do not.
2. Sippy's delegation middleware compares `X-Forwarded-User` exactly with the
   `--trusted-delegator` allowlist. For listed principals a non-blank `X-Acting-For` becomes the
   effective user in the request context. It is required on mutating methods (else `400`); `GET`,
   `HEAD`, and `OPTIONS` without it run as the principal. Other requests are unchanged.
3. `getUserForRequest` returns the effective user to every handler. Triage writes return `401`
   without a user and put the user in the GORM context, where the triage hooks write audit records.
4. The MCP tool reads the record back (or confirms `404` after delete) and returns the verified state.

Partial updates are implemented in the MCP tool as read, merge, and full `PUT` replacement, because
the REST API replaces the whole record. The API has no version check, so this is not atomic.

## Key code

| Location | Role |
| --- | --- |
| `pkg/sippyserver/identity.go` | `delegationResolver` and its middleware (exact allowlist, `X-Acting-For` required on mutating methods) |
| `pkg/sippyserver/mockauth.go` | `getUserForRequest`, shared identity helper (delegated, header, then `DEV_MODE` mock) |
| `pkg/flags/api.go` | `--trusted-delegator` flag and validation |
| `pkg/sippyserver/server.go` | Triage handlers (`requireTriageWriteUser`, `404` handling), middleware wiring |
| `pkg/api/componentreadiness/triage.go` | Triage CRUD and `applyResolutionReason` |
| `pkg/db/models/triage.go` | Audit hooks that record `CurrentUserKey` |
| `mcp/sippy/` | `sippy` MCP server, client, tests, Dockerfile ([README](../../mcp/sippy/README.md)) |
| `test/e2e/componentreadiness/triage/delegation_test.go` | Server-side delegation and audit e2e tests |

Configuration is documented in `config/README.md` (trusted delegators) and
`mcp/sippy/README.md` (MCP runtime settings and consumer contract). Deployment values live
outside this repository.
