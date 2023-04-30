package api

import (
	"errors"
	"fmt"
	"net/http"
	gosort "sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
	log "github.com/sirupsen/logrus"
)

const (
	// maxFailuresToFullyAnalyze is a limit to the number of failures we'll attempt to
	// individually analyze, if you exceed this the job failure is classified as high risk.
	maxFailuresToFullyAnalyze = 20
)

func (runs apiRunResults) sort(req *http.Request) apiRunResults {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))

	if sortField == "" {
		sortField = "test_failures"
	}

	if sort == "" {
		sort = apitype.SortDescending
	}

	gosort.Slice(runs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return filter.Compare(runs[i], runs[j], sortField)
		}
		return filter.Compare(runs[j], runs[i], sortField)
	})

	return runs
}

func (runs apiRunResults) limit(req *http.Request) apiRunResults {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(runs) >= limit {
		return runs[:limit]
	}

	return runs
}

type apiRunResults []apitype.JobRun

// JobsRunsReportFromDB renders a filtered summary of matching jobs.
func JobsRunsReportFromDB(dbc *db.DB, filterOpts *filter.FilterOptions, release string, pagination *apitype.Pagination, reportEnd time.Time) (*apitype.PaginationResult, error) {
	jobsResult := make([]apitype.JobRun, 0)
	table := "prow_job_runs_report_matview"
	q, err := filter.FilterableDBResult(dbc.DB.Table(table), filterOpts, apitype.JobRun{})
	if err != nil {
		return nil, err
	}

	if len(release) > 0 {
		q = q.Where("release = ?", release)
	}

	q = q.Where("timestamp < ?", reportEnd.UnixMilli())

	// Get the row count before pagination
	var rowCount int64
	q.Count(&rowCount)

	// Paginate the results:
	if pagination == nil {
		pagination = &apitype.Pagination{
			PerPage: int(rowCount),
			Page:    0,
		}
	} else {
		q = q.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	res := q.Scan(&jobsResult)
	return &apitype.PaginationResult{
		Rows:      jobsResult,
		TotalRows: rowCount,
		PageSize:  pagination.PerPage,
		Page:      pagination.Page,
	}, res.Error
}

func FetchJobRun(dbc *db.DB, jobRunID int64, logger *log.Entry) (*models.ProwJobRun, int, error) {

	jobRun := &models.ProwJobRun{}
	// Load the ProwJobRun, ProwJob, and failed tests:
	// TODO: we may want to expand to analyzing flakes here in the future
	res := dbc.DB.Joins("ProwJob").
		Preload("Tests", "status = 12").
		Preload("Tests.Test").
		Preload("Tests.Suite").First(jobRun, jobRunID)
	if res.Error != nil {
		return nil, -1, res.Error
	}

	jobRunTestCount, err := query.JobRunTestCount(dbc, jobRunID)
	if err != nil {
		logger.WithError(err).Error("Error getting job run test count")
		jobRunTestCount = -1
	}

	return jobRun, jobRunTestCount, nil
}

func findReleaseMatchJobNames(dbc *db.DB, jobRun *models.ProwJobRun, compareRelease string, logger *log.Entry) ([]string, int, error) {
	segments := strings.Split(jobRun.ProwJob.Name, "-")

	// if we don't find enough jobs to match against we can try the prior release
	// and see if it has enough, think about cutover to a new release, etc.

	for i := 0; i < len(segments); i++ {
		name := joinSegments(segments, i, "-")

		if len(name) > 0 {
			jobs, err := query.ProwJobSimilarName(dbc, name, compareRelease)

			if err != nil {
				logger.WithError(err).Errorf("Failed to find similar name for release: %s, root: %s", compareRelease, name)
			}

			if len(jobs) > 0 {
				logger.Infof("Found %d matches with: %s", len(jobs), name)

				// the first hit we get
				// compare the variants
				// for the matches
				// query the run count for each id
				// and total it up

				allJobNames := make([]string, 0)
				totalJobRunsCount := 0
				hasNeverStableJob := false
				variants := jobRun.ProwJob.Variants
				gosort.Strings(variants)
				for _, job := range jobs {
					// this is a weird way to get the variant we want, but it allows re-use
					// of the existing code.
					// how do we handle never-stable
					if len(job.Variants) == 1 && job.Variants[0] == testidentification.NeverStable {
						hasNeverStableJob = true
					} else {
						gosort.Strings(job.Variants)
						if stringSlicesEqual(variants, job.Variants) {

							jobIds, err := query.ProwJobRunIds(dbc, job.ID)

							if err != nil {
								logger.WithError(err).Errorf("Failed to query job run ids for %d", job.ID)
								continue
							}

							totalJobRunsCount += len(jobIds)
							allJobNames = append(allJobNames, "'"+job.Name+"'")
						}
					}
				}

				if len(allJobNames) == 0 && hasNeverStableJob {
					return nil, 0, errors.New(testidentification.NeverStable)
				}

				return allJobNames, totalJobRunsCount, nil
			}

		}
	}
	return nil, 0, nil
}

func joinSegments(segments []string, start int, separator string) string {
	if start > len(segments)-1 {
		return ""
	}
	return strings.Join(segments[start:], separator)
}

// JobRunRiskAnalysis checks the test failures and linked bugs for a job run, and reports back an estimated
// risk level for each failed test, and the job run overall.
func JobRunRiskAnalysis(dbc *db.DB, jobRun *models.ProwJobRun, jobRunTestCount int, logger *log.Entry) (apitype.ProwJobRunRiskAnalysis, error) {

	// If this job is a Presubmit, compare to test results from master, not presubmits, which may perform
	// worse due to dev code that hasn't merged. We do not presently track presubmits on branches other than
	// master, so it should be safe to assume the latest compareRelease in the db.
	compareRelease := jobRun.ProwJob.Release
	neverStableJob := false
	if compareRelease == "Presubmits" {
		// Get latest release from the DB:
		ar, err := query.ReleasesFromDB(dbc)
		if err != nil {
			return apitype.ProwJobRunRiskAnalysis{}, err
		}
		if len(ar) == 0 {
			return apitype.ProwJobRunRiskAnalysis{}, fmt.Errorf("no releases found in db")
		}

		compareRelease = ar[0].Release
	}

	historicalCount, err := query.ProwJobHistoricalTestCounts(dbc, jobRun.ProwJob.ID)

	// if we had an error we will continue the risk analysis and not elevate based on test counts
	if err != nil {
		logger.WithError(err).Error("Error comparing historical job run test count")
		historicalCount = 0
	}

	// -1 indicates an error getting the jobRunTest count we will log an error and skip this validation
	if jobRunTestCount < 0 {
		logger.Error("Unable to determine job run test count, initializing to historical count")
		jobRunTestCount = historicalCount
	} else if jobRunTestCount == 0 {
		// hack since we don't currently get the jobRunTestCount for 4.12 jobs.
		// If the jobRunTestCount is 0 and we are pre 4.13 set the jobRunTestCount to the historicalCount
		preSupportVersion, _ := version.NewVersion("4.12")
		currentVersion, _ := version.NewVersion(compareRelease)
		if preSupportVersion.GreaterThanOrEqual(currentVersion) {
			jobRunTestCount = historicalCount
		}
	}

	// we want to get a count of jobRunIds and fall back to include prior release if needed,
	// variants don't cover all of our cases, like etcd-scaling so we want to
	// find a job match against releases and analyze the pass rates
	jobNames, totalJobRuns, err := findReleaseMatchJobNames(dbc, jobRun, compareRelease, logger)

	if err != nil {
		if err.Error() == "never-stable" {
			neverStableJob = true
		} else {
			logger.WithError(err).Errorf("Failed to find matching jobIds for: %s", jobRun.ProwJob.Name)
		}
	}

	if totalJobRuns < 20 {
		// go back to the prior release and get more jobIds to compare against
		currentVersion, _ := version.NewVersion(compareRelease)
		majminor := currentVersion.Segments()
		// 4.14 is returned as 4,14,0
		if len(majminor) == 3 && majminor[1] > 0 {
			majminor[1]--
			priorRelease := fmt.Sprintf("%d.%d", majminor[0], majminor[1])
			priorJobNames, _, err := findReleaseMatchJobNames(dbc, jobRun, priorRelease, logger)

			if err != nil {
				// since this is for the prior release we won't return the never-stable error in this case
				if err.Error() != "never-stable" {
					logger.WithError(err).Errorf("Failed to find matching jobIds for: %s", jobRun.ProwJob.Name)
				}
			}

			jobNames = append(jobNames, priorJobNames...)
		}
	}

	logger.Infof("Found %d matching jobs for: %s", len(jobNames), jobRun.ProwJob.Name)

	// NOTE: we are including bugs for all releases, may want to filter here in future to just those
	// with an AffectsVersions that seems to match our compareRelease?
	jobBugs, err := query.LoadBugsForJobs(dbc, []int{int(jobRun.ProwJob.ID)}, true)
	if err != nil {
		logger.WithError(err).Errorf("Error evaluating bugs for prow job: %d", jobRun.ProwJob.ID)
	} else {
		jobRun.ProwJob.Bugs = jobBugs
	}

	// Pre-load test bugs as well:
	if len(jobRun.Tests) <= maxFailuresToFullyAnalyze {
		for i, tr := range jobRun.Tests {
			bugs, err := query.LoadBugsForTest(dbc, tr.Test.Name, true)
			if err != nil {
				logger.WithError(err).Errorf("Error evaluating bugs for prow job: %d, test name: %s", jobRun.ProwJob.ID, tr.Test.Name)
			} else {
				logger.Infof("Found %d bugs for test %s", len(bugs), tr.Test.Name)
				tr.Test.Bugs = bugs
				jobRun.Tests[i] = tr
			}
		}
	}

	return runJobRunAnalysis(jobRun, compareRelease, jobRunTestCount, historicalCount, neverStableJob, jobNames, logger.WithField("func", "runJobRunAnalysis"),
		jobNamesTestResultFunc(dbc))
}

// testResultsFunc is used for injecting db responses in unit tests.
type testResultsFunc func(testName string, release, suite string, variants []string, jobNames []string) (*apitype.Test, error)

// jobNamesTestResultFunc looks to match job runs based on the jobnames
func jobNamesTestResultFunc(dbc *db.DB) testResultsFunc {
	return func(testName, release, suite string, variants []string, jobNames []string) (*apitype.Test, error) {
		sql := fmt.Sprintf(query.QueryTestAnalysis, testName, strings.Join(jobNames, ","))
		testReport := apitype.Test{}
		q := dbc.DB.Raw(sql)

		if q.Error != nil {
			return nil, q.Error
		}

		q.First(&testReport)
		testReport.Name = testName
		// hack for now
		// cleanup the message if we convert
		testReport.Variants = jobNames
		return &testReport, nil
	}
}

// variantsTestResultFunc looks to match job runs based on variant matches
// this may be deprecated in favor of the jobnames match, need to verify
func variantsTestResultFunc(dbc *db.DB) testResultsFunc {
	return func(testName, release, suite string, variants []string, jobNames []string) (*apitype.Test, error) {

		fil := &filter.Filter{
			Items: []filter.FilterItem{
				{
					Field:    "name",
					Not:      false,
					Operator: filter.OperatorEquals,
					Value:    testName,
				},
			},
			LinkOperator: "and",
		}
		testResults, _, err := BuildTestsResults(dbc, release, "default", false, false,
			fil)
		if err != nil {
			return nil, err
		}
		gosort.Strings(variants)
		for _, testResult := range testResults {
			// this is a weird way to get the variant we want, but it allows re-use
			// of the existing code.
			gosort.Strings(testResult.Variants)
			if stringSlicesEqual(variants, testResult.Variants) && testResult.SuiteName == suite {
				return &testResult, nil
			}
		}

		// otherwise, what is our best match...
		// do something more expensive and check to see
		// which testResult contains all the variants we have currently
		for _, testResult := range testResults {
			// we didn't find an exact variant match
			// next best guess is the first variant list that contains all of our known variants
			if stringSubSlicesEqual(variants, testResult.Variants) && testResult.SuiteName == suite {
				return &testResult, nil
			}
		}

		return nil, nil
	}
}

func runJobRunAnalysis(jobRun *models.ProwJobRun, compareRelease string, jobRunTestCount int, historicalRunTestCount int, neverStableJob bool, jobNames []string, logger *log.Entry,
	testResultsFunc testResultsFunc) (apitype.ProwJobRunRiskAnalysis, error) {

	logger.Info("loaded prow job run for analysis")
	logger.Infof("this job run has %d failed tests", len(jobRun.Tests))

	response := apitype.ProwJobRunRiskAnalysis{
		ProwJobRunID:   jobRun.ID,
		ProwJobName:    jobRun.ProwJob.Name,
		Release:        jobRun.ProwJob.Release,
		CompareRelease: compareRelease,
		Tests:          []apitype.ProwJobRunTestRiskAnalysis{},
		OverallRisk: apitype.FailureRisk{
			Level:   apitype.FailureRiskLevelNone,
			Reasons: []string{},
		},
		OpenBugs: jobRun.ProwJob.Bugs,
	}

	switch {

	// Return early if we see a large gap in the number of tests:
	// order matters, if we have 0 tests that ran && 0 tests that failed we
	// want to compare that here before the 'no test failures' case
	case jobRunTestCount < (int(float64(historicalRunTestCount) * .75)):
		response.OverallRisk.Level = apitype.FailureRiskLevelIncompleteTests
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("Tests for this run (%d) are below the historical average (%d): IncompleteTests", jobRunTestCount, historicalRunTestCount))
		return response, nil

	// Return early if no tests failed in this run:
	case len(jobRun.Tests) == 0:
		response.OverallRisk.Level = apitype.FailureRiskLevelNone
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			"No test failures found in this job run.")
		return response, nil

	// Return early if we see mass test failures:
	case len(jobRun.Tests) > maxFailuresToFullyAnalyze:
		response.OverallRisk.Level = apitype.FailureRiskLevelHigh
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("%d tests failed in this run: High", len(jobRun.Tests)))
		return response, nil

	// Return early is this is for a never-stable job
	case neverStableJob:
		response.OverallRisk.Level = apitype.FailureRiskLevelLow
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			"Job is marked as never-stable")
		return response, nil
	}

	var maxTestRiskReason string

	// Iterate each failed test, query it's pass rates by NURPs, find a matching variant combo, and
	// see how often we've passed in the last week.
	for _, ft := range jobRun.Tests {

		if ft.Test.Name == testidentification.OpenShiftTestsName || testidentification.IsIgnoredTest(ft.Test.Name) {
			continue
		}

		logger.WithFields(log.Fields{
			"name": ft.Test.Name,
		}).Debug("failed test")

		testResult, err := testResultsFunc(
			ft.Test.Name, compareRelease, ft.Suite.Name, jobRun.ProwJob.Variants, jobNames)
		if err != nil {
			return response, err
		}
		// Watch out for tests that ran in previous period, but not current, no sense comparing to 0 runs:
		if testResult != nil && testResult.CurrentRuns > 0 {
			testRiskLvl := getSeverityLevelForPassRate(testResult.CurrentPassPercentage)
			if testRiskLvl.Level >= response.OverallRisk.Level.Level {
				response.OverallRisk.Level = testRiskLvl
				maxTestRiskReason = fmt.Sprintf("Maximum failed test risk: %s", testRiskLvl.Name)
			}
			response.Tests = append(response.Tests, apitype.ProwJobRunTestRiskAnalysis{
				Name: testResult.Name,
				Risk: apitype.FailureRisk{
					Level: testRiskLvl,
					Reasons: []string{
						fmt.Sprintf("This test has passed %.2f%% of %d runs on release %s %v in the last week.",
							testResult.CurrentPassPercentage, testResult.CurrentRuns, compareRelease, testResult.Variants),
					},
				},
				OpenBugs: ft.Test.Bugs,
			})
		} else {
			testRiskLvl := apitype.FailureRiskLevelUnknown
			if testRiskLvl.Level >= response.OverallRisk.Level.Level {
				response.OverallRisk.Level = testRiskLvl
				maxTestRiskReason = fmt.Sprintf("Maximum failed test risk: %s", testRiskLvl.Name)
			}
			response.Tests = append(response.Tests, apitype.ProwJobRunTestRiskAnalysis{
				Name: ft.Test.Name,
				Risk: apitype.FailureRisk{
					Level: testRiskLvl,
					Reasons: []string{
						fmt.Sprintf("Unable to find matching test results for variants: %v",
							jobRun.ProwJob.Variants),
					},
				},
				OpenBugs: ft.Test.Bugs,
			})
		}
	}

	response.OverallRisk.Reasons = append(response.OverallRisk.Reasons, maxTestRiskReason)

	return response, nil
}

func getSeverityLevelForPassRate(passPercentage float64) apitype.RiskLevel {
	switch {
	case passPercentage >= 98.0:
		return apitype.FailureRiskLevelHigh
	case passPercentage >= 80:
		return apitype.FailureRiskLevelMedium
	case passPercentage < 80:
		return apitype.FailureRiskLevelLow
	}
	return apitype.FailureRiskLevelUnknown
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func stringSubSlicesEqual(a, b []string) bool {
	// we are going to check if b contains all the values in a

	// have to have something to match on
	// and be less than or equal to b
	if len(a) < 1 {
		return false
	}
	if len(a) > len(b) {
		return false
	}
	for _, v := range a {
		found := false
		for _, s := range b {
			if v == s {
				found = true
			}
		}

		if !found {
			return false
		}
	}
	return true
}
