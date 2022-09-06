package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
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

type PayloadFailedTest struct {
	ID            uint
	Release       string
	Architecture  string
	Stream        string
	ReleaseTag    string
	TestID        uint
	SuiteID       uint
	Status        int
	Name          string
	ProwJobRunID  uint
	ProwJobRunURL string
	ProwJobName   string
}

// GetPayloadStreamTestFailures loads the most recent payloads for a stream and attempts to search for most commonly
// failing tests, possible perma-failing blockers, etc.
func GetPayloadStreamTestFailures(dbc *db.DB, release, stream, arch string, filterOpts *filter.FilterOptions, reportEnd time.Time) ([]*apitype.TestFailureAnalysis, error) {

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
		stream, arch, reportEnd)
	if err != nil {
		return nil, err
	}
	logger.WithField("payloads", len(lastPayloads)).Debug("loaded last payloads")
	result.PayloadsAnalyzed = len(lastPayloads)

	if len(lastPayloads) == 0 {
		logger.Debug("no payload tags found")
		return result.TestFailures, nil
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

	// Query all test failures for the given payload stream in the last two weeks:
	failedTests := []PayloadFailedTest{}
	q := dbc.DB.Table(payloadFailedTests14dMatView).
		Where("release = ?", release).
		Where("architecture = ?", arch).
		Where("stream = ?", stream).
		Order("release_tag DESC")
	q, err = filter.FilterableDBResult(q, filterOpts, nil)
	if err != nil {
		return nil, err
	}
	q.Find(&failedTests)
	logger.WithField("failedTestCount", len(failedTests)).Debug("found failed tests")

	// Iterate all failed tests, build structs showing what payloads and jobs it failed in.
	testNameToAnalysis := map[string]*apitype.TestFailureAnalysis{}
	for _, ft := range failedTests {
		if ft.Name == testidentification.OpenShiftTestsName {
			// Skip the "all tests passed" test we inject, it's not relevant here.
			continue
		}
		if _, ok := testNameToAnalysis[ft.Name]; !ok {
			testNameToAnalysis[ft.Name] = &apitype.TestFailureAnalysis{
				Name:                ft.Name,
				ID:                  ft.TestID,
				FailedPayloads:      map[string]*apitype.FailedPayload{},
				BlockerScoreReasons: []string{},
			}
		}
		ta := testNameToAnalysis[ft.Name]
		ta.FailureCount++
		pl := ft.ReleaseTag
		if _, ok := ta.FailedPayloads[pl]; !ok {
			ta.FailedPayloads[pl] = &apitype.FailedPayload{
				FailedJobs:    []string{},
				FailedJobRuns: []string{},
			}
		}

		ta.FailedPayloads[pl].FailedJobs = append(ta.FailedPayloads[pl].FailedJobs, ft.ProwJobName)
		ta.FailedPayloads[pl].FailedJobRuns = append(ta.FailedPayloads[pl].FailedJobRuns, ft.ProwJobRunURL)
	}
	testFailures := make([]*apitype.TestFailureAnalysis, 0, len(testNameToAnalysis))

	for _, v := range testNameToAnalysis {
		testFailures = append(testFailures, v)
		calculateBlockerScore(result.ConsecutiveFailedPayloads, v)
	}

	// sort so the most likely blocker test failures are first in the slice:
	sort.Slice(testFailures, func(i, j int) bool { return testFailures[i].BlockerScore >= testFailures[j].BlockerScore })
	result.TestFailures = testFailures
	return result.TestFailures, nil
}

// calculateBlockerScore uses the list of most recent failed payloads, and compares to the failures we found
// for a particular test, then attempts to calculate a blocker score between 0.0 (not a blocker) and 1.0 (almost
// certainly a blocker) based on a number of criteria.
//
// consecutiveFailedPayloadTags is the list of our current streak of rejected payload tags. If most recent payload
// was accepted, this list will be empty, and we don't have much processing to do.
func calculateBlockerScore(consecutiveFailedPayloadTags []string, ta *apitype.TestFailureAnalysis) {
	if len(consecutiveFailedPayloadTags) == 0 {
		// our most recent state is Accepted, could be intermittent, but for the purposes of a blocker
		// we have to assume 0.
		ta.BlockerScore = 0
		ta.BlockerScoreReasons = append(ta.BlockerScoreReasons, "most recent payload was Accepted, test may be failing intermittently but cannot be fully blocking")
		return
	}

	failedInConsecPayloads := 0
	var failedInConsecBreakFound bool
	failedInStreak := 0
	for _, payloadTag := range consecutiveFailedPayloadTags {
		if _, ok := ta.FailedPayloads[payloadTag]; ok {
			if !failedInConsecBreakFound {
				failedInConsecPayloads++
			}
			failedInStreak++
		} else {
			// We didn't fail in a payload in the current streak of failures, but this could be infra
			// failures, so we keep checking if we failed in more beyond this.
			failedInConsecBreakFound = true
		}
	}

	// Currently assuming 25% per consecutive failure, at 4 we are 100% sure this is a blocker.
	ta.BlockerScore = int(math.Min(float64(failedInConsecPayloads*25), 100))
	message := fmt.Sprintf("failed in %d most recent rejected payloads", failedInConsecPayloads)
	ta.BlockerScoreReasons = append(ta.BlockerScoreReasons, message)

	// Override the score if we see we failed in a more substantial portion of the current rejected streak
	// (a test can disappear in a run if it fails on infra or other reasons).
	failedInStreakPercentage := int((float64(failedInStreak) / float64(len(consecutiveFailedPayloadTags))) * 100)
	ta.BlockerScoreReasons = append(ta.BlockerScoreReasons,
		fmt.Sprintf("failed in %d/%d of current rejected payload streak", failedInStreak, len(consecutiveFailedPayloadTags)))
	if ta.BlockerScore >= 50 && failedInStreakPercentage >= ta.BlockerScore {
		ta.BlockerScore = failedInStreakPercentage
	}

}

