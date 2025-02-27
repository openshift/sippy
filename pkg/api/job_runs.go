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
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/param"
	log "github.com/sirupsen/logrus"
)

const (
	// maxFailuresToFullyAnalyze is a limit to the number of failures we'll attempt to
	// individually analyze, if you exceed this the job failure is classified as high risk.
	maxFailuresToFullyAnalyze = 20
)

// nonDeterministicRiskLevels indicate incomplete analysis and allow for fallback to other analysis methodologies name -> variant
var nonDeterministicRiskLevels = []int{apitype.FailureRiskLevelUnknown.Level, apitype.FailureRiskLevelIncompleteTests.Level, apitype.FailureRiskLevelMissingData.Level}

func (runs apiRunResults) sort(req *http.Request) apiRunResults {
	sortField := param.SafeRead(req, "sortField")
	sort := apitype.Sort(param.SafeRead(req, "sort"))

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

// FetchJobRun returns a single job run loaded from postgres and populated with the ProwJob and test results.
// If unknownTests is true, all tests not registered in test_ownerships are loaded; otherwise any failed tests are loaded.
func FetchJobRun(dbc *db.DB, jobRunID int64, unknownTests bool, logger *log.Entry) (*models.ProwJobRun, error) {
	jobRun := &models.ProwJobRun{}

	// Load the ProwJobRun, ProwJob, and (failed|unknown) tests:
	// TODO: we may want to expand to analyzing flakes here in the future
	q := dbc.DB.Joins("ProwJob")
	if unknownTests {
		// this doesn't establish that the tests are new, but it does filter out any that sippy registers
		q = q.Preload("Tests", "test_id not in (select test_id from test_ownerships)")
	} else { // load only failures
		q = q.Preload("Tests", "status = ?", sippyprocessingv1.TestStatusFailure)
	}
	res := q.Preload("Tests.Test").
		Preload("Tests.Suite").
		First(jobRun, jobRunID)
	if res.Error != nil {
		return nil, res.Error
	}

	jobRunTestCount, err := query.JobRunTestCount(dbc, jobRunID)
	if err != nil { // should be unusual
		logger.WithError(err).Errorf("Error getting test count for job run %d", jobRunID)
		jobRunTestCount = -1
	}
	jobRun.TestCount = jobRunTestCount

	return jobRun, nil
}

// findReleaseMatchJobNames looks for the first matches with a common root job name specific to the
// compareRelease and the prowJob variants, starting with the full name.  When no match is found it will iterate while
// removing the leading 'string-'
// and try to find a match until successful or no matches are found.
//
// The use case is for pull request jobs that we want to find a matching periodic that is running the
// same root job.  We use the periodic as the 'standard' to compare test rates.
// e.g.
// pull-ci-openshift-origin-master- e2e-vsphere-ovn-etcd-scaling
// periodic-ci-openshift-release-master-nightly-4.14- e2e-vsphere-ovn-etcd-scaling
// our common root is e2e-vsphere-ovn-etcd-scaling and our compareRelease is 4.14
// if we don't have enough data from the current compareRelease we fall back to include the previous release as well
func findReleaseMatchJobNames(dbc *db.DB, jobRun *models.ProwJobRun, compareRelease string, logger *log.Entry) ([]string, int, error) {
	segments := strings.Split(jobRun.ProwJob.Name, "-")
	logger = logger.WithField("func", "findReleaseMatchJobNames").WithField("job", jobRun.ProwJob.Name)

	// if we don't find enough jobs to match against we can try the prior release
	// and see if it has enough, think about cutover to a new release, etc.

	for i := 0; i < len(segments); i++ {

		// pull-ci-openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// ci-openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// openshift-origin-master-e2e-vsphere-ovn-etcd-scaling
		// origin-master-e2e-vsphere-ovn-etcd-scaling
		// master-e2e-vsphere-ovn-etcd-scaling
		// e2e-vsphere-ovn-etcd-scaling
		// matches periodic-ci-openshift-release-master-nightly-4.14-e2e-vsphere-ovn-etcd-scaling
		// when we specify the 4.14 release
		name := joinSegments(segments, i, "-")

		if len(name) > 0 {
			jobs, err := query.ProwJobSimilarName(dbc, name, compareRelease)

			if err != nil {
				logger.WithError(err).Errorf("Failed to find similar name for release: %s, root: %s", compareRelease, name)
			}

			if len(jobs) > 0 {
				logger.Debugf("Found %d potential name matches", len(jobs))

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
					}

					gosort.Strings(job.Variants)
					if stringSlicesEqual(variants, job.Variants) {

						jobIDs, err := query.ProwJobRunIDs(dbc, job.ID)

						if err != nil {
							logger.WithError(err).Errorf("Failed to query job run ids for %d", job.ID)
							continue
						}

						totalJobRunsCount += len(jobIDs)
						allJobNames = append(allJobNames, job.Name)
					}
				}

				// logging at info for now so we can monitor, can dial down to debug if / when preferred
				if len(allJobNames) > 0 {
					logger.Infof("Matched job name: %s to %v", jobRun.ProwJob.Name, allJobNames)
				}

				var err error
				if hasNeverStableJob {
					err = errors.New(testidentification.NeverStable)
				}

				return allJobNames, totalJobRunsCount, err
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
func JobRunRiskAnalysis(dbc *db.DB, jobRun *models.ProwJobRun, logger *log.Entry) (apitype.ProwJobRunRiskAnalysis, error) {
	logger = logger.WithField("func", "JobRunRiskAnalysis")
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
	if jobRun.TestCount < 0 {
		logger.Error("Unable to determine job run test count, initializing to historical count")
		jobRun.TestCount = historicalCount
	} else if jobRun.TestCount == 0 {
		// hack since we don't currently get the jobRunTestCount for 4.12 jobs.
		// If the jobRunTestCount is 0 and we are pre 4.13 set the jobRunTestCount to the historicalCount
		preSupportVersion, _ := version.NewVersion("4.12")
		currentVersion, err := version.NewVersion(compareRelease)
		if err != nil {
			logger.WithError(err).Errorf("Failed to parse release '%s' for prow job %d", compareRelease, jobRun.ProwJob.ID)
		} else if preSupportVersion.GreaterThanOrEqual(currentVersion) {
			jobRun.TestCount = historicalCount
		}
	}

	// we want to get a list of job names and a count of jobRunIds and fall back to include prior release if needed,
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
		if currentVersion, err := version.NewVersion(compareRelease); err != nil {
			logger.WithError(err).Errorf("Failed to parse release '%s' for prow job %d", compareRelease, jobRun.ProwJob.ID)
		} else {
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
	}

	logger.Infof("Found %d matching job(s) for: %s", len(jobNames), jobRun.ProwJob.Name)

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
				logger.Debugf("Found %d bugs for test '%s'", len(bugs), tr.Test.Name)
				tr.Test.Bugs = bugs
				jobRun.Tests[i] = tr
			}
		}
	}

	return runJobRunAnalysis(jobRun, compareRelease, historicalCount, neverStableJob, jobNames, logger, jobNamesTestResultFunc(dbc), variantsTestResultFunc(dbc))
}

