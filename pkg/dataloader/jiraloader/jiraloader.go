package jiraloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/sets"

	v1jira "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// JiraLoader loads various data sources directly from the Jira API, such as TRT incidents and OCPBUGS components.
type JiraLoader struct {
	dbc           *db.DB
	errors        []error
	ctx           context.Context
	httpClient    *http.Client
	baseURL       string
	authorization string
}

// New preserves the site's existing PAT or unauthenticated loading configuration.
func New(dbc *db.DB) *JiraLoader {
	loader := NewWithClient(context.Background(), dbc, &http.Client{}, "https://redhat.atlassian.net/")
	loader.authorization = loader.getAuthorizationHeader()
	return loader
}

// NewWithClient uses the supplied authenticated HTTP client and Jira base URL.
// ctx must remain valid until loading completes, including any token renewals.
// A nil client causes reads to return an error.
func NewWithClient(ctx context.Context, dbc *db.DB, client *http.Client, baseURL string) *JiraLoader {
	return &JiraLoader{
		dbc:        dbc,
		ctx:        ctx,
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/") + "/",
	}
}

const jiraTimeLayout = "2006-01-02T15:04:05.000Z0700"

// Name identifies the loader for command selection and error reporting.
func (jl *JiraLoader) Name() string {
	return "jira"
}

// getAuthorizationHeader looks for environment variable settings
// and returns the authorization string based on the setting present
// if any.  UAT environment requires authentication
// may be temporary when that becomes production
// this will retrieve the authorization value if present

func (jl *JiraLoader) getAuthorizationHeader() string {
	// For a fresh sync to a developer DB, no jira token is needed since the issues API is still open. However, if
	// we need to find cards where the trt-incident label was removed this API is not protected and returns 401
	// if tried unauthed.  So really this only affects long-lived instances of Sippy.
	//
	// WARNING: DO NOT give public-facing Sippy a personal developer token, use a service account that is not marked
	// as a Red Hat employee.

	// bearer token supports service accounts
	bearerToken := os.Getenv("JIRA_TOKEN")
	if len(bearerToken) > 0 {
		return fmt.Sprintf("Bearer %s", bearerToken)
	}

	// basic token supports personal access tokens
	basicToken := os.Getenv("JIRA_TOKEN_BASIC")
	if len(basicToken) > 0 {
		return fmt.Sprintf("Basic %s", basicToken)
	}

	// may not be required so return empty string
	log.Warningf("not all jira api queries are available without a token; some requests may fail")
	return ""
}

// Load refreshes the component and incident data stored in PostgreSQL.
func (jl *JiraLoader) Load() {
	// Load components into DB
	jl.componentLoader()

	// Load incidents
	jl.incidentLoader()
}

// Errors reports failures encountered while refreshing Jira data.
func (jl *JiraLoader) Errors() []error {
	return jl.errors
}

func findResolutionTime(issue *v1jira.Issue) *time.Time {
	var resolutionTime *time.Time

	changelogLayout := "2006-01-02T15:04:05.999-0700"

	for _, history := range issue.Changelog.Histories {
		for _, item := range history.Items {
			// If trt-incident label was removed from a jira ticket, consider that the time when the incident
			// was resolved.
			if !issueContainsLabel(issue, "trt-incident") && item.Field == "labels" && item.FromString == "trt-incident" && item.ToString != "trt-incident" {
				createdTime, err := time.Parse(changelogLayout, history.Created)
				if err != nil {
					log.WithError(err).Warningf("parsing error: %s", history.Created)
					continue
				}
				// We pick the oldest time we removed the trt-incident label (maybe we toggled back and forth a few
				// times).
				if resolutionTime == nil || resolutionTime.Before(createdTime) {
					log.Debugf("trt-incident label was removed from %s at %+v", issue.Key, createdTime)
					resolutionTime = &createdTime
				}
			}

			// OCPBUGS don't get a resolution time until it's closed which happens when a release GA's. We
			// find the first terminal incident status changelog instead. From TRT's perspective, we don't
			// care about OCPBUGS incidents after they go to MODIFIED.
			resolvedStatuses := []string{"MODIFIED", "ON_QA", "Verified", "Closed"}
			for _, status := range resolvedStatuses {
				if item.ToString == status {
					createdTime, err := time.Parse(changelogLayout, history.Created)
					if err != nil {
						log.WithError(err).Warningf("parsing error: %s", history.Created)
						continue
					}
					// We pick the oldest state change
					if resolutionTime == nil || resolutionTime.After(createdTime) {
						log.Debugf("%s to %s at %+v", issue.Key, status, createdTime)
						resolutionTime = &createdTime
					}
				}
			}
		}
	}

	// Fallback to the jira resolution time
	if issue.Fields.ResolutionDate != "" && resolutionTime == nil {
		jiraResolutionTime, err := time.Parse(jiraTimeLayout, issue.Fields.ResolutionDate)
		if err != nil {
			fmt.Printf("parsing error: %+v", err)
		}
		log.Debugf("resolution time for %s is %+v", issue.Key, jiraResolutionTime)
		resolutionTime = &jiraResolutionTime
	}

	return resolutionTime
}

