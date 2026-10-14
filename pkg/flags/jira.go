package flags

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/andygrunwald/go-jira"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/jiraauth"
)

// JiraFlags holds Jira configuration information for Sippy.
type JiraFlags struct {
	JiraTokenFile  string
	JiraURL        string
	ServiceAccount bool
}

// NewJiraFlags supplies the default Jira site for commands sharing authentication flags.
func NewJiraFlags() *JiraFlags {
	return &JiraFlags{
		JiraURL: "https://redhat.atlassian.net/",
	}
}

// BindFlags exposes shared Jira authentication and endpoint configuration.
func (f *JiraFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.JiraTokenFile,
		"jira-token-file",
		f.JiraTokenFile,
		"file containing Jira token")
	fs.StringVar(&f.JiraURL, "jira-url", f.JiraURL, "Jira URL")
	fs.BoolVar(&f.ServiceAccount, "jira-service-account", f.ServiceAccount, "Use Jira service-account OAuth credentials")
}

type authTransport struct {
	Token     string
	Transport http.RoundTripper
	Type      string
}

func (at *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("%s %s", at.Type, at.Token))
	return at.transport().RoundTrip(req)
}

func (at *authTransport) transport() http.RoundTripper {
	if at.Transport != nil {
		return at.Transport
	}
	return http.DefaultTransport
}

// GetJiraClient selects OAuth when ServiceAccount is set, otherwise retaining
// PAT authentication. OAuth retains ctx for initial token acquisition and all
// later renewals, so use a server or job lifetime context, not a request or
// constructor-only context. Jira API requests use their own request contexts.
// Without PAT credentials it returns a nil client; OAuth configuration errors
// are returned without falling back to PAT.
func (f *JiraFlags) GetJiraClient(ctx context.Context) (*jira.Client, error) {
	if f.ServiceAccount {
		httpClient, baseURL, err := f.GetServiceAccountHTTPClient(ctx)
		if err != nil {
			return nil, err
		}
		client, err := jira.NewClient(httpClient, baseURL)
		if err != nil {
			return nil, fmt.Errorf("create Jira service-account client: %w", err)
		}
		return client, nil
	}
	var jiraToken string
	authorizationType := "Bearer"

	// First try token file
	if f.JiraTokenFile != "" {
		tokenBytes, err := os.ReadFile(f.JiraTokenFile)
		if err != nil {
			log.WithError(err).Error("failed to read jira token file")
			return nil, err
		}
		jiraToken = string(tokenBytes)
	}

	// Fallback to environment variable
	if jiraToken == "" {
		jiraToken = os.Getenv("JIRA_TOKEN")
	}

	// Fallback to basic
	// Basic token is only supported via ENV VAR currently
	if jiraToken == "" {
		jiraToken = os.Getenv("JIRA_TOKEN_BASIC")
		authorizationType = "Basic"
	}

	if jiraToken == "" {
		log.Warn("JIRA_TOKEN not set and no token file provided, Jira client will be nil, and utilizing it will result in dry-run functionality")
		return nil, nil
	}

	httpClient := &http.Client{Transport: &authTransport{Token: jiraToken, Type: authorizationType}}

	jiraClient, err := jira.NewClient(httpClient, f.JiraURL)
	if err != nil {
		return nil, err
	}
	return jiraClient, nil
}

// GetServiceAccountHTTPClient shares credential selection and token renewal
// with raw HTTP consumers. OAuth retains ctx for token acquisition and renewal;
// use a server or job lifetime context. API requests use their own contexts.
func (f *JiraFlags) GetServiceAccountHTTPClient(ctx context.Context) (*http.Client, string, error) {
	clientID, idSet := os.LookupEnv("JIRA_CLIENT_ID")
	clientSecret, secretSet := os.LookupEnv("JIRA_CLIENT_SECRET")
	if !idSet && !secretSet {
		clientID = os.Getenv("JIRA_READ_ONLY_CLIENT_ID")
		clientSecret = os.Getenv("JIRA_READ_ONLY_CLIENT_SECRET")
	}
	config := jiraauth.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CloudID:      os.Getenv("JIRA_CLOUD_ID"),
	}
	httpClient, baseURL, err := config.NewClient(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("configure Jira service-account client (JIRA_CLIENT_ID, JIRA_CLIENT_SECRET, JIRA_CLOUD_ID; legacy aliases JIRA_READ_ONLY_CLIENT_ID, JIRA_READ_ONLY_CLIENT_SECRET): %w", err)
	}
	return httpClient, baseURL, nil
}
