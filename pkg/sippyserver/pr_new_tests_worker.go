package sippyserver

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	sippyApi "github.com/openshift/sippy/pkg/api"
	apiModels "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/prow"
	spv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
)

// NewTest represents a test result whose name has not been seen in merged code before.
// we want to call out these out in comments alongside risk analysis.
type NewTest struct {
	JobName  string
	JobRunID uint
	TestName string
	Success  bool
	Failure  bool // and if both, it's a flake
}

// NewTestRisk is created for the following scenarios:
// 1. Any PR job adds a new test that appears in one run and not another at the same sha - high risk
// 2. PR adds new test that appears in only a single job and:
//   - it fails at all - high risk
//   - it succeeds or flakes - medium risk (might not be intended for multiple jobs)
//
// 3. PR adds new test that appears in more than one job and (at latest sha):
//   - it fails at all  - high risk
//   - it succeeds or flakes - no risk (only included in list of all new tests)
type NewTestRisk struct {
	TestName   string
	AnyMissing bool // it was a new test in one run but missing in another at same sha
	Runs       int  // how many job runs did we examine
	Failures   int  // how many of the test results were failure
	Flakes     int  // or flakes
	OnlyInOne  bool // new test was only seen in one job of multiple for this PR
	NewTests   []NewTest
	Level      apiModels.RiskLevel
	Reason     string
}

// JobNewTestRisks represents the new test risks for (all runs of) a single job
type JobNewTestRisks struct {
	JobName      string
	NewTestRisks map[string]*NewTestRisk // one risk record per new test name
}

func SortByJobNameNT(risks []*JobNewTestRisks) {
	slices.SortFunc(risks, func(a, b *JobNewTestRisks) int {
		return strings.Compare(a.JobName, b.JobName)
	})
}

type NewTestFilter interface {
	// IsNewTest given a candidate test determines if it is really new with this PR
	IsNewTest(logger *logrus.Entry, test models.ProwJobRunTest) (bool, error)
}

// pgNewTestFilter queries postgres to determine if a test is new. We can share
// a single instance between workers and cache results so we are not constantly
// querying postgres for the same test.
type pgNewTestFilter struct {
	dbc         *db.DB
	notNewTests sets.Set[uint] // cache of test names that turn out not to be new
	nnTmutex    *sync.Mutex    // protect notNewTests from concurrent access
}

type JobRunFilter interface {
	// OnlyLatestSha filters out runs that are not against the PR's latest sha
	OnlyLatestSha(entry *logrus.Entry, info prJobInfo) []*prow.ProwJob
	// JobFailedEarly determines if a run did not get far enough to be included in new test analysis
	JobFailedEarly(logger *logrus.Entry, run *models.ProwJobRun) bool
}

type pgJobRunFilter struct {
	dbc                 *db.DB
	historicalTestCount map[uint]int // in-memory cache of historical test counts per job id
}

// NewTestsWorker analyzes PR jobs looking for new tests and determining their risks.
type NewTestsWorker struct {
	dbc           *db.DB
	newTestFilter NewTestFilter
	jobRunFilter  JobRunFilter
	fetchJobRun   func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error)
}

// StandardNewTestsWorker is a convenience method to create a NewTestsWorker with standard filters
func StandardNewTestsWorker(dbc *db.DB) *NewTestsWorker {
	_, ntw := internalNewTestsWorker(dbc)
	return ntw
}
func internalNewTestsWorker(dbc *db.DB) (*pgNewTestFilter, *NewTestsWorker) {
	ntf := &pgNewTestFilter{dbc: dbc, notNewTests: sets.Set[uint]{}, nnTmutex: &sync.Mutex{}}
	jrf := &pgJobRunFilter{dbc: dbc, historicalTestCount: map[uint]int{}}
	ntw := &NewTestsWorker{
		dbc:           dbc,
		newTestFilter: ntf,
		jobRunFilter:  jrf,
		fetchJobRun:   sippyApi.FetchJobRun,
	}
	return ntf, ntw // for tests it can be useful to have these explicitly
}

