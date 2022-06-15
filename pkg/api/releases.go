package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"

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

func ListPayloadJobRuns(dbClient *db.DB, filterOpts *filter.FilterOptions, release string) ([]models.ReleaseJobRun, error) {
	jobRuns := make([]models.ReleaseJobRun, 0)
	var err error
	q := dbClient.DB
	if release != "" {
		q = q.Where("release = ?", release)
	}
	q = q.Joins(`JOIN release_tags on release_tags.id = release_job_runs.release_tag_id`)
	q, err = filter.FilterableDBResult(q, filterOpts, nil)
	if err != nil {
		return jobRuns, err
	}

	res := q.Find(&jobRuns)
	return jobRuns, res.Error
}

// GetPayloadAnalysis loads the most recent payloads  for a stream and attempts to search for most commonly
// failing tests, possible perma-failing blockers, etc.
func GetPayloadAnalysis(dbc *db.DB, release, stream, arch string, numPayloadsToAnalyze int) (*apitype.PayloadStreamAnalysis, error) {

	logger := log.WithFields(log.Fields{
		"release": release,
		"stream":  stream,
		"arch":    arch,
	})

	result := &apitype.PayloadStreamAnalysis{
		ConsecutiveFailedPayloads: []string{},
	}

	// Get latest payload tags for analysis:
	lastPayloads, err := query.GetLastPayloadTags(dbc.DB, release,
		stream, arch, numPayloadsToAnalyze)
	if err != nil {
		return nil, err
	}
	logger.WithField("payloads", len(lastPayloads)).Debug("loaded last payloads")
	result.PayloadsAnalyzed = len(lastPayloads)

	if len(lastPayloads) == 0 {
		logger.Debug("no payload tags found")
		return result, nil
	}

	result.LastPhase = lastPayloads[0].Phase
	lastPhaseCount := 0
	onlyFailedPayloads := []models.ReleaseTag{}
	for i, p := range lastPayloads {
		if p.Phase == apitype.PayloadRejected {
			onlyFailedPayloads = append(onlyFailedPayloads, p)
		}
		// If we haven't yet found the number of payloads in last phase, check if this is the boundary:
		if result.LastPhaseCount == 0 {
			if p.Phase == result.LastPhase {
				lastPhaseCount++
			}

			if !(p.Phase == result.LastPhase) || i == len(lastPayloads)-1 {
				// We'll stop looking after this is set.
				result.LastPhaseCount = lastPhaseCount
			}

			if result.LastPhase == apitype.PayloadRejected && p.Phase == apitype.PayloadRejected {
				result.ConsecutiveFailedPayloads = append(result.ConsecutiveFailedPayloads, p.ReleaseTag)
			}
		}
	}
	logger.WithField("failedPayloads", len(onlyFailedPayloads)).Debug("failed payloads")

	lastFailedPayloadIDs := make([]uint, len(onlyFailedPayloads))
	for i, lfp := range onlyFailedPayloads {
		lastFailedPayloadIDs[i] = lfp.ID
	}
	logger.WithField("failedPayloadIDs", lastFailedPayloadIDs).Debug("last failed payload IDs")

	// Get the job runs for each of the last failed payloads:
	jobRuns, err := query.ListPayloadBlockingFailedJobRuns(dbc.DB, lastFailedPayloadIDs)
	if err != nil {
		return nil, err
	}
	logger.WithField("jobRuns", len(jobRuns)).Debug("loaded latest job runs")

	// Build a map of job run to it's payload, we'll use later for identifying unique payloads:
	jobRunToPayload := map[uint]string{}
	for _, jr := range jobRuns {
		jobRunToPayload[jr.Name] = jr.ReleaseTag.ReleaseTag
	}

	jobRunIDs := make([]uint, len(jobRuns))
	for i, jr := range jobRuns {
		jobRunIDs[i] = jr.Name
	}
	logger.WithField("jobRunIDs", jobRunIDs).Debug("got job run IDs")

	// Query all test failures for the given job run IDs:
	// NOTE: slow query here over our biggest table.
	failedTests := []models.ProwJobRunTest{}
	dbc.DB.Preload("Test").Where("prow_job_run_id IN ? AND status = 12", jobRunIDs).Find(&failedTests)
	logger.WithField("failedTestCount", len(failedTests)).Debug("found failed tests")

	// Iterate all failed tests, aggregate each test name to the number of times we saw it, and the number of unique
	// jobs we saw it in.
	testNameToAnalysis := map[string]*apitype.TestFailureAnalysis{}
	for _, ft := range failedTests {
		if ft.Test.Name == testidentification.OpenShiftTestsName {
			// Skip the "all tests passed" test we inject, it's not relevant here.
			continue
		}
		if _, ok := testNameToAnalysis[ft.Test.Name]; !ok {
			testNameToAnalysis[ft.Test.Name] = &apitype.TestFailureAnalysis{
				Name:           ft.Test.Name,
				ID:             ft.TestID,
				FailedJobRuns:  []string{},
				FailedPayloads: []string{},
			}
		}
		ta := testNameToAnalysis[ft.Test.Name]
		ta.FailureCount++
		if !util.StrSliceContains(ta.FailedJobRuns, strconv.Itoa(int(ft.ProwJobRunID))) {
			ta.FailedJobRuns = append(ta.FailedJobRuns, strconv.Itoa(int(ft.ProwJobRunID)))
		}
		jrID := jobRunToPayload[ft.ProwJobRunID]
		if !util.StrSliceContains(ta.FailedPayloads, jrID) {
			ta.FailedPayloads = append(ta.FailedPayloads, jrID)
		}
	}
	testFailures := make([]*apitype.TestFailureAnalysis, 0, len(testNameToAnalysis))
	for _, v := range testNameToAnalysis {
		testFailures = append(testFailures, v)

		consecPayloadFailsCtr := 0
		for _, payload := range result.ConsecutiveFailedPayloads {
			if util.StrSliceContains(v.FailedPayloads, payload) {
				consecPayloadFailsCtr++
			}
		}
		v.ConsecutiveFailedPayloadsCount = consecPayloadFailsCtr
		// If at least the last three payloads are in rejected state, and this test is broken in all of them,
		// flag that we may have a blocker.
		// TODO: refine these criteria? 75% of last failures?
		if result.LastPhase == apitype.PayloadRejected && result.LastPhaseCount >= 3 &&
			consecPayloadFailsCtr == len(result.ConsecutiveFailedPayloads) {
			v.PossibleBlocker = true
		}
	}

	// sort so the most common test failures are first in the slice
	sort.Slice(testFailures, func(i, j int) bool { return testFailures[i].FailureCount >= testFailures[j].FailureCount })
	result.TestFailures = testFailures
	return result, nil
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
