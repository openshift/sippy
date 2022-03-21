package api

import (
	"fmt"
	"net/http"

	"k8s.io/klog"

	"github.com/lib/pq"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"gorm.io/gorm"
)

func PrintPullRequestsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q := releaseFilter(req, dbClient)
	q = q.Joins(`INNER JOIN release_tag_pull_requests ON release_tag_pull_requests.pull_request_id = pull_requests.id JOIN release_tags on release_tags.id = release_tag_pull_requests.release_tag_id`)
	q, err := FilterableDBResult(req, "id", apitype.SortDescending, q, nil)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	prs := make([]models.PullRequest, 0)
	q.Find(&prs)
	RespondWithJSON(http.StatusOK, w, prs)
}

func PrintReleaseJobRunsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q := releaseFilter(req, dbClient)
	q = q.Joins(`JOIN release_tags on release_tags.id = job_runs."releaseTagID"`)
	q, err := FilterableDBResult(req, "id", apitype.SortDescending, q, nil)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	jobRuns := make([]models.JobRun, 0)
	q.Find(&jobRuns)
	RespondWithJSON(http.StatusOK, w, jobRuns)
}

func PrintReleasesReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	type apiReleaseTag struct {
		models.ReleaseTag
		FailedJobNames pq.StringArray `gorm:"type:text[];column:failedJobNames" json:"failedJobNames,omitempty"`
	}

	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q, err := FilterableDBResult(req, "releaseTag", apitype.SortDescending, releaseFilter(req, dbClient), nil)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	releases := make([]apiReleaseTag, 0)

	// This join looks up the names of failed jobs, if any, and returns them as
	// a JSON aggregation (i.e. failedJobNames will contain a JSON array).
	q.Table("release_tags").
		Select(`release_tags.*, job_runs."failedJobNames"`).
		Joins(`LEFT OUTER JOIN 
   			(
				SELECT
					release_tags."releaseTag", array_agg(job_runs."jobName" ORDER BY job_runs."jobName" asc) AS "failedJobNames"
				FROM
					job_runs
   				JOIN
					release_tags ON release_tags."id" = job_runs."releaseTagID"
   				WHERE
					job_runs.state = 'Failed'
	   			GROUP BY
					release_tags."releaseTag"
			) job_runs using ("releaseTag")`).
		Scan(&releases)

	RespondWithJSON(http.StatusOK, w, releases)
}

func PrintReleaseHealthReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	type apiResult struct {
		models.ReleaseTag
		LastPhase string `json:"lastPhase"`
		Count     int    `json:"count"`
	}

	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	release := req.URL.Query().Get("release")
	if release == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"release" is required`),
		})
		return
	}

	results, err := models.GetLastAcceptedByArchitectureAndStream(dbClient.DB, release)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	apiResults := make([]apiResult, 0)
	for _, archStream := range results {
		phase, count, err := models.GetLastPayloadStatus(dbClient.DB, archStream.Architecture, archStream.Stream, release)
		if err != nil {
			klog.V(1).Infof("got error when trying to find last payload status: %s", err.Error())
		}
		apiResults = append(apiResults, apiResult{
			ReleaseTag: archStream,
			LastPhase:  phase,
			Count:      count,
		})
	}

	RespondWithJSON(http.StatusOK, w, apiResults)
}

func releaseFilter(req *http.Request, dbClient *db.DB) *gorm.DB {
	releaseFilter := req.URL.Query().Get("release")
	if releaseFilter != "" {
		return dbClient.DB.Where("release = ?", releaseFilter)
	}

	return dbClient.DB
}
