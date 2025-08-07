package flags

import (
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
		JiraURL: "https://issues.redhat.com/",
	}
}

func (f *JiraFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.JiraTokenFile,
		"jira-token-file",
		f.JiraTokenFile,
		"file containing Jira token")
	fs.StringVar(&f.JiraURL, "jira-url", f.JiraURL, "Jira URL")
}

type bearerAuthTransport struct {
	Token     string
	Transport http.RoundTripper
}

func (bat *bearerAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", "Bearer "+bat.Token)
	return bat.transport().RoundTrip(req)
}

func (bat *bearerAuthTransport) transport() http.RoundTripper {
	if bat.Transport != nil {
		return bat.Transport
	}
	return http.DefaultTransport
}

// GetJiraClient initializes and returns a Jira client if token is available
func (f *JiraFlags) GetJiraClient() (*jira.Client, error) {
	var jiraToken string

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

	if jiraToken == "" {
		log.Warn("JIRA_TOKEN not set and no token file provided, Jira client will be nil, and utilizing it will result in dry-run functionality")
		return nil, nil
	}

	httpClient := &http.Client{Transport: &bearerAuthTransport{Token: jiraToken}}

	jiraClient, err := jira.NewClient(httpClient, f.JiraURL)
	if err != nil {
		return nil, err
	}
	return jiraClient, nil
}
