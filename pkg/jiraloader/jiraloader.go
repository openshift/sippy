package jiraloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	v1jira "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

type JIRALoader struct {
	dbc *db.DB
}

func New(dbc *db.DB) *JIRALoader {
	return &JIRALoader{
		dbc: dbc,
	}
}

const jiraTimeLayout = "2006-01-02T15:04:05.000Z0700"

func (jl *JIRALoader) LoadJIRAIncidents() error {
	start := time.Now()
	log.Infof("fetching incidents from jira...")

	/* Note a token isn't currently required to hit the issues.redhat.com API. This gets
	   us public jira cards which is probably what we want, I don't think we're ever doing non-public
	   incidents. That way we don't leak anything embargoed. If at some point we do need a token, you
	   can do it with the below, but make sure it has limited privileges and can only see public cards.

		token := os.Getenv("JIRA_TOKEN")
		req.Header.Add("Authorization", "Bearer "+token)
	*/

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://issues.redhat.com/rest/api/2/search?jql=labels%20in%20(trt-incident)&expand=changelog", nil)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var issues struct {
		Issues []v1jira.Issue `json:"issues"`
	}
	err = json.Unmarshal(body, &issues)
	if err != nil {
		return err
	}

	for i, issue := range issues.Issues {
		jiraID, err := strconv.ParseUint(issue.ID, 10, 64)
		if err != nil {
			fmt.Printf("parsing error: %+v", err)
			continue
		}

		var startTimeP *time.Time
		if issue.Fields.Created != "" {
			startTime, err := time.Parse(jiraTimeLayout, issue.Fields.Created)
			if err != nil {
				fmt.Printf("parsing error: %+v", err)
				continue
			}
			startTimeP = &startTime
		}

		model := &models.JiraIncident{
			Model: models.Model{
				ID: uint(jiraID),
			},
			Key:            issue.Key,
			Summary:        issue.Fields.Summary,
			StartTime:      startTimeP,
			ResolutionTime: findResolutionTime(&issues.Issues[i]),
		}

		if res := jl.dbc.DB.Save(model); res.Error != nil {
			log.WithError(err).Errorf("couldn't save jira incident to DB")
			return res.Error
		}
	}

	log.Infof("jira incident fetch complete in %+v", time.Since(start))
	return nil
}

func findResolutionTime(issue *v1jira.Issue) *time.Time {
	var oldestResolutionTime *time.Time

	changelogLayout := "2006-01-02T15:04:05.999-0700"

	// OCPBUGS don't get a resolution time until it's closed which happens when a release GA's. We
	// find the first terminal incident status changelog instead. From TRT's perspective, we don't
	// care about OCPBUGS incidents after they go to MODIFIED.
	for _, history := range issue.Changelog.Histories {
		for _, item := range history.Items {
			resolvedStatuses := []string{"MODIFIED", "ON_QA", "Verified", "Closed"}
			for _, status := range resolvedStatuses {
				if item.ToString == status {
					createdTime, err := time.Parse(changelogLayout, history.Created)
					if err != nil {
						log.WithError(err).Warningf("parsing error: %s", history.Created)
						continue
					}
					if oldestResolutionTime == nil || oldestResolutionTime.After(createdTime) {
						log.Debugf("%s to %s at %+v", issue.Key, status, createdTime)
						oldestResolutionTime = &createdTime
					}
				}
			}
		}
	}

	// Fallback to the jira resolution time
	if issue.Fields.ResolutionDate != "" && oldestResolutionTime == nil {
		resolutionTime, err := time.Parse(jiraTimeLayout, issue.Fields.ResolutionDate)
		if err != nil {
			fmt.Printf("parsing error: %+v", err)
		}
		log.Debugf("resolution time for %s is %+v", issue.Key, resolutionTime)
		oldestResolutionTime = &resolutionTime
	}

	return oldestResolutionTime
}