// GetPayloadEvents returns the list of release tags in a format suitable for a calendar
// like FullCalendar.
func GetPayloadEvents(dbClient *db.DB, release string, filterOpts *filter.FilterOptions,
	start, end *time.Time) ([]apitype.PayloadEvent, error) {
	releases := make([]apitype.PayloadEvent, 0)

	if dbClient == nil || dbClient.DB == nil {
		return nil, fmt.Errorf("no db client found")
	}

	q, err := filter.FilterableDBResult(dbClient.DB.Where("release = ?", release), filterOpts, nil)
	if err != nil {
		return nil, err
	}

	q.Table("release_tags").
		Select(`DATE(release_tags.release_time) as start, release_tag as title, phase, 'TRUE' as all_day`)

	if start != nil {
		q = q.Where("release_time >= ?", start)
	}

	if end != nil {
		q = q.Where("release_time <= ?", end)
	}

	q.Scan(&releases)

	return releases, nil
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
func ReleaseHealthReports(dbClient *db.DB, release string, reportEnd time.Time) ([]apitype.ReleaseHealthReport, error) {
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

		totalPhaseCountsDB, err := query.GetPayloadStreamPhaseCounts(dbClient.DB, release, archStream.Architecture, archStream.Stream, nil)
		if err != nil {
			return apiResults, errors.Wrapf(err, "error finding %s payload status counts for %s %s",
				release, archStream.Architecture, archStream.Stream)
		}
		totalAcceptanceStatistics, err := query.GetPayloadAcceptanceStatistics(dbClient.DB, release, archStream.Architecture, archStream.Stream, nil)
		if err != nil {
			return apiResults, errors.Wrapf(err, "error finding %s payload acceptance statistics for %s %s",
				release, archStream.Architecture, archStream.Stream)
		}

		weekAgo := reportEnd.Add(-7 * 24 * time.Hour)
		currentWeekPhaseCountsDB, err := query.GetPayloadStreamPhaseCounts(dbClient.DB, release, archStream.Architecture, archStream.Stream, &weekAgo)
		if err != nil {
			return apiResults, errors.Wrapf(err, "error finding %s payload status counts for %s %s",
				release, archStream.Architecture, archStream.Stream)
		}
		currentWeekAcceptanceStatistics, err := query.GetPayloadAcceptanceStatistics(dbClient.DB, release, archStream.Architecture, archStream.Stream, &weekAgo)
		if err != nil {
			return apiResults, errors.Wrapf(err, "error finding %s payload acceptance statistics for %s %s",
				release, archStream.Architecture, archStream.Stream)
		}

		currentWeekPhaseCounts := dbPayloadPhaseCountToAPI(currentWeekPhaseCountsDB)
		totalPhaseCounts := dbPayloadPhaseCountToAPI(totalPhaseCountsDB)

		apiResults = append(apiResults, apitype.ReleaseHealthReport{
			ReleaseTag: archStream,
			LastPhase:  phase,
			Count:      count,
			PhaseCounts: apitype.PayloadPhaseCounts{
				CurrentWeek: currentWeekPhaseCounts,
				Total:       totalPhaseCounts,
			},
			PayloadStatistics: apitype.PayloadStatistics{
				CurrentWeek: apitype.PayloadStatistic{PayloadStatistics: currentWeekAcceptanceStatistics},
				Total:       apitype.PayloadStatistic{PayloadStatistics: totalAcceptanceStatistics},
			},
		})
	}

	return apiResults, nil
}

func dbPayloadPhaseCountToAPI(dbpc []models.PayloadPhaseCount) apitype.PayloadPhaseCount {
	apipc := apitype.PayloadPhaseCount{}
	for _, c := range dbpc {
		switch c.Phase {
		case "Accepted":
			apipc.Accepted = c.Count
		case "Rejected":
			apipc.Rejected = c.Count
		default:
			log.Warnf("Unexpected payload phase: %s", c.Phase)
		}
	}
	return apipc
}

// ScanForReleaseWarnings looks for problems in current release health and returns them to the user.
func ScanForReleaseWarnings(dbClient *db.DB, release string, reportEnd time.Time) []string {
	payloadHealth, err := ReleaseHealthReports(dbClient, release, reportEnd)
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