// testResultsByJobNameFunc is used for injecting db responses in unit tests.
type testResultsByJobNameFunc func(testName string, jobNames []string) (*apitype.Test, error)

type testResultsByVariantsFunc func(testName string, release, suite string, variants []string, jobNames []string) (*apitype.Test, error)

// jobNamesTestResultFunc looks to match job runs based on the jobnames
func jobNamesTestResultFunc(dbc *db.DB) testResultsByJobNameFunc {
	return func(testName string, jobNames []string) (*apitype.Test, error) {
		if len(jobNames) == 0 {
			return nil, nil
		}

		analyzeSince := time.Now().Add(-14 * 24 * time.Hour)

		q := dbc.DB.Raw(query.QueryTestAnalysis, analyzeSince, testName, jobNames)
		if q.Error != nil {
			return nil, q.Error
		}

		testReport := apitype.Test{}
		q.First(&testReport)
		testReport.Name = testName
		return &testReport, nil
	}
}

// variantsTestResultFunc looks to match job runs based on variant matches
func variantsTestResultFunc(dbc *db.DB) testResultsByVariantsFunc {
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
		testResults, overallTest, err := BuildTestsResults(dbc, release, "default", false, true,
			fil)
		if err != nil {
			return nil, err
		}
		if overallTest != nil {
			overallTest.Variants = append(overallTest.Variants, "Overall")
		}
		gosort.Strings(variants)
		for _, testResult := range testResults {
			// this is a weird way to get the variant we want, but it allows re-use
			// of the existing code.
			gosort.Strings(testResult.Variants)
			if stringSlicesEqual(variants, testResult.Variants) && testResult.SuiteName == suite {
				if overallTest.CurrentPassPercentage < testResult.CurrentPassPercentage {
					return overallTest, nil
				}
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
				if overallTest.CurrentPassPercentage < testResult.CurrentPassPercentage {
					return overallTest, nil
				}
				return &testResult, nil
			}
		}

		return nil, nil
	}
}

