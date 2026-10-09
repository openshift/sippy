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
	JiraTokenFile string
	JiraURL       string
}

func NewJiraFlags() *JiraFlags {
	return &JiraFlags{
		JiraURL: "https://redhat.atlassian.net/",
	}
}

func (f *JiraFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.JiraTokenFile,
		"jira-token-file",
		f.JiraTokenFile,
		"file containing Jira token")
	fs.StringVar(&f.JiraURL, "jira-url", f.JiraURL, "Jira URL")
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

// GetJiraClient initializes and returns a Jira client if token is available
func (f *JiraFlags) GetJiraClient() (*jira.Client, error) {
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

// GetReadOnlyServiceAccountClient explicitly selects the read-only OAuth
// credential. JiraURL and PAT configuration do not apply to this gateway client.
func (f *JiraFlags) GetReadOnlyServiceAccountClient(ctx context.Context) (*jira.Client, error) {
	config := jiraauth.Config{
		ClientID:     os.Getenv("JIRA_READ_ONLY_CLIENT_ID"),
		ClientSecret: os.Getenv("JIRA_READ_ONLY_CLIENT_SECRET"),
		CloudID:      os.Getenv("JIRA_CLOUD_ID"),
	}
	httpClient, baseURL, err := config.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("configure read-only Jira service-account client (JIRA_READ_ONLY_CLIENT_ID, JIRA_READ_ONLY_CLIENT_SECRET, JIRA_CLOUD_ID): %w", err)
	}
	client, err := jira.NewClient(httpClient, baseURL)
	if err != nil {
		return nil, fmt.Errorf("create read-only Jira service-account client: %w", err)
	}
	return client, nil
}
