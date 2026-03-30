package util

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andygrunwald/go-jira"
)

// FileBugRequest represents the JSON request structure for filing Jira bugs
type FileBugRequest struct {
	Summary         string   `json:"summary"`
	Description     string   `json:"description"`
	AffectsVersions []string `json:"affects_versions"`
	Components      []string `json:"components"`
	ComponentID     string   `json:"component_id"`
	Labels          []string `json:"labels"`
	Project         string   `json:"project"`
}

// FileBugResponse represents the JSON response structure for filing Jira bugs
type FileBugResponse struct {
	Success bool   `json:"success"`
	DryRun  bool   `json:"dry_run"`
	JiraKey string `json:"jira_key"`
	JiraURL string `json:"jira_url"`
}

func PopulateJiraIssue(jiraClient *jira.Client, bugRequest FileBugRequest, user string) (jira.Issue, error) {
	description := bugRequest.Description

	project := bugRequest.Project
	if project == "" {
		project = "OCPBUGS"
	}

	issue := jira.Issue{
		Fields: &jira.IssueFields{
			Description: description,
			Type: jira.IssueType{
				Name: "Bug",
			},
			Project: jira.Project{
				Key: project,
			},
			Summary: bugRequest.Summary,
		},
	}

	affectsVersions := make([]*jira.AffectsVersion, len(bugRequest.AffectsVersions))
	for i, version := range bugRequest.AffectsVersions {
		affectsVersions[i] = &jira.AffectsVersion{
			Name: version,
		}
	}
	issue.Fields.AffectsVersions = affectsVersions

	components := make([]*jira.Component, 0)
	for _, comp := range bugRequest.Components {
		components = append(components, &jira.Component{Name: comp})
	}
	if len(bugRequest.ComponentID) > 0 {
		components = append(components, &jira.Component{ID: bugRequest.ComponentID})
	}
	if len(components) > 0 {
		issue.Fields.Components = components
	}

	if len(bugRequest.Labels) > 0 {
		issue.Fields.Labels = bugRequest.Labels
	}

	if jiraClient != nil {
		var reporter *jira.User
		var err error
		if len(user) > 0 {
			findUser := user
			// need to match on the email address so we don't get partial / multiple matches
			if !strings.HasSuffix(findUser, "@redhat.com") {
				findUser = fmt.Sprintf("%s@redhat.com", user)
			}
			jiraUser, _, err := jiraClient.User.Find(findUser)
			if err != nil {
				return issue, err
			}

			if len(jiraUser) == 0 {
				return issue, fmt.Errorf("no jira user found for: %s", user)
			}

			ju := jiraUser[0]
			reporter = &ju
		} else {
			reporter, _, err = jiraClient.User.GetSelf()
			if err != nil {
				return issue, err
			}
		}
		issue.Fields.Reporter = reporter
	}
	return issue, nil
}

// GetUnknownField is lifted from "sigs.k8s.io/prow/pkg/jira" to remove the dependency
func GetUnknownField(field string, issue *jira.Issue, fn func() interface{}) error {
	obj := fn()
	unknownField, ok := issue.Fields.Unknowns[field]
	if !ok {
		return nil
	}
	bytes, err := json.Marshal(unknownField)
	if err != nil {
		return fmt.Errorf("failed to process the custom field %s. Error : %v", field, err)
	}
	if err := json.Unmarshal(bytes, obj); err != nil {
		return fmt.Errorf("failed to unmarshall the json to struct for %s. Error: %v", field, err)
	}
	return err
}
