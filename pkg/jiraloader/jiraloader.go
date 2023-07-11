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
	req, err := http.NewRequest("GET", "https://issues.redhat.com/rest/api/2/search?jql=labels%20in%20(trt-incident)", nil)
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

	layout := "2006-01-02T15:04:05.000Z0700"
	for _, issue := range issues.Issues {
		jiraID, err := strconv.ParseUint(issue.ID, 10, 64)
		if err != nil {
			fmt.Printf("parsing error: %+v", err)
			continue
		}

		var startTimeP, resolutionTimeP *time.Time
		if issue.Fields.Created != "" {
			startTime, err := time.Parse(layout, issue.Fields.Created)
			if err != nil {
				fmt.Printf("parsing error: %+v", err)
				continue
			}
			startTimeP = &startTime
		}

		if issue.Fields.ResolutionDate != "" {
			resolutionTime, err := time.Parse(layout, issue.Fields.ResolutionDate)
			if err != nil {
				fmt.Printf("parsing error: %+v", err)
				continue
			}
			resolutionTimeP = &resolutionTime
		}

		model := &models.JiraIncident{
			Model: models.Model{
				ID: uint(jiraID),
			},
			Key:            issue.Key,
			Summary:        issue.Fields.Summary,
			StartTime:      startTimeP,
			ResolutionTime: resolutionTimeP,
		}

		if res := jl.dbc.DB.Save(model); res.Error != nil {
			log.WithError(err).Errorf("couldn't save jira incident to DB")
			return res.Error
		}
	}

	log.Infof("jira incident fetch complete in %+v", time.Since(start))
	return nil
}