// analyzeRisks processes one job's runs looking for new tests and assessing their risk
func (ntw *NewTestsWorker) analyzeRisks(logger *logrus.Entry, jobs []prJobInfo) []*JobNewTestRisks {
	logger = logger.WithField("func", "analyzeRisks")
	jobRisks := []*JobNewTestRisks{}
	for _, jobInfo := range jobs {
		latestRuns := ntw.jobRunFilter.OnlyLatestSha(logger, jobInfo)
		if latestRuns == nil {
			logger.Infof(
				"Skipping new test analysis for job %s as there are no completed runs against the PR's shasum %s",
				jobInfo.name, jobInfo.prShaSum)
			continue
		}

		risks := ntw.assessJobRisks(logger, latestRuns)
		if risks != nil { // there were runs to analyze and check for new tests
			jobRisks = append(jobRisks, &JobNewTestRisks{JobName: jobInfo.name, NewTestRisks: risks})
		} // else all runs were excluded; do not use this job's lack of new tests for comparison
	}

	if len(jobRisks) > 0 {
		// look across the PR's jobs and upgrade risks for new tests that are only found in one job.
		// a new test that is only seen in one job is a risk similar to one not seen across all runs.
		assessCrossJobRisks(jobRisks, jobs)
		// and finally, assign risk levels given everything we know about the new tests
		assignRiskLevels(jobRisks)
	}

	return jobRisks
}

// look across the PR's jobs and upgrade risks for new tests that are only found in one job
func assessCrossJobRisks(jobRisks []*JobNewTestRisks, jobs []prJobInfo) {
	if len(jobs) < 2 {
		return // we need at least two jobs to compare new tests
	}

	// first figure out how many jobs saw each new test
	newTestJobCount := make(map[string]int)
	for _, jobRisk := range jobRisks {
		for testName := range jobRisk.NewTestRisks {
			newTestJobCount[testName]++
		}
	}

	// upgrade risk of any new test that is unique to one job
	for _, jobRisk := range jobRisks {
		for testName, risk := range jobRisk.NewTestRisks {
			if newTestJobCount[testName] == 1 {
				risk.OnlyInOne = true
			}
		}
	}
}

// update the risk record of each new test, assigning a risk level and reason based on risk factors seen
func assignRiskLevels(jobRisks []*JobNewTestRisks) {
	for _, jobRisk := range jobRisks {
		for _, risk := range jobRisk.NewTestRisks {
			if risk.AnyMissing {
				// 1. Any PR job adds a new test that appears in one run and not another at the same sha - high risk
				risk.Level = apiModels.FailureRiskLevelHigh
				risk.Reason = "is a new test that was not present in all runs against the current commit."
				if risk.Failures > 0 {
					risk.Reason = fmt.Sprintf("is a new test that was not present in all runs against the current commit, and also failed %d time(s).", risk.Failures)
				}
			} else if risk.OnlyInOne {
				// 2. PR adds new test that appears in only a single job and:
				if risk.Failures > 0 {
					//   - it fails at all - high risk
					risk.Level = apiModels.FailureRiskLevelHigh
					risk.Reason = fmt.Sprintf("is a new test, was only seen in one job, and failed %d time(s) against the current commit.", risk.Failures)
				} else {
					//   - it succeeds or flakes - medium risk (might not be intended for multiple jobs)
					risk.Level = apiModels.FailureRiskLevelMedium
					risk.Reason = "is a new test, and was only seen in one job."
				}
			} else {
				// 3. PR adds new test that appears in more than one job and (at latest sha):
				if risk.Failures > 0 {
					//   - it fails at all - high risk
					risk.Level = apiModels.FailureRiskLevelHigh
					risk.Reason = fmt.Sprintf("is a new test that failed %d time(s) against the current commit", risk.Failures)
				} else {
					//   - it succeeds or flakes - no risk (only included in list of all new tests)
					risk.Level = apiModels.FailureRiskLevelNone
				}
			}
		}
	}
}

// OnlyLatestSha allows only runs against the PR's current shasum (do not flag new tests from earlier shasums)
func (jrf *pgJobRunFilter) OnlyLatestSha(logger *logrus.Entry, jobInfo prJobInfo) []*prow.ProwJob {
	logger = logger.WithField("func", "OnlyLatestSha").WithField("job", jobInfo.name).WithField("sha", jobInfo.prShaSum)
	var latestRuns []*prow.ProwJob
	for idx, run := range jobInfo.prowJobRuns {
		if sha := run.Spec.Refs.Pulls[0].SHA; sha != jobInfo.prShaSum {
			logger.Debugf("Excluding run %s against sha %s", run.Status.BuildID, sha)
			continue
		}
		logger.Debugf("Including run %s", run.Status.BuildID)
		latestRuns = append(latestRuns, jobInfo.prowJobRuns[idx])
	}
	return latestRuns
}

