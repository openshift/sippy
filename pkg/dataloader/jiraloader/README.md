# Jira loader

The loader reads OCPBUGS components, TRT incidents through paginated enhanced
search, and individual issue changelogs for incidents no longer returned by
search. It stores components and incident status in PostgreSQL. It does not
write to Jira.

`New` retains the existing site URL and `JIRA_TOKEN` / `JIRA_TOKEN_BASIC`
authentication, including unauthenticated reads when neither token is supplied.
`NewWithClient` accepts an authenticated HTTP client, base URL, and context for
other authentication modes. All reads share that client and preserve gateway
path prefixes. Keep the context alive until loading finishes, including any
OAuth token renewals.

The HTTP queries and response decoding are separate from database updates so
their authentication, URL handling, and pagination can be tested without a
database connection.

`sippy load --jira-service-account` supplies the shared OAuth client to
`NewWithClient`. When regression tracking also runs, its Jira SDK client uses
the same HTTP client and OAuth token cache. See [configuration and rollout instructions](../../../config/README.md#jira-data-loader-rollout)
for credential variables and deployment details.
