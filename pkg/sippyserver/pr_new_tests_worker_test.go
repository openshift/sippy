package sippyserver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/prow"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func TestBuildJobMap(t *testing.T) {
	// initialize AnalysisWorker
	gcsBucket := util.GetGcsBucket(t)
	aw := AnalysisWorker{gcsBucket: gcsBucket}
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithContext(context.TODO())
	runs := aw.buildProwJobRuns(logger, "pr-logs/pull/29501/pull-ci-openshift-origin-master-e2e-aws-ovn-edge-zones/")
	if assert.Greater(t, len(runs), 1, "expect multiple job runs") {
		cmpTime := time.Now() // expect these to be sorted by decreasing start time
		for _, run := range runs {
			nextTime := run.Status.StartTime
			assert.Truef(t, nextTime.Before(cmpTime), "expect %s start time %v to be before %v", run.Status.BuildID, nextTime, cmpTime)
			cmpTime = nextTime
			assert.Equal(t, "pull-ci-openshift-origin-master-e2e-aws-ovn-edge-zones", run.Spec.Job)
		}
	}
}

func TestAssessJobRisks(t *testing.T) {
	logger := logrus.WithContext(context.TODO())
	logrus.SetLevel(logrus.InfoLevel)

	ntw := StandardNewTestsWorker(util.GetDbHandle(t))

	// Initialize GCS client and look up known job in the bucket
	bucket := util.GetGcsBucket(t)
	aw := AnalysisWorker{gcsBucket: bucket, newTestsWorker: ntw}
	jobRuns := aw.buildProwJobRuns(logger, "pr-logs/pull/29512/pull-ci-openshift-origin-master-e2e-aws-ovn-single-node/")
	numRuns := len(jobRuns)
	if !assert.True(t, numRuns > 0, "Failed to load job runs") {
		return
	}
	if !assert.Equal(t, "1885131315280351232", jobRuns[numRuns-1].Status.BuildID, "Unexpected build ID") {
		return // expected to use the earliest job run (last in list) as a test subject
	}

	// Assess single job risks
	jobRisks := ntw.assessJobRisks(logger, []*prow.ProwJob{jobRuns[numRuns-1]})
	if !assert.Equalf(t, numRuns, 2, "expect risks only for the two that were new; saw %+v", jobRisks) {
		return
	}
	risks := []*JobNewTestRisks{{JobName: "some-job", NewTestRisks: jobRisks}}
	assignRiskLevels(risks)
	failed, ok := jobRisks["a failed test that has never been seen before"]
	if assert.True(t, ok, "Should have found failed test") {
		assert.Equal(t, 1, failed.Failures, "Unexpected number of failures")
		assert.Equal(t, api.FailureRiskLevelHigh, failed.Level, "Expected high risk for failing test")
	}
	passed, ok := jobRisks["a passed test that has never been seen before"]
	if assert.True(t, ok, "Should have found failed test") {
		assert.Equal(t, 0, passed.Failures, "Unexpected failure found")
		assert.Equal(t, api.FailureRiskLevelNone, passed.Level, "Expected no risk for passing test")
	}

	// Assess multi run risks where new tests go missing
	if !assert.True(t, numRuns > 1, "Expected at least 2 job runs") {
		return
	}
	// skippingNewTestFilter alternately filters out new tests found to simulate missing tests
	ntw.newTestFilter = &skippingNewTestFilter{newTestFilter: ntw.newTestFilter, sawPrevious: map[string]bool{}}
	jobRisks = ntw.assessJobRisks(logger, jobRuns)
	failed, ok = jobRisks["a failed test that has never been seen before"]
	if assert.True(t, ok, "Should have found failed test") {
		if !assert.True(t, failed.AnyMissing, "Expected test missing in at least one run") {
			return
		}
	}
	risks = []*JobNewTestRisks{{JobName: "some-job", NewTestRisks: jobRisks}}
	assignRiskLevels(risks)
	failed, ok = risks[0].NewTestRisks["a failed test that has never been seen before"]
	if assert.True(t, ok, "Should have found failed test") {
		assert.Equal(t, api.FailureRiskLevelHigh, failed.Level, "Expected high risk for failing intermittent test")
	}
	passed, ok = risks[0].NewTestRisks["a passed test that has never been seen before"]
	if assert.True(t, ok, "Should have found passed test") {
		assert.Equal(t, api.FailureRiskLevelHigh, passed.Level, "Expected high risk for passing intermittent test")
		assert.NotContains(t, passed.Reason, "failed", "Risk reason is not due to failure")
	}
}