// JobFailedEarly filters out runs with significantly fewer tests than usual;
// when looking for new tests, runs that broke before running all the tests will muddy the analysis.
// such runs should be left to risk analysis for comment.
// if any errors occur, return true so the run is not included in the new test analysis.
func (jrf *pgJobRunFilter) JobFailedEarly(logger *logrus.Entry, run *models.ProwJobRun) bool {
	logger = logger.WithField("func", "JobFailedEarly").WithField("job", run.ProwJob.Name).WithField("run", run.ID)
	if run.TestCount <= 0 { // this can in theory happen when building the run, filter these out
		logger.Warn("Failed to count tests earlier, ignoring this run")
		return true
	}
	// figure out how many runs this job usually has
	historicalCount, ok := jrf.historicalTestCount[run.ProwJob.ID]
	if !ok {
		var err error
		historicalCount, err = query.ProwJobHistoricalTestCounts(jrf.dbc, run.ProwJobID)
		if err != nil {
			logger.WithError(err).Error("Error determining historical job test count, ignoring this run")
			return true
		}
		jrf.historicalTestCount[run.ProwJob.ID] = historicalCount
	}

	if run.TestCount*100 < historicalCount*75 {
		logger.Infof("Much fewer tests ran (%d) than historically (%d), ignoring this run", run.TestCount, historicalCount)
		return true
	}
	return false
}

// assessJobRisks walks the runs for a job looking for new tests and assessing their risk.
// returns map of risks by test name. this will likely be empty, meaning no risks were found.
// if it is nil, all job runs were excluded, so this job should not be included in the analysis.
func (ntw *NewTestsWorker) assessJobRisks(logger *logrus.Entry, jobRuns []*prow.ProwJob) map[string]*NewTestRisk {
	logger = logger.WithField("func", "assessJobRisks").WithField("runs", len(jobRuns))

	// find the new tests in all the comparable runs we have for one job
	newTestsByName := map[string][]NewTest{}
	sawValidRun := false // track whether any runs were not excluded
	for _, run := range jobRuns {
		logger.Infof("Finding new tests for job %s run %s", run.Spec.Job, run.Status.BuildID)
		if newTests, err := ntw.getNewTestsForJobRun(logger, run); err == nil {
			sawValidRun = true
			for _, test := range newTests {
				newTestsByName[test.TestName] = append(newTestsByName[test.TestName], test)
			}
		}
	}

	if !sawValidRun {
		return nil // no runs to analyze, do not consider this job in the analysis
	}

	// evaluate this job's run(s) of each new test for risk
	risksByName := map[string]*NewTestRisk{}
	for testName, tests := range newTestsByName {
		risksByName[testName] = makeNewTestRisk(testName, len(jobRuns), tests)
	}
	// later, we can further compare new tests across multiple jobs, for the same PR.
	return risksByName
}

// makeNewTestRisk builds the risk record of a new test based on multiple runs of one job
func makeNewTestRisk(testName string, jobRuns int, tests []NewTest) *NewTestRisk {
	// new tests in general are a low risk if they succeed, mainly we want to record their existence
	risk := NewTestRisk{
		TestName:   testName,
		AnyMissing: false,
		Runs:       jobRuns,
		NewTests:   tests,
	}

	// new tests that fail in any runs are a high risk
	for _, test := range tests {
		if test.Failure && test.Success {
			// sometimes new tests are deliberately introduced to flake;
			// count these for informational purposes but do not consider them an extra risk
			risk.Flakes++
		} else if test.Failure {
			risk.Failures++
		}
	}

	// with multiple runs, check whether new tests also showed up in all runs;
	// if not, they likely either have dynamic names or do not consistently run, either of which is a risk
	if len(tests) < jobRuns {
		risk.AnyMissing = true
	}
	return &risk
}

// getNewTestsForJobRun builds sippy's model of a job run and searches it for new tests.
func (ntw *NewTestsWorker) getNewTestsForJobRun(logger *logrus.Entry, prowjob *prow.ProwJob) (newTests []NewTest, err error) {
	logger = logger.WithField("func", "getNewTestsForJobRun").WithField("job", prowjob.Spec.Job).WithField("run", prowjob.Status.BuildID)
	var jobRun *models.ProwJobRun
	if jobRunIntID, err := strconv.ParseInt(prowjob.Status.BuildID, 10, 64); err != nil {
		logger.WithError(err).Error("Failed to parse jobRunId id") // this would be exceedingly strange
		return nil, err
	} else if jobRun, err = ntw.fetchJobRun(ntw.dbc, jobRunIntID, true, nil, logger); err != nil {
		// RecordNotFound can be expected if the jobRunId job isn't in sippy yet. log any other error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Debug("Job run not found")
		} else {
			logger.WithError(err).Error("Error fetching job run")
		}
		return nil, err
	} else if ntw.jobRunFilter.JobFailedEarly(logger, jobRun) {
		return nil, errors.New("job run failed early, ignore")
	}

	for _, test := range jobRun.Tests {
		test.ProwJobRun = *jobRun // sometimes handy for a test to know whence it came
		if isNew, err := ntw.newTestFilter.IsNewTest(logger, test); err != nil {
			logger.WithError(err).Error("Error checking if test is new")
			return nil, err // if this errors, it muddies this job's analysis, so throw it out
		} else if isNew {
			newTests = append(newTests, NewTest{
				JobName:  prowjob.Spec.Job,
				JobRunID: jobRun.ID,
				TestName: test.Test.Name,
				Success:  test.Status == int(spv1.TestStatusSuccess) || test.Status == int(spv1.TestStatusFlake),
				Failure:  test.Status == int(spv1.TestStatusFailure) || test.Status == int(spv1.TestStatusFlake),
			})
		}
	}
	return newTests, nil
}