func issueContainsLabel(issue *v1jira.Issue, label string) bool {
	for _, issueLabel := range issue.Fields.Labels {
		if issueLabel == label {
			return true
		}
	}

	return false
}

// queryComponents fetches Jira components separately from their database updates.
func (jl *JiraLoader) queryComponents() ([]v1jira.Component, error) {
	body, err := jl.jiraRequest("rest/api/2/project/OCPBUGS/components")
	if err != nil {
		return nil, err
	}
	var components []v1jira.Component
	if err := json.Unmarshal(body, &components); err != nil {
		return nil, fmt.Errorf("decode Jira components: %w", err)
	}
	return components, nil
}

// componentLoader synchronizes Jira components and removes obsolete database records.
func (jl *JiraLoader) componentLoader() {
	start := time.Now()
	log.Infof("loading jira ocpbugs component information...")
	components, err := jl.queryComponents()
	if err != nil {
		jl.errors = append(jl.errors, err)
		return
	}

	var ids []uint
	for _, c := range components {
		jiraID, err := strconv.ParseUint(c.ID, 10, 64)
		if err != nil {
			msg := "error parsing jira ID"
			log.WithError(err).Warn(msg)
			jl.errors = append(jl.errors, errors.WithMessage(err, msg))
			continue
		}

		mc := models.JiraComponent{
			Model: models.Model{
				ID: uint(jiraID),
			},
			Name:        c.Name,
			Description: c.Description,
			LeadName:    c.Lead.DisplayName,
			LeadEmail:   c.Lead.Name,
		}

		if err := jl.dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Save(&mc).Error; err != nil {
			jl.errors = append(jl.errors, err)
			log.WithError(err).Warningf("failed to save component %q", c.Name)
			continue
		}
		ids = append(ids, mc.ID)
	}

	log.Infof("deleting old records...")
	oldRecords := jl.dbc.DB.Where("id NOT IN ?", ids).Unscoped().Delete(&models.JiraComponent{})
	if oldRecords.Error != nil {
		log.WithError(oldRecords.Error).Warningf("couldn't delete old records")
		jl.errors = append(jl.errors, oldRecords.Error)
	}

	log.WithFields(log.Fields{
		"component_count": len(components),
		"obsolete":        oldRecords.RowsAffected,
	}).Infof("component load complete in %+v", time.Since(start))
}

// queryIncidentPage preserves the incident query and escapes pagination tokens.
func (jl *JiraLoader) queryIncidentPage(nextPageToken string) (*v1jira.SearchResponse, error) {
	query := url.Values{
		"jql":    {`labels = "trt-incident" AND updated >= -60d`},
		"fields": {"summary,created,labels,resolutiondate"},
		"expand": {"changelog"},
	}
	if nextPageToken != "" {
		query.Set("nextPageToken", nextPageToken)
	}
	body, err := jl.jiraRequest("rest/api/3/search/jql?" + query.Encode())
	if err != nil {
		return nil, err
	}
	var response v1jira.SearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Jira incident page: %w", err)
	}
	return &response, nil
}

