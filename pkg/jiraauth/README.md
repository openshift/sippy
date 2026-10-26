# Jira service-account authentication

`Config.NewClient` returns an authenticated `http.Client` and the Jira gateway
base URL. Both raw HTTP consumers and `go-jira` can use these together.

The constructor obtains the initial token so invalid credentials fail at startup.
`golang.org/x/oauth2/clientcredentials` supplies the bearer header, reuses valid
tokens, and obtains replacement tokens on expiry. No token files or renewal
timers are needed. Keep the supplied context alive for the client's lifetime.

The OAuth library retains that context for the initial token grant and every
later renewal. Pass the server or job lifetime context, rather than an incoming
request context or a constructor timeout that is canceled on return. Canceling
the context prevents subsequent token grants; a cached valid token may still be
used until it expires. Jira API request cancellation is controlled separately
by each request's context.

`serve` and `component-readiness` pass their command contexts. `load` derives a
four-hour job context from its command context, and `automate-jira` derives a
one-hour context. These job contexts remain alive until their commands finish
or are canceled.

Scopes are assigned when the credential is created in Atlassian Administration.
See [Atlassian's service-account OAuth documentation](https://support.atlassian.com/user-management/docs/create-oauth-2-0-credential-for-service-accounts/).

Command configuration lives in `pkg/flags.JiraFlags`. SDK callers use
`GetJiraClient(ctx)`, which selects OAuth when `ServiceAccount` is enabled and
otherwise retains PAT authentication. Raw HTTP consumers use
`GetServiceAccountHTTPClient(ctx)` after selecting the OAuth path. Credential
selection and token renewal remain shared between both forms of client.