/*
IsNewTest queries postgres to determine if a test not registered in `test_ownerships`
is in fact new. For various $reasons, not all tests that we import in sippy are registered
in that table, so we need additional verification to prevent flagging the same test as
"new" over and over again.

The search strategy is to look for instances of the test that ran against
PRs that merged before the test under consideration began. If there are any,
we can cache that test name as not new. If there are none, then consider this
a new test.
Records for PRs and potential PR comments are both created/updated at the same time,
so this should be a reasonably robust strategy, though not infallible.
*/
func (ntf *pgNewTestFilter) IsNewTest(logger *logrus.Entry, testRun models.ProwJobRunTest) (bool, error) {
	logger = logger.WithField("func", "IsNewTest").WithField("test", testRun.Test.Name)
	ntf.nnTmutex.Lock()
	if ntf.notNewTests.Has(testRun.TestID) {
		// some past query found a PR that merged with this test.
		logger.Debug("Test previously cached as not new")
		ntf.nnTmutex.Unlock()
		return false, nil
	}
	ntf.nnTmutex.Unlock()

	pjpr := models.ProwPullRequest{}
	res := ntf.dbc.DB.
		Table("prow_job_run_tests as t").
		Joins("INNER JOIN prow_job_run_prow_pull_requests as prmap on prmap.prow_job_run_id = t.prow_job_run_id").
		Joins("INNER JOIN prow_pull_requests as prs on prs.id = prmap.prow_pull_request_id").
		Where("t.test_id = ?", testRun.TestID).
		Where("merged_at is not null").
		Select("org, repo, number, sha, merged_at").
		Limit(1).Find(&pjpr) // any result demonstrates this is not new
	if res.Error != nil {
		logger.WithError(res.Error).Error("Error querying for PRs that included this test.")
		return false, res.Error
	}
	if pjpr.MergedAt != nil {
		// means such a record was found, so this is not new
		logger.Debugf("Test ran in previously-merged PR %s/%s#%d@%s", pjpr.Org, pjpr.Repo, pjpr.Number, pjpr.SHA)
		ntf.nnTmutex.Lock()
		ntf.notNewTests.Insert(testRun.TestID) // do not need to look up next time
		ntf.nnTmutex.Unlock()
		return false, nil
	}
	// query succeeded but no such record was found, so this is new
	logger.Debug("Test has not run in any previously-merged PR, considering it new.")
	return true, nil
}

// summarizeNewTestRisks looks at all the risks spread across jobs and consolidates the stats;
// it also removes the "risks" that are None, as those are for new tests with no issues, leaving
// only the actual risks to report by job.
func summarizeNewTestRisks(jobRisks []*JobNewTestRisks) ([]*JobNewTestRisks, []NewTestRisk) {
	notableJobRisks := []*JobNewTestRisks{}          // filter out jobs with only "risks" that are None
	testSummariesByName := map[string]*NewTestRisk{} // sum up the runs we saw
	for _, jr := range jobRisks {
		actualRisks := map[string]*NewTestRisk{} // filter out the test "risks" that are None
		for name, risk := range jr.NewTestRisks {
			if risk.Level != apiModels.FailureRiskLevelNone {
				actualRisks[name] = risk
			}
			summary, exists := testSummariesByName[name]
			if !exists {
				summary = &NewTestRisk{TestName: name}
			}
			summary.Failures += risk.Failures
			summary.Flakes += risk.Flakes
			summary.Runs += risk.Runs
			testSummariesByName[name] = summary
		}
		if len(actualRisks) > 0 { // only note jobs including actual risks
			notableJobRisks = append(notableJobRisks, &JobNewTestRisks{
				JobName:      jr.JobName,
				NewTestRisks: actualRisks,
			})
		}
	}

	return notableJobRisks, sortedTestRisks(testSummariesByName)
}

func sortedTestRisks(testSummariesByName map[string]*NewTestRisk) []NewTestRisk {
	testSummaries := []NewTestRisk{}
	for _, summary := range testSummariesByName {
		testSummaries = append(testSummaries, *summary)
	}
	slices.SortFunc(testSummaries, func(a, b NewTestRisk) int {
		return strings.Compare(a.TestName, b.TestName)
	})
	return testSummaries
}
