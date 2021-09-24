package testgridconversion

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"k8s.io/klog"
)

const overall string = "Overall"

type SyntheticTestManager interface {
	// CreateSyntheticTests takes the JobRunResult information and produces some pre-analysis by interpreting different types of failures
	// and potentially producing synthetic test results and aggregations to better inform sippy.
	// This needs to be called after all the JobDetails have been processed.
	// This method mutates the rawJobResults
	// returns warnings found in the data. Not failures to process it.
	CreateSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string
}

type ProcessingOptions struct {
	SyntheticTestManager SyntheticTestManager
	StartDay             int
	NumDays              int
}

// ProcessTestGridDataIntoRawJobResults returns the raw data and a list of warnings encountered processing the data.
func (o ProcessingOptions) ProcessTestGridDataIntoRawJobResults(testGridJobInfo []testgridv1.JobDetails) (testgridanalysisapi.RawData, []string) {
	rawJobResults := testgridanalysisapi.RawData{JobResults: map[string]testgridanalysisapi.RawJobResult{}}

	for _, jobDetails := range testGridJobInfo {
		klog.V(2).Infof("processing test details for job %s\n", jobDetails.Name)
		startCol, endCol := computeLookback(o.StartDay, o.NumDays, jobDetails.Timestamps)
		processJobDetails(rawJobResults, jobDetails, startCol, endCol)
	}

	// now that we have all the JobRunResults, use them to create synthetic tests for install, upgrade, and infra
	warnings := o.SyntheticTestManager.CreateSyntheticTests(rawJobResults)

	return rawJobResults, warnings
}

func processJobDetails(rawJobResults testgridanalysisapi.RawData, job testgridv1.JobDetails, startCol, endCol int) {
	for i, test := range job.Tests {
		klog.V(4).Infof("Analyzing results from %d to %d from job %s for test %s\n", startCol, endCol, job.Name, test.Name)
		for _, prefix := range testSuitePrefixes {
			test.Name = strings.TrimPrefix(test.Name, prefix)
		}
		job.Tests[i] = test
		processTest(rawJobResults, job, test, startCol, endCol)
	}
}

func computeLookback(startDay, numDays int, timestamps []int) (int, int) {
	stopTs := time.Now().Add(time.Duration(-1*(startDay+numDays)*24)*time.Hour).Unix() * 1000
	startTs := time.Now().Add(time.Duration(-1*startDay*24)*time.Hour).Unix() * 1000
	if startDay <= -1 { // find the most recent startTime
		mostRecentTimestamp := 0
		for _, t := range timestamps {
			if t > mostRecentTimestamp {
				mostRecentTimestamp = t
			}
		}
		// more negative numbers mean we have a further offset, so work that out
		startTs = int64(mostRecentTimestamp) + int64((startDay+1)*24*int(time.Hour.Seconds())*1000)
		stopTs = startTs - int64(numDays*24*int(time.Hour.Seconds())*1000)
	}

	klog.V(2).Infof("starttime: %d\nendtime: %d\n", startTs, stopTs)
	start := math.MaxInt32 // start is an int64 so leave overhead for wrapping to negative in case this gets incremented(it does).
	for i, t := range timestamps {
		if int64(t) < startTs && i < start {
			start = i
		}
		if int64(t) < stopTs {
			return start, i
		}
	}
	return start, len(timestamps)
}

// testSuitePrefixes is a list of suite prefixes to remove from test names
var testSuitePrefixes = []string{
	"openshift-tests.",
	"Cluster upgrade.",
	"Symptom detection.",
	"Operator results.",
	"OSD e2e suite.",
	"Log Metrics.",
}

