package flags

import (
	"fmt"
	"net/http"
	"os"

	"github.com/andygrunwald/go-jira"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
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