// incidentLoader refreshes recent incidents and resolves incidents absent from the search results.
func (jl *JiraLoader) incidentLoader() {
	start := time.Now()
	log.Infof("populating unresolved jira incident cache...")
	var dbIssues []string
	jl.dbc.DB.Table("jira_incidents").Where("resolution_time IS NULL").Pluck("key", &dbIssues)
	// unseenUnresolvedIssues contains the set of unresolved issues we have in the DB, but didn't see yet from the jira API. At the end,
	// we'll query to see what happened to the unseen issues. Most likely, we removed the trt-incident label, so we need
	// to dig into the changelog and find that state transition and consider the incident closed then.
	unseenUnresolvedIssues := sets.New(dbIssues...)
	log.Infof("cache populated in %+v with %d records", time.Since(start), len(dbIssues))

	start = time.Now()
	log.Infof("fetching incidents from jira...")

	nextPageToken := ""
	pageCount := 0
	totalIssues := 0

	for {
		pageCount++
		log.Infof("fetching page %d of incidents...", pageCount)
		response, err := jl.queryIncidentPage(nextPageToken)
		if err != nil {
			jl.errors = append(jl.errors, err)
			return
		}

		log.Infof("processing %d issues from page %d", len(response.Issues), pageCount)
		for i, issue := range response.Issues {
			unseenUnresolvedIssues.Delete(issue.Key)

			model, err := issueToDB(&response.Issues[i])
			if err != nil {
				log.WithError(err).Errorf("couldn't convert jira issue to db model")
				continue
			}
			if res := jl.dbc.DB.Save(model); res.Error != nil {
				log.WithError(res.Error).Errorf("couldn't save jira incident to DB")
				jl.errors = append(jl.errors, res.Error)
				return
			}
		}

		totalIssues += len(response.Issues)

		if response.IsLast {
			log.Infof("reached last page (%d pages, %d total issues)", pageCount, totalIssues)
			break
		}

		nextPageToken = response.NextPageToken
		if nextPageToken == "" {
			err := errors.Errorf("nextPageToken is empty but isLast is false, stopping pagination  (%d pages, %d processed issues)", pageCount, totalIssues)
			log.Error(err)
			jl.errors = append(jl.errors, err)
			break
		}
	}

	log.Infof("we have %d unseen and unresolved jira incidents", unseenUnresolvedIssues.Len())
	for _, unseen := range sets.List(unseenUnresolvedIssues) {
		log.Infof("processing unseen, unresolved jira incidents (trt-incident label removed?)...")
		issue, err := jl.queryJiraAPI(unseen)
		if err != nil {
			log.WithError(err).Warnf("couldn't query details for %s. this is expected for cards that are restricted to 'Red Hat Only'", unseen)
			continue
		}

		model, err := issueToDB(issue)
		if err != nil {
			log.WithError(err).Errorf("couldn't convert jira issue to db model")
			continue
		}
		if res := jl.dbc.DB.Save(model); res.Error != nil {
			log.WithError(res.Error).Errorf("couldn't save jira incident to DB")
			jl.errors = append(jl.errors, res.Error)
			return
		}
	}

	log.Infof("jira incident fetch complete in %+v", time.Since(start))
}

// queryJiraAPI returns a singular jira issue
func (jl *JiraLoader) queryJiraAPI(issueID string) (*v1jira.Issue, error) {
	bodyBytes, err := jl.jiraRequest("rest/api/2/issue/" + url.PathEscape(issueID) + "?expand=changelog")
	if err != nil {
		return nil, err
	}

	var issueDetails v1jira.Issue
	if err := json.Unmarshal(bodyBytes, &issueDetails); err != nil {
		return nil, fmt.Errorf("decode Jira issue: %w", err)
	}

	return &issueDetails, nil
}

func issueToDB(issue *v1jira.Issue) (*models.JiraIncident, error) {
	jiraID, err := strconv.ParseUint(issue.ID, 10, 64)
	if err != nil {
		return nil, err
	}

	var startTimeP *time.Time
	if issue.Fields.Created != "" {
		startTime, err := time.Parse(jiraTimeLayout, issue.Fields.Created)
		if err != nil {
			return nil, err
		}
		startTimeP = &startTime
	}

	return &models.JiraIncident{
		Model: models.Model{
			ID: uint(jiraID),
		},
		Key:            issue.Key,
		Summary:        issue.Fields.Summary,
		StartTime:      startTimeP,
		ResolutionTime: findResolutionTime(issue),
	}, nil
}

// jiraRequest shares authentication and context across all loader reads.
// apiPath is a relative Jira REST path, including any encoded query parameters.
func (jl *JiraLoader) jiraRequest(apiPath string) ([]byte, error) {
	if jl.httpClient == nil {
		return nil, fmt.Errorf("jira request failed: HTTP client is nil")
	}

	req, err := http.NewRequestWithContext(jl.ctx, http.MethodGet, jl.baseURL+apiPath, nil)
	if err != nil {
		return nil, err
	}

	if jl.authorization != "" {
		req.Header.Add("Authorization", jl.authorization)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	resp, err := jl.httpClient.Do(req) //nolint:gosec // G704: configured Jira base URL, internal REST paths, escaped issue keys and query parameters
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// Response bodies can contain private Jira data and these errors reach logs.
		return nil, fmt.Errorf("jira request failed: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return io.ReadAll(resp.Body)
}
