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

type ProcessingOptions struct {
	StartDay int
	NumDays  int
}

// returns the raw data and a list of warnings encountered processing the data.
func (o ProcessingOptions) ProcessTestGridDataIntoRawJobResults(testGridJobInfo []testgridv1.JobDetails) (testgridanalysisapi.RawData, []string) {
	rawJobResults := testgridanalysisapi.RawData{JobResults: map[string]testgridanalysisapi.RawJobResult{}}

	for _, jobDetails := range testGridJobInfo {
		klog.V(2).Infof("processing test details for job %s\n", jobDetails.Name)
		startCol, endCol := computeLookback(o.StartDay, o.NumDays, jobDetails.Timestamps)
		processJobDetails(rawJobResults, jobDetails, startCol, endCol)
	}

	// now that we have all the JobRunResults, use them to create synthetic tests for install, upgrade, and infra
	warnings := createSyntheticTests(rawJobResults)

	return rawJobResults, warnings
}

func processJobDetails(rawJobResults testgridanalysisapi.RawData, job testgridv1.JobDetails, startCol, endCol int) {
	for i, test := range job.Tests {
		klog.V(4).Infof("Analyzing results from %d to %d from job %s for test %s\n", startCol, endCol, job.Name, test.Name)

		test.Name = strings.TrimSpace(tagStripRegex.ReplaceAllString(test.Name, ""))
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

// tagStripRegex removes test markers deemed unhelpful at one point in time.
// TODO relitigate the value of doing this.  Without these markers, I don't think it is possible to run the failing test back through `openshift-tests run-test <foo>`
var tagStripRegex = regexp.MustCompile(`\[Skipped:.*?\]|\[Suite:.*\]`)

// ignoreTestRegex is used to strip o ut tests that don't have predictive or diagnostic value.  We don't want to show these in our data.
var ignoreTestRegex = regexp.MustCompile(`Run multi-stage test|operator.Import the release payload|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)

// processTestToJobRunResults adds the tests to the provided jobresult to the provided JobResult and returns the passed, failed, flaked for the test
func processTestToJobRunResults(jobResult testgridanalysisapi.RawJobResult, job testgridv1.JobDetails, test testgridv1.Test, startCol, endCol int) (passed int, failed int, flaked int) {
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
		case 1, 13: // success, flake(failed one or more times but ultimately succeeded)
			for i := col; i < col+remaining && i < endCol; i++ {
				passed++
				if result.Value == 13 {
					flaked++
				}
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunURL: joburl,
					}
				}
				switch {
				case test.Name == "Overall":
					jrr.Succeeded = true
				case testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(test.Name):
					matches := testgridanalysisapi.OperatorConditionsTestCaseName.FindStringSubmatch(test.Name)
					operatorIndex := testgridanalysisapi.OperatorConditionsTestCaseName.SubexpIndex("operator")
					jrr.SadOperators = append(jrr.SadOperators, testgridanalysisapi.OperatorState{
						Name:  matches[operatorIndex],
						State: testgridanalysisapi.Success,
					})
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorUpgradePrefix):
					jrr.UpgradeOperators = append(jrr.UpgradeOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorUpgradePrefix):],
						State: testgridanalysisapi.Success,
					})
				case testidentification.IsSetupContainerEquivalent(test.Name):
					jrr.SetupStatus = testgridanalysisapi.Success
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		case 12: // failure
			for i := col; i < col+remaining && i < endCol; i++ {
				failed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunURL: joburl,
					}
				}
				// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
				// which we use to set various JobRunResult markers
				if test.Name != "Overall" && !testidentification.IsSetupContainerEquivalent(test.Name) {
					jrr.FailedTestNames = append(jrr.FailedTestNames, test.Name)
					jrr.TestFailures++
				}

				switch {
				case test.Name == "Overall":
					jrr.Failed = true
				case testgridanalysisapi.OperatorConditionsTestCaseName.MatchString(test.Name):
					matches := testgridanalysisapi.OperatorConditionsTestCaseName.FindStringSubmatch(test.Name)
					operatorIndex := testgridanalysisapi.OperatorConditionsTestCaseName.SubexpIndex("operator")
					jrr.SadOperators = append(jrr.SadOperators, testgridanalysisapi.OperatorState{
						Name:  matches[operatorIndex],
						State: testgridanalysisapi.Failure,
					})
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorUpgradePrefix):
					jrr.UpgradeOperators = append(jrr.UpgradeOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorUpgradePrefix):],
						State: testgridanalysisapi.Failure,
					})
				case testidentification.IsSetupContainerEquivalent(test.Name):
					jrr.SetupStatus = testgridanalysisapi.Failure
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		}
		col += remaining
	}

	addTestResult(jobResult.TestResults, test.Name, passed, failed, flaked)

	return
}

func processTest(rawJobResults testgridanalysisapi.RawData, job testgridv1.JobDetails, test testgridv1.Test, startCol, endCol int) {
	// strip out tests that don't have predictive or diagnostic value
	// we have to know about overall to be able to set the global success or failure.
	// we have to know about container setup to be able to set infra failures
	// TODO stop doing this so we can avoid any filtering. We can filter when preparing to create the data for display
	if test.Name != "Overall" && !testidentification.IsSetupContainerEquivalent(test.Name) && ignoreTestRegex.MatchString(test.Name) {
		return
	}

	jobResult, ok := rawJobResults.JobResults[job.Name]
	if !ok {
		jobResult = testgridanalysisapi.RawJobResult{
			JobName:        job.Name,
			TestGridJobUrl: job.TestGridUrl,
			JobRunResults:  map[string]testgridanalysisapi.RawJobRunResult{},
			TestResults:    map[string]testgridanalysisapi.RawTestResult{},
		}
	}

	processTestToJobRunResults(jobResult, job, test, startCol, endCol)

	// we have mutated, so assign back to our intermediate value
	rawJobResults.JobResults[job.Name] = jobResult
}

func addTestResult(testResults map[string]testgridanalysisapi.RawTestResult, testName string, passed, failed, flaked int) {
	result, ok := testResults[testName]
	if !ok {
		result = testgridanalysisapi.RawTestResult{}
	}
	result.Name = testName
	result.Successes += passed
	result.Failures += failed
	result.Flakes += flaked

	testResults[testName] = result
}
