package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/lib/pq"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/pkg/errors"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func PrintPullRequestsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q := releaseFilter(req, dbClient.DB)
	q = q.Joins(`INNER JOIN release_tag_pull_requests ON release_tag_pull_requests.release_pull_request_id = release_pull_requests.id JOIN release_tags on release_tags.id = release_tag_pull_requests.release_tag_id`)
	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}
	q, err = filter.FilterableDBResult(q, filterOpts, nil)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	prs := make([]models.ReleasePullRequest, 0)
	q.Find(&prs)
	RespondWithJSON(http.StatusOK, w, prs)
}

func PrintReleaseJobRunsReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	q := releaseFilter(req, dbClient.DB)
	q = q.Joins(`JOIN release_tags on release_tags.id = release_job_runs.release_tag_id`)
	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
			"message": "Error building job run report:" + err.Error()})
		return
	}
	q, err = filter.FilterableDBResult(q, filterOpts, nil)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
		return
	}

	jobRuns := make([]models.ReleaseJobRun, 0)
	q.Find(&jobRuns)
	RespondWithJSON(http.StatusOK, w, jobRuns)
}

func PrintReleasesReport(w http.ResponseWriter, req *http.Request, dbClient *db.DB) {
	type apiReleaseTag struct {
		models.ReleaseTag
		FailedJobNames pq.StringArray `gorm:"type:text[];column:failed_job_names" json:"failed_job_names,omitempty"`
	}

	if dbClient == nil || dbClient.DB == nil {
		RespondWithJSON(http.StatusOK, w, []struct{}{})
	}

	filterOpts, err := filter.FilterOptionsFromRequest(req, "release_tag", apitype.SortDescending)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job run report:" + err.Error()})
		return
	}
	q, err := filter.FilterableDBResult(releaseFilter(req, dbClient.DB), filterOpts, nil)
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
		Select(`release_tags.*, release_job_runs.failed_job_names`).
		Joins(`LEFT OUTER JOIN 
   			(
				SELECT
					release_tags.release_tag, array_agg(release_job_runs.job_name ORDER BY release_job_runs.job_name asc) AS failed_job_names
				FROM
					release_job_runs
   				JOIN
					release_tags ON release_tags.id = release_job_runs.release_tag_id
   				WHERE
					release_job_runs.state = 'Failed'
	   			GROUP BY
					release_tags.release_tag
			) release_job_runs using (release_tag)`).
		Scan(&releases)

	RespondWithJSON(http.StatusOK, w, releases)
}

// ReleaseHealthReports returns a report on the most recent payload status for each arch/stream in
// the given release.
func ReleaseHealthReports(dbClient *db.DB, release string) ([]apitype.ReleaseHealthReport, error) {
	apiResults := make([]apitype.ReleaseHealthReport, 0)
	if dbClient == nil || dbClient.DB == nil {
		return apiResults, fmt.Errorf("no db client configured")
	}

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbClient.DB, release)
	if err != nil {
		return apiResults, err
	}

	for _, archStream := range results {
		phase, count, err := query.GetLastPayloadStatus(dbClient.DB, archStream.Architecture, archStream.Stream, release)
		if err != nil {
			return apiResults, errors.Wrapf(err, "error finding last %s payload status for %s %s",
				release, archStream.Architecture, archStream.Stream)
		}
		apiResults = append(apiResults, apitype.ReleaseHealthReport{
			ReleaseTag: archStream,
			LastPhase:  phase,
			Count:      count,
		})
	}

	return apiResults, nil
}

// ScanForReleaseWarnings looks for problems in current release health and returns them to the user.
func ScanForReleaseWarnings(dbClient *db.DB, release string) []string {
	payloadHealth, err := ReleaseHealthReports(dbClient, release)
	if err != nil {
		// treat the error as a warning itself
		return []string{fmt.Sprintf("error checking release health, see logs: %v", err)}
	}
	// May add more release health checks in future
	return ScanReleaseHealthForRHCOSVersionMisMatches(payloadHealth)
}

func ScanReleaseHealthForRHCOSVersionMisMatches(payloadHealth []apitype.ReleaseHealthReport) []string {

	warnings := make([]string, 0)
	for _, streamHealth := range payloadHealth {
		// Remove the dots in release version to compare against os version.
		// i.e. compare 4.11 to 411.85.202203171100-0
		release := strings.ReplaceAll(streamHealth.Release, ".", "")
		osVerTokens := strings.Split(streamHealth.CurrentOSVersion, ".")
		if len(osVerTokens) <= 2 {
			warnings = append(warnings, fmt.Sprintf("unable to parse OpenShift version from OS version %s",
				streamHealth.CurrentOSVersion))
			continue
		}
		osVer := osVerTokens[0]
		if release != osVer {
			warnings = append(warnings, fmt.Sprintf("OS version %s does not match OpenShift release %s",
				streamHealth.CurrentOSVersion, streamHealth.Release))
			continue
		}

	}
	return warnings
}

func releaseFilter(req *http.Request, dbc *gorm.DB) *gorm.DB {
	releaseFilter := req.URL.Query().Get("release")
	if releaseFilter != "" {
		return dbc.Where("release = ?", releaseFilter)
	}

	return dbc
}