func runJobRunAnalysis(jobRun *models.ProwJobRun, compareRelease string, historicalRunTestCount int, neverStableJob bool, jobNames []string, logger *log.Entry,
	testResultsJobNameFunc testResultsByJobNameFunc, testResultsVariantsFunc testResultsByVariantsFunc) (apitype.ProwJobRunRiskAnalysis, error) {

	logger = logger.WithField("func", "runJobRunAnalysis").WithField("job", jobRun.ProwJob.Name)
	logger.Infof("analyzing prow job run with %d failed test(s)", len(jobRun.Tests))

	response := apitype.ProwJobRunRiskAnalysis{
		ProwJobRunID:   jobRun.ID,
		ProwJobName:    jobRun.ProwJob.Name,
		Release:        jobRun.ProwJob.Release,
		CompareRelease: compareRelease,
		Tests:          []apitype.TestRiskAnalysis{},
		OverallRisk: apitype.JobFailureRisk{
			Level:                  apitype.FailureRiskLevelNone,
			Reasons:                []string{},
			JobRunTestCount:        jobRun.TestCount,
			JobRunTestFailures:     len(jobRun.Tests),
			NeverStableJob:         neverStableJob,
			HistoricalRunTestCount: historicalRunTestCount,
		},
		OpenBugs: jobRun.ProwJob.Bugs,
	}

	switch {

	// Return early if we see a large gap in the number of tests:
	// order matters, if we have 0 tests that ran && 0 tests that failed we
	// want to compare that here before the 'no test failures' case
	case jobRun.TestCount < (int(float64(historicalRunTestCount) * .75)):
		response.OverallRisk.Level = apitype.FailureRiskLevelIncompleteTests
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("Tests for this run (%d) are below the historical average (%d): IncompleteTests (not enough tests ran to make a reasonable risk analysis; this could be due to infra, installation, or upgrade problems)", jobRun.TestCount, historicalRunTestCount))
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
	}

	maxTestRisk := apitype.FailureRiskLevelNone

	for _, ft := range jobRun.Tests {

		if ft.Test.Name == testidentification.OpenShiftTestsName || testidentification.IsIgnoredTest(ft.Test.Name) {
			continue
		}

		loggerFields := logger.WithField("test", ft.Test.Name)
		analysis, err := runTestRunAnalysis(ft, jobRun, compareRelease, loggerFields, testResultsJobNameFunc, jobNames, testResultsVariantsFunc, neverStableJob)
		if err != nil {
			continue // ignore runs where analysis failed
		}
		if analysis.Risk.Level.Level > maxTestRisk.Level {
			maxTestRisk = analysis.Risk.Level
		}
		response.Tests = append(response.Tests, analysis)
	}
	if maxTestRisk.Level >= response.OverallRisk.Level.Level {
		response.OverallRisk.Level = maxTestRisk
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons, fmt.Sprintf("Maximum failed test risk: %s", maxTestRisk.Name))
	}

	return response, nil
}