func TestAssessCrossJobRisks(t *testing.T) {
	// setting up unit tests for this would be atrocious, but a functional test
	// for ntw.analyzeRisks runs assessCrossJobRisks() so just use real data and tricks to test
	logger := logrus.WithContext(context.TODO())
	logrus.SetLevel(logrus.InfoLevel)

	// look up test-bed PR jobs in the bucket
	ntw := StandardNewTestsWorker(util.GetDbHandle(t))
	aw := AnalysisWorker{gcsBucket: util.GetGcsBucket(t), newTestsWorker: ntw}
	completedJobs := aw.getPrJobsIfFinished(logger, "pr-logs/pull/29512/")
	if !assert.Greater(t, len(completedJobs), 2, "Failed to load all job runs") {
		return
	}

	// run just the new test analysis, but only find new tests in one job
	ntw.newTestFilter = &oneJobNewTestFilter{newTestFilter: ntw.newTestFilter, jobName: "pull-ci-openshift-origin-master-e2e-aws-ovn-single-node"}
	for idx, jobInfo := range completedJobs {
		completedJobs[idx].prowJobRuns = aw.buildProwJobRuns(logger, jobInfo.bucketPrefix)
		completedJobs[idx].prShaSum = "8849ed78d4c51e2add729a68a2cbf8551c6d60c9" // so we can check whether runs are against the expected PR commit
	}
	risks := ntw.analyzeRisks(logger, completedJobs)
	if !assert.Greater(t, len(risks), 1, "Expected a risk each for the two new tests") {
		return
	}

	// check that risks match expectations
	var sawPassedTest, sawFailedTest bool
	for _, jobRisk := range risks {
		if jobRisk.JobName == "pull-ci-openshift-origin-master-e2e-vsphere-ovn-upi" {
			// this is a test for JobFailedEarly as well...
			assert.Fail(t, "JobFailedEarly should have filtered out this job's broken run")
		}
		for _, testRisk := range jobRisk.NewTestRisks {
			fmt.Printf("risk: %q: %+v\n", jobRisk.JobName, *testRisk)
			switch testRisk.TestName {
			case "a failed test that has never been seen before":
				assert.True(t, testRisk.OnlyInOne, "Expected test to only be seen in one job")
				assert.Equal(t, api.FailureRiskLevelHigh, testRisk.Level,
					"Expected high risk for failing test seen in one job")
				sawFailedTest = true
			case "a passed test that has never been seen before":
				assert.True(t, testRisk.OnlyInOne, "Expected test to only be seen in one job")
				assert.Equal(t, api.FailureRiskLevelMedium, testRisk.Level,
					"Expected medium risk for passing test seen in one job")
				sawPassedTest = true
			default:
				t.Errorf("Did not expect to see new test %q", testRisk.TestName)
			}
		}
	}
	assert.True(t, sawPassedTest, "Should have found risk for passed test")
	assert.True(t, sawFailedTest, "Should have found risk for failed test")
}

func newTest(name string, success, failure bool) NewTest {
	return NewTest{
		TestName: name,
		Success:  success,
		Failure:  failure,
	}
}