// ignoreTestRegex is used to strip o ut tests that don't have predictive or diagnostic value.  We don't want to show these in our data.
var ignoreTestRegex = regexp.MustCompile(`Run multi-stage test|operator.Import the release payload|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)

// processTestToJobRunResults adds the tests to the provided jobresult to the provided JobResult and returns the passed, failed, flaked for the test
//nolint:gocyclo // TODO: Break this function up, see: https://github.com/fzipp/gocyclo
func processTestToJobRunResults(jobResult testgridanalysisapi.RawJobResult, job testgridv1.JobDetails, test testgridv1.Test, startCol, endCol int) (passed, failed, flaked int) {
	col := 0
	for _, result := range test.Statuses {
		if col > endCol {
			break
		}

		// the test results are run length encoded(e.g. "6 passes, 5 failures, 7 passes"), but since we are searching for a test result
		// from a specific time period, it's possible a particular run of results overlaps the start-point
		// for the time period we care about.  So we need to iterate each encoded run until we get to the column
		// we care about(a column which falls within the timestamp range we care about, then start the analysis with the remaining
		// columns in the run.
		remaining := result.Count
		if col < startCol {
			for i := 0; i < result.Count && col < startCol; i++ {
				col++
				remaining--
			}
		}
		// if after iterating above we still aren't within the column range we care about, don't do any analysis
		// on this run of results.
		if col < startCol {
			continue
		}
		switch result.Value {
		case testgridv1.TestStatusSuccess, testgridv1.TestStatusFlake: // success, flake(failed one or more times but ultimately succeeded)
			for i := col; i < col+remaining && i < endCol; i++ {
				passed++
				if result.Value == testgridv1.TestStatusFlake {
					flaked++
				}
				joburl := fmt.Sprintf("https://prow.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunID:  job.ChangeLists[i],
						JobRunURL: joburl,
						Timestamp: job.Timestamps[i],
					}
				}
				switch {
				case test.Name == overall:
					jrr.Succeeded = true
					// if the overall job succeeded, setup is always considered successful, even for jobs
					// that don't have an explicitly defined setup test.
					jrr.SetupStatus = testgridanalysisapi.Success
				case testidentification.IsOperatorHealthTest(test.Name):
					jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, testgridanalysisapi.OperatorState{
						Name:  testidentification.GetOperatorNameFromTest(test.Name),
						State: testgridanalysisapi.Success,
					})
				case testidentification.IsSetupContainerEquivalent(test.Name):
					jrr.SetupStatus = testgridanalysisapi.Success
				case testidentification.IsUpgradeStartedTest(test.Name):
					jrr.UpgradeStarted = true
				case testidentification.IsOperatorsUpgradedTest(test.Name):
					jrr.UpgradeForOperatorsStatus = testgridanalysisapi.Success
				case testidentification.IsMachineConfigPoolsUpgradedTest(test.Name):
					jrr.UpgradeForMachineConfigPoolsStatus = testgridanalysisapi.Success
				case testidentification.IsOpenShiftTest(test.Name):
					// If there is a failed test, the aggregated value should stay "Failure"
					if jrr.OpenShiftTestsStatus == "" {
						jrr.OpenShiftTestsStatus = testgridanalysisapi.Success
					}
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		case testgridv1.TestStatusFailure:
			for i := col; i < col+remaining && i < endCol; i++ {
				failed++
				joburl := fmt.Sprintf("https://prow.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunURL: joburl,
						Timestamp: job.Timestamps[i],
					}
				}
				// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
				// which we use to set various JobRunResult markers
				if test.Name != overall && !testidentification.IsSetupContainerEquivalent(test.Name) {
					jrr.FailedTestNames = append(jrr.FailedTestNames, test.Name)
					jrr.TestFailures++
				}

				switch {
				case test.Name == overall:
					jrr.Failed = true
				case testidentification.IsOperatorHealthTest(test.Name):
					jrr.FinalOperatorStates = append(jrr.FinalOperatorStates, testgridanalysisapi.OperatorState{
						Name:  testidentification.GetOperatorNameFromTest(test.Name),
						State: testgridanalysisapi.Failure,
					})
				case testidentification.IsSetupContainerEquivalent(test.Name):
					jrr.SetupStatus = testgridanalysisapi.Failure
				case testidentification.IsUpgradeStartedTest(test.Name):
					jrr.UpgradeStarted = true // this is still true because we definitely started
				case testidentification.IsOperatorsUpgradedTest(test.Name):
					jrr.UpgradeForOperatorsStatus = testgridanalysisapi.Failure
				case testidentification.IsMachineConfigPoolsUpgradedTest(test.Name):
					jrr.UpgradeForMachineConfigPoolsStatus = testgridanalysisapi.Failure
				case testidentification.IsOpenShiftTest(test.Name):
					jrr.OpenShiftTestsStatus = testgridanalysisapi.Failure
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		}
		col += remaining
	}

	// don't add results for tests that did not run
	if passed+failed+flaked == 0 {
		return
	}

	// we override some test names based on their type.  Historically we misnamed the install and upgrade tests
	// what we really want is to call these the final state
	// This prevents any real results with this junit from counting.  This should only be needed during our transition  and
	// we have to keep it to interpret historical results from 4.6.
	testName := test.Name
	if testidentification.IsOldInstallOperatorTest(test.Name) || testidentification.IsOldUpgradeOperatorTest(test.Name) {
		operatorName := testidentification.GetOperatorNameFromTest(test.Name)
		testName = testgridanalysisapi.OperatorFinalHealthPrefix + " " + operatorName
	}

	addTestResult(jobResult.TestResults, &job, testName, passed, failed, flaked)

	return
}

func processTest(rawJobResults testgridanalysisapi.RawData, job testgridv1.JobDetails, test testgridv1.Test, startCol, endCol int) {
	// strip out tests that don't have predictive or diagnostic value
	// we have to know about overall to be able to set the global success or failure.
	// we have to know about container setup to be able to set infra failures
	// TODO stop doing this so we can avoid any filtering. We can filter when preparing to create the data for display
	if test.Name != overall && !testidentification.IsSetupContainerEquivalent(test.Name) && ignoreTestRegex.MatchString(test.Name) {
		return
	}

	jobResult, ok := rawJobResults.JobResults[job.Name]
	if !ok {
		jobResult = testgridanalysisapi.RawJobResult{
			JobName:        job.Name,
			TestGridJobURL: job.TestGridURL,
			JobRunResults:  map[string]testgridanalysisapi.RawJobRunResult{},
			TestResults:    map[string]testgridanalysisapi.RawTestResult{},
		}
	}

	processTestToJobRunResults(jobResult, job, test, startCol, endCol)

	// we have mutated, so assign back to our intermediate value
	rawJobResults.JobResults[job.Name] = jobResult
}

func addTestResult(testResults map[string]testgridanalysisapi.RawTestResult, job *testgridv1.JobDetails, testName string, passed, failed, flaked int) {
	result, ok := testResults[testName]
	if !ok {
		result = testgridanalysisapi.RawTestResult{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Flakes += flaked

	if job != nil {
		result.Timestamps = append(result.Timestamps, job.Timestamps...)
	}

	testResults[testName] = result
}
