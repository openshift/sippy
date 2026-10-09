package jiraautomator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	jiratype "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExistingIssuesForComponent(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantIssue  bool
		wantError  bool
	}{
		{
			name:       "existing issue includes fields used by the automator",
			statusCode: http.StatusOK,
			body: fmt.Sprintf(`{"issues":[{"id":"123","key":"OCPBUGS-123","fields":{
				"status":{"name":"Closed"},"resolutiondate":"2026-10-01T12:00:00.000+0000",
				%q:{"value":"Approved"}}}],"isLast":true}`, jiratype.CustomFieldReleaseBlockerName),
			wantIssue: true,
		},
		{
			name:       "no matching issues",
			statusCode: http.StatusOK,
			body:       `{"issues":[],"isLast":true}`,
		},
		{
			name:       "search error is returned",
			statusCode: http.StatusUnauthorized,
			body:       `{"errorMessages":["Authentication required"]}`,
			wantError:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				if !assert.Equal(t, "/rest/api/2/search/jql", r.URL.Path) {
					w.WriteHeader(http.StatusGone)
					return
				}
				query := r.URL.Query()
				assert.Equal(t, "1", query.Get("maxResults"))
				assert.Equal(t, "project=OCPBUGS&&component='Networking'&&creator='automator'&&affectedVersion=4.22&&labels in (ComponentAutomatedRegression) ORDER BY createdDate", query.Get("jql"))
				assert.ElementsMatch(t, []string{"key", "status", "resolutiondate", jiratype.CustomFieldReleaseBlockerName, "unknowns"}, strings.Split(query.Get("fields"), ","))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, err := fmt.Fprint(w, tt.body)
				assert.NoError(t, err)
			}))
			defer server.Close()

			client, err := jira.NewClient(server.Client(), server.URL)
			require.NoError(t, err)
			automator := JiraAutomator{jiraClient: client, jiraAccount: "automator", dryRun: true}
			view := crview.View{SampleRelease: reqopts.RelativeRelease{Release: reqopts.Release{Name: "4.22"}}}
			issues, err := automator.getExistingIssuesForComponent(view, JiraComponent{Project: "OCPBUGS", Component: "Networking"})
			if tt.wantError {
				require.ErrorContains(t, err, "Authentication required")
				return
			}
			require.NoError(t, err)
			if !tt.wantIssue {
				assert.Empty(t, issues)
				return
			}
			require.Len(t, issues, 1)
			assert.Equal(t, "123", issues[0].ID)
			assert.Equal(t, "OCPBUGS-123", issues[0].Key)
			require.NotNil(t, issues[0].Fields)
			require.NotNil(t, issues[0].Fields.Status)
			assert.Equal(t, jiratype.StatusClosed, issues[0].Fields.Status.Name)
			assert.Equal(t, time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC), time.Time(issues[0].Fields.Resolutiondate).UTC())
			assert.True(t, isReleaseBlockerApproved(&issues[0]))
		})
	}
}
