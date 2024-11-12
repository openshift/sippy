# go-githubauth

[![GoDoc](https://img.shields.io/static/v1?label=godoc&message=reference&color=blue)](https://pkg.go.dev/github.com/jferrl/go-githubauth)
[![Test Status](https://github.com/jferrl/go-githubauth/workflows/tests/badge.svg)](https://github.com/jferrl/go-githubauth/actions?query=workflow%3Atests)
[![codecov](https://codecov.io/gh/jferrl/go-githubauth/branch/main/graph/badge.svg?token=68I4BZF235)](https://codecov.io/gh/jferrl/go-githubauth)
[![Go Report Card](https://goreportcard.com/badge/github.com/jferrl/go-githubauth)](https://goreportcard.com/report/github.com/jferrl/go-githubauth)

`go-githubauth` is a Go package that provides utilities for GitHub authentication, including generating and using GitHub App tokens and installation tokens.

## Features

`go-githubauth` package provides implementations of the `TokenSource` interface from the `golang.org/x/oauth2` package. This interface has a single method, Token, which returns an *oauth2.Token.

- Generate GitHub Application JWT [Generating a jwt for a github app](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app>)
- Obtain GitHub App installation tokens [Authenticating as a GitHub App](https://docs.github.com/en/rest/authentication/authenticating-to-the-rest-api?apiVersion=2022-11-28#authenticating-with-a-token-generated-by-an-app)

This package is designed to be used with the `golang.org/x/oauth2` package, which provides support for OAuth2 authentication.

## Installation

To use `go-githubauth` in your project, you need to have Go installed. You can get the package via:

```bash
go get -u github.com/jferrl/go-githubauth
```

## Usage

### Usage with [go-github](https://github.com/google/go-github)  and [oauth2](golang.org/x/oauth2)

```go
package main

import (
 "context"
 "fmt"
 "os"
 "strconv"

 "github.com/google/go-github/v62/github"
 "github.com/jferrl/go-githubauth"
 "golang.org/x/oauth2"
)

func main() {
 privateKey := []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
 appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
 installationID, _ := strconv.ParseInt(os.Getenv("GITHUB_INSTALLATION_ID"), 10, 64)

 appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKey)
 if err != nil {
  fmt.Println("Error creating application token source:", err)
  return
 }

 installationTokenSource := githubauth.NewInstallationTokenSource(installationID, appTokenSource)

 // oauth2.NewClient create a new http.Client that adds an Authorization header with the token.
 // Transport src use oauth2.ReuseTokenSource to reuse the token.
 // The token will be reused until it expires.
 // The token will be refreshed if it's expired.
 httpClient := oauth2.NewClient(context.Background(), installationTokenSource)

 githubClient := github.NewClient(httpClient)

 _, _, err = githubClient.PullRequests.CreateComment(context.Background(), "owner", "repo", 1, &github.PullRequestComment{
  Body: github.String("Awesome comment!"),
 })
 if err != nil {
  fmt.Println("Error creating comment:", err)
  return
 }
}
```

### Generate GitHub Application Token

First of all you need to create a GitHub App and generate a private key.

To authenticate as a GitHub App, you need to generate a JWT. [Generating a jwt for a github app](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app>)

```go
package main

import (
 "fmt"
 "os"
 "time"

 "github.com/jferrl/go-githubauth"
)

func main() {
 privateKey := []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
 appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)

 tokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKey, githubauth.WithApplicationTokenExpiration(5*time.Minute))
 if err != nil {
  fmt.Println("Error creating token source:", err)
  return
 }

 token, err := tokenSource.Token()
 if err != nil {
  fmt.Println("Error generating token:", err)
  return
 }

 fmt.Println("Generated token:", token.AccessToken)
}
```

### Generate GitHub App Installation Token

To authenticate as a GitHub App installation, you need to obtain an installation token.

```go
package main

import (
 "fmt"
 "os"
 "strconv"

 "github.com/jferrl/go-githubauth"
)

func main() {
 privateKey := []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
 appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
 installationID, _ := strconv.ParseInt(os.Getenv("GITHUB_INSTALLATION_ID"), 10, 64)

 appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKey)
 if err != nil {
  fmt.Println("Error creating application token source:", err)
  return
 }

 installationTokenSource := githubauth.NewInstallationTokenSource(installationID, appTokenSource)

 token, err := installationTokenSource.Token()
 if err != nil {
  fmt.Println("Error generating installation token:", err)
  return
 }

 fmt.Println("Generated installation token:", token.AccessToken)
}
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
