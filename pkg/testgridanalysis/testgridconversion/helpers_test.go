package testgridconversion_test

import (
	"fmt"
	"testing"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/util/sets"
)

// Assertions
func assertNoWarnings(t *testing.T, warnings []string) {
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func assertWarningsEqual(t *testing.T, have, want []string) {
	haveSet := sets.NewString(have...)
	wantSet := sets.NewString(want...)

	if !haveSet.Equal(wantSet) {
		t.Errorf("expected to find warnings %v, got: %v", wantSet.List(), haveSet.List())
	}
}

func assertRawTestResultsNamesEqual(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expectedTestNames []string) {
	wantSet := sets.NewString(expectedTestNames...)
	haveSet := sets.StringKeySet(rawJobResult.TestResults)

	if !wantSet.Equal(haveSet) {
		t.Errorf("raw test result names not equal: have: %v, want: %v", haveSet.List(), wantSet.List())
	}
}

func assertHasRawTestResults(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, testNames []string) {
	for _, testName := range testNames {
		if _, ok := rawJobResult.TestResults[testName]; !ok {
			t.Errorf("expected to find raw test result with name: %s", testName)
		}
	}
}

func assertNotHasRawTestResults(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, testNames []string) {
	for _, testName := range testNames {
		if _, ok := rawJobResult.TestResults[testName]; ok {
			t.Errorf("expected to not to find raw test result with name: %s", testName)
		}
	}
}

func assertTestGridJobDetailsOK(t *testing.T, jobDetails []testgridv1.JobDetails, jobNum int) {
	// Do some validating of our generated input.
	// Specifically, we're ensuring that the following is true:
	// - We have an equal number of timestamps and changelists.
	// - The test status counts for all tests are equal to one another and to the
	// number of timestamps and changelists we have.
	// - The number of jobs (jobNum) is equal to the number of changelists,
	// timestamps, test counts, etc.
	// I'm sure there are additional edge-cases which these expectations may not
	// be useful for, however this helped with my understanding of how the
	// testgridconversion code parses testgrid output.
	testStatusCount := getTestGridTestStatusCount(jobDetails[0].Tests[0].Statuses)

	for _, test := range jobDetails[0].Tests {
		count := getTestGridTestStatusCount(test.Statuses)
		if count != testStatusCount {
			t.Errorf("test %s has status count %d, expected: %d", test.Name, count, testStatusCount)
		}
	}

	if testStatusCount != jobNum {
		t.Errorf("expected test status count (%d) to equal number of jobs (%d)", testStatusCount, jobNum)
	}

	changelistLen := len(jobDetails[0].ChangeLists)
	timestampLen := len(jobDetails[0].Timestamps)
	if changelistLen != timestampLen {
		t.Errorf("expected changelist (%d) and timestamp (%d) lengths to be equal", changelistLen, timestampLen)
	}

	if changelistLen != testStatusCount {
		t.Errorf("expected changelist (%d) length to equal test status count (%d)", changelistLen, testStatusCount)
	}
}

// Input generation funcs
func getTestGridJobDetailsFromTestNames(testNames []string, status testgridv1.TestStatus) []testgridv1.JobDetails {
	return getTestGridJobDetails(getTestGridTests(testNames, status))
}

func getTestGridTimestamp() time.Time {
	// Need to subtract a second from now because the code under test also calls
	// time.Now() and if these times are the same, this data will be ignored.
	return time.Now().Add(-1 * time.Second)
}

func getTestGridJobDetails(tests []testgridv1.Test) []testgridv1.JobDetails {
	// This creates a single JobDetails instance with all the tests provided.
	// This assumes single job run with a single change list.
	timestamp := getTestGridTimestamp()

	return []testgridv1.JobDetails{
		{
			Name:  jobName,
			Query: "origin-ci-test/logs/" + jobName,
			ChangeLists: []string{
				"0123456789",
			},
			Tests: tests,
			Timestamps: []int{
				int(timestamp.Unix() * 1000),
			},
		},
	}
}

func getTestGridJobDetailsForRunLength(tests []testgridv1.Test, changelists []string) []testgridv1.JobDetails {
	start := getTestGridTimestamp()

	// Creates a series of timestamps 24 hours apart from one another.
	// Note: We must have exactly as many changelists as timestamps.
	timestamps := []int{}
	for i := 0; i < len(changelists); i++ {
		result := start.Add(time.Duration(-24*i) * time.Hour)
		timestamps = append(timestamps, int(result.Unix()*1000))
	}

	jobDetails := getTestGridJobDetails(tests)

	// Overwrite the default timestamps and changelists since we have our own.
	jobDetails[0].Timestamps = timestamps
	jobDetails[0].ChangeLists = changelists

	return jobDetails
}

func getTestGridTests(testNames []string, status testgridv1.TestStatus) []testgridv1.Test {
	testGridTests := []testgridv1.Test{}

	for _, testName := range testNames {
		testGridTests = append(testGridTests, testgridv1.Test{
			Name: testName,
			Statuses: []testgridv1.TestResult{
				// For simplicity, we only assume a single test status.
				{
					Count: numOfJobs,
					Value: status,
				},
			},
		})
	}

	return testGridTests
}

// Misc. Helper Funcs
func getProwURL(changelist string) string {
	prowURLTemplate := "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/%s/%s"

	return fmt.Sprintf(prowURLTemplate, jobName, changelist)
}

func hasFailedTest(jrr testgridanalysisapi.RawJobRunResult, testName string) bool {
	return sets.NewString(jrr.FailedTestNames...).Has(testName)
}

func hasTestFailures(jrr testgridanalysisapi.RawJobRunResult) bool {
	return jrr.TestFailures > 0 || len(jrr.FailedTestNames) > 0
}

func getTestGridTestStatusCount(results []testgridv1.TestResult) int {
	count := 0

	for _, result := range results {
		count += result.Count
	}

	return count
}

func getStatusName(status testgridv1.TestStatus) string {
	statuses := map[testgridv1.TestStatus]string{
		testgridv1.TestStatusSuccess: "Success",
		testgridv1.TestStatusFailure: "Failure",
		testgridv1.TestStatusFlake:   "Flake",
	}

	statusName, ok := statuses[status]
	if !ok {
		return ""
	}

	return statusName
}