// For a failed test, query its pass rates by NURPs, find a matching variant combo, and
// see how often we've passed in the last week.
func runTestRunAnalysis(failedTest models.ProwJobRunTest, jobRun *models.ProwJobRun, compareRelease string, logger *log.Entry, testResultsJobNameFunc testResultsByJobNameFunc, jobNames []string, testResultsVariantsFunc testResultsByVariantsFunc, neverStableJob bool) (apitype.TestRiskAnalysis, error) {

	logger.Debug("failed test")

	var testResultsJobNames, testResultsVariants *apitype.Test
	var errJobNames, errVariants error

	// set upper and lower bounds for the number of jobNames we look to match against
	if testResultsJobNameFunc != nil {
		if len(jobNames) < 5 && len(jobNames) > 0 {
			testResultsJobNames, errJobNames = testResultsJobNameFunc(failedTest.Test.Name, jobNames)

			if errJobNames == nil && testResultsJobNames != nil {
				if testResultsJobNames.CurrentRuns == 0 {
					// do we need to prepend the suite name to the test?
					testResultsJobNames, errJobNames = testResultsJobNameFunc(fmt.Sprintf("%s.%s", failedTest.Suite.Name, failedTest.Test.Name), jobNames)
				}
			}

		} else {
			logger.Warningf("Skipping job names test analysis due to jobNames length: %d", len(jobNames))
		}
	}

	// if this matched a neverStableJob we don't want to use the variant match as it will include
	// results from stable jobs and potentially skew results.
	// we will rely on the jobname match, if any, for analysis
	if testResultsVariantsFunc != nil && !neverStableJob {
		testResultsVariants, errVariants = testResultsVariantsFunc(failedTest.Test.Name, compareRelease, failedTest.Suite.Name, jobRun.ProwJob.Variants, jobNames)

		if errVariants == nil && (testResultsVariants == nil || testResultsVariants.CurrentRuns == 0) {
			// do we need to prepend the suite name to the test?
			// drop passing the suite name to the func as we are prepending it to the test name
			testResultsVariants, errVariants = testResultsVariantsFunc(fmt.Sprintf("%s.%s", failedTest.Suite.Name, failedTest.Test.Name), compareRelease, "", jobRun.ProwJob.Variants, jobNames)
		}
	}

	if errJobNames != nil && errVariants != nil {
		logger.WithError(errVariants).Error("Failed test results by variants")
		logger.WithError(errJobNames).Error("Failed test results job names")
		return apitype.TestRiskAnalysis{}, errJobNames
	}

	analysis := apitype.TestRiskAnalysis{
		Name:     failedTest.Test.Name,
		TestID:   failedTest.Test.ID,
		OpenBugs: failedTest.Test.Bugs,
	}
	// Watch out for tests that ran in previous period, but not current, no sense comparing to 0 runs:
	if (testResultsVariants != nil && testResultsVariants.CurrentRuns > 0) || (testResultsJobNames != nil && testResultsJobNames.CurrentRuns > 0) {
		// select the 'best' test result
		analysis.Risk = selectRiskAnalysisResult(testResultsJobNames, testResultsVariants, jobNames, compareRelease)
	} else {
		analysis.Risk = apitype.TestFailureRisk{
			Level: apitype.FailureRiskLevelUnknown,
			Reasons: []string{
				fmt.Sprintf("Unable to find matching test results for variants: %v",
					jobRun.ProwJob.Variants),
			},
		}
	}
	return analysis, nil
}

func selectRiskAnalysisResult(testResultsJobNames, testResultsVariants *apitype.Test, jobNames []string, compareRelease string) apitype.TestFailureRisk {

	var variantRisk, jobRisk apitype.TestFailureRisk

	if testResultsJobNames != nil && testResultsJobNames.CurrentRuns > 0 {
		jobRisk = apitype.TestFailureRisk{
			Level: getSeverityLevelForPassRate(testResultsJobNames.CurrentPassPercentage),
			Reasons: []string{
				fmt.Sprintf("This test has passed %.2f%% of %d runs on jobs %v in the last 14 days.",
					testResultsJobNames.CurrentPassPercentage, testResultsJobNames.CurrentRuns, jobNames),
			},
			CurrentRuns:           testResultsJobNames.CurrentRuns,
			CurrentPassPercentage: testResultsJobNames.CurrentPassPercentage,
			CurrentPasses:         testResultsJobNames.CurrentSuccesses,
		}
	}

	if testResultsVariants != nil && testResultsVariants.CurrentRuns > 0 {
		variantRisk = apitype.TestFailureRisk{
			Level: getSeverityLevelForPassRate(testResultsVariants.CurrentPassPercentage),
			Reasons: []string{
				fmt.Sprintf("This test has passed %.2f%% of %d runs on release %s %v in the last week.",
					testResultsVariants.CurrentPassPercentage, testResultsVariants.CurrentRuns, compareRelease, testResultsVariants.Variants),
			},
			CurrentRuns:           testResultsVariants.CurrentRuns,
			CurrentPassPercentage: testResultsVariants.CurrentPassPercentage,
			CurrentPasses:         testResultsVariants.CurrentSuccesses,
		}

	}

	// if both are empty then return Unknown
	if len(jobRisk.Level.Name) == 0 && len(variantRisk.Level.Name) == 0 {
		return apitype.TestFailureRisk{
			Level:   apitype.FailureRiskLevelUnknown,
			Reasons: []string{"Analysis was not performed for this test due to lack of current runs"},
		}
	}

	switch {
	// if one is empty return the other
	case len(jobRisk.Level.Name) == 0:
		return variantRisk
	case len(variantRisk.Level.Name) == 0:
		return jobRisk
	case containsValue(nonDeterministicRiskLevels, jobRisk.Level.Level):
		// if jobnames nondeterministic then return variants
		return variantRisk
	case containsValue(nonDeterministicRiskLevels, variantRisk.Level.Level):
		// if variants nondeterministic then return jobnames
		return jobRisk
	case variantRisk.Level.Level < jobRisk.Level.Level:
		// biased to return the lower risk level
		return variantRisk
	default:
		return jobRisk
	}
}

func containsValue(values []int, value int) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
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
