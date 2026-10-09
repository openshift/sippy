# Jira service-account authentication

`Config.NewClient` returns an authenticated `http.Client` and the Jira gateway
base URL. Both raw HTTP consumers and `go-jira` can use these together.

The constructor obtains the initial token so invalid credentials fail at startup.
`golang.org/x/oauth2/clientcredentials` supplies the bearer header, reuses valid
tokens, and obtains replacement tokens on expiry. No token files or renewal
timers are needed. Keep the supplied context alive for the client's lifetime.

Scopes are assigned when the credential is created in Atlassian Administration.
See [Atlassian's service-account OAuth documentation](https://support.atlassian.com/user-management/docs/create-oauth-2-0-credential-for-service-accounts/).