func TestRiskScenarios(t *testing.T) {
	cases := []struct {
		name     string
		tests    []NewTest
		expected NewTestRisk
	}{ // all assume two job runs
		{
			name: "AllTestsPassing",
			tests: []NewTest{
				newTest("test", true, false),
				newTest("test", true, false),
			},
			expected: NewTestRisk{
				Failures:   0,
				Flakes:     0,
				AnyMissing: false,
			},
		},
		{
			name: "SomeTestsFailing",
			tests: []NewTest{
				newTest("something", true, false),
				newTest("something", false, true),
			},
			expected: NewTestRisk{
				Failures:   1,
				Flakes:     0,
				AnyMissing: false,
			},
		},
		{
			name: "FlakyTest and MissingTest",
			tests: []NewTest{
				newTest("test", true, true),
			},
			expected: NewTestRisk{
				Failures:   0,
				Flakes:     1,
				AnyMissing: true,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			risk := makeNewTestRisk("test", 2, c.tests)
			assert.Equal(t, c.expected.Failures, risk.Failures)
			assert.Equal(t, c.expected.Flakes, risk.Flakes)
			assert.Equal(t, c.expected.AnyMissing, risk.AnyMissing)
		})
	}
}

func TestUnit_getNewTestsForJobRun(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	jobRun := &prow.ProwJob{
		Spec:   prow.ProwJobSpec{Job: "test-jobRun"},
		Status: prow.ProwJobStatus{BuildID: "12345"},
	}
	tests := []struct {
		name          string
		fetchJobRun   func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error)
		testFilter    NewTestFilter
		expectedTests []NewTest
		expectedError error
	}{
		{
			name: "successful fetch",
			fetchJobRun: func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error) {
				pjr := models.ProwJobRun{
					Tests: []models.ProwJobRunTest{
						{Test: models.Test{Name: "test1"}, Status: int(v1.TestStatusSuccess)},
						{Test: models.Test{Name: "test2"}, Status: int(v1.TestStatusFailure)},
						{Test: models.Test{Name: "test3"}, Status: int(v1.TestStatusFlake)},
					},
				}
				pjr.ID = 12345 // Gorm model ID for some reason can't be put in the struct literal
				return &pjr, nil
			},
			testFilter: &oneNewTestFilter{}, // filters to only "test2"
			expectedTests: []NewTest{
				{JobName: "test-jobRun", JobRunID: 12345, TestName: "test2", Success: false, Failure: true},
			},
			expectedError: nil,
		},
		{
			name: "error on filtering",
			fetchJobRun: func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error) {
				pjr := models.ProwJobRun{
					Tests: []models.ProwJobRunTest{
						{Test: models.Test{Name: "test1"}, Status: int(v1.TestStatusSuccess)},
					},
				}
				pjr.ID = 12345 // Gorm model ID for some reason can't be put in the struct literal
				return &pjr, nil
			},
			testFilter:    &errorNewTestFilter{}, // mocks a failure in the filter
			expectedTests: nil,
			expectedError: errors.New("filter error"),
		},
		{
			name: "jobRun run not found",
			fetchJobRun: func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error) {
				return nil, gorm.ErrRecordNotFound
			},
			expectedTests: nil,
			expectedError: gorm.ErrRecordNotFound,
		},
		{
			name: "error fetching jobRun run",
			fetchJobRun: func(dbc *db.DB, jobRunID int64, unknownTests bool, preloads []string, logger *logrus.Entry) (*models.ProwJobRun, error) {
				return nil, errors.New("fetch error")
			},
			expectedTests: nil,
			expectedError: errors.New("fetch error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ntw := &NewTestsWorker{
				dbc:           nil,
				newTestFilter: tt.testFilter,
				fetchJobRun:   tt.fetchJobRun,
				jobRunFilter:  &jobRunUnfiltered{},
			}
			newTests, err := ntw.getNewTestsForJobRun(logger, jobRun)
			assert.Equal(t, tt.expectedTests, newTests)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

type oneNewTestFilter struct{}
type errorNewTestFilter struct{}
type skippingNewTestFilter struct { // alternate skipping or including actual new tests
	newTestFilter NewTestFilter
	sawPrevious   map[string]bool
}
type oneJobNewTestFilter struct { // only return new tests against one job
	newTestFilter NewTestFilter
	jobName       string
}

func (m *oneNewTestFilter) IsNewTest(_ *logrus.Entry, test models.ProwJobRunTest) (bool, error) {
	if test.Test.Name == "test2" {
		return true, nil
	}
	return false, nil
}
func (m *errorNewTestFilter) IsNewTest(_ *logrus.Entry, _ models.ProwJobRunTest) (bool, error) {
	return false, errors.New("filter error")
}

func (ntf *skippingNewTestFilter) IsNewTest(logger *logrus.Entry, test models.ProwJobRunTest) (bool, error) {
	if isNew, err := ntf.newTestFilter.IsNewTest(logger, test); err != nil {
		return false, err
	} else if isNew {
		ntf.sawPrevious[test.Test.Name] = !ntf.sawPrevious[test.Test.Name] // alternate skipping or including actual new tests
		if ntf.sawPrevious[test.Test.Name] {
			return true, nil
		}
	}
	return false, nil
}
func (ntf *oneJobNewTestFilter) IsNewTest(logger *logrus.Entry, test models.ProwJobRunTest) (bool, error) {

	if isNew, err := ntf.newTestFilter.IsNewTest(logger, test); err != nil {
		return false, err
	} else if isNew && ntf.jobName == test.ProwJobRun.ProwJob.Name {
		return true, nil
	}
	return false, nil
}

type jobRunUnfiltered struct{}

func (n *jobRunUnfiltered) OnlyLatestSha(_ *logrus.Entry, jobInfo prJobInfo) []*prow.ProwJob {
	return jobInfo.prowJobRuns
}

func (n *jobRunUnfiltered) JobFailedEarly(_ *logrus.Entry, _ *models.ProwJobRun) bool {
	return false
}

func TestFunc_getNewTestsForJobRun(t *testing.T) {
	ntf, ntw := internalNewTestsWorker(util.GetDbHandle(t))

	// try with a known job run
	jobRun := &prow.ProwJob{
		Spec:   prow.ProwJobSpec{Job: "pull-ci-openshift-origin-master-e2e-aws-ovn-single-node"},
		Status: prow.ProwJobStatus{BuildID: "1885131315280351232"},
	}
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithContext(context.TODO())
	newTests, err := ntw.getNewTestsForJobRun(logger, jobRun)

	fmt.Printf("new tests: %v\n", newTests)
	assert.NoError(t, err, "Failed to get new tests")
	assert.Equal(t, 2, len(newTests), "Unexpected number of new tests")
	assert.True(t, ntf.notNewTests.Has(522), "Test 522 should be considered not new")
	assert.False(t, ntf.notNewTests.Has(160471), "Test 160471 should be left out to be considered new")
	assert.False(t, ntf.notNewTests.Has(160472), "Test 160472 should be left out to be considered new")
}

func TestIsNewTest(t *testing.T) {
	dbc := util.GetDbHandle(t)

	ntf := &pgNewTestFilter{
		dbc:         dbc,
		notNewTests: sets.Set[uint]{},
		nnTmutex:    &sync.Mutex{},
	}
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithContext(context.TODO())

	test := models.Test{Name: "[sig-sippy] openshift-tests should work"}
	test.ID = 522
	testRun := models.ProwJobRunTest{
		Test:      test,
		TestID:    test.ID,
		CreatedAt: time.Now(),
	}
	isNew, err := ntf.IsNewTest(logger, testRun)

	assert.Nil(t, err, "Failed to check if test is new")
	assert.Equal(t, false, isNew, "Test should not be new")

	test.Name = "a failed test that has never been seen before"
	test.ID = 160471
	testRun.Test = test
	testRun.TestID = test.ID
	isNew, err = ntf.IsNewTest(logger, testRun)

	assert.Nil(t, err, "Failed to check if test is new")
	assert.Equal(t, true, isNew, "Test should be new")
}
