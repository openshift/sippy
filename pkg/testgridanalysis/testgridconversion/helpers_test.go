package testgridconversion_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/util/sets"
)

// Assertions
func assertChangelistsEqual(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expectedChangelists []string) {
	t.Helper()

	actualChangelists := sets.StringKeySet(rawJobResult.JobRunResults)
	expectedChangelistsSet := changelistsToProwURLSet(expectedChangelists)

	if !actualChangelists.Equal(expectedChangelistsSet) {
		t.Errorf("expected changelist(s) to equal: %v, got: %v",
			getChangelistsFromProwURLSet(expectedChangelistsSet),
			getChangelistsFromProwURLSet(actualChangelists))
	}
}

func assertHasChangelists(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expectedChangelists []string) {
	t.Helper()

	actualChangelists := sets.StringKeySet(rawJobResult.JobRunResults)
	expectedChangelistsSet := changelistsToProwURLSet(expectedChangelists)

	if !actualChangelists.IsSuperset(expectedChangelistsSet) {
		diff := expectedChangelistsSet.Difference(actualChangelists)
		t.Errorf("expected to find changelist(s): %v", getChangelistsFromProwURLSet(diff))
	}
}

func assertNotHasChangelists(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expectedNotToHaveChangelists []string) {
	t.Helper()

	actualChangelists := sets.StringKeySet(rawJobResult.JobRunResults)
	expectedChangelistsSet := changelistsToProwURLSet(expectedNotToHaveChangelists)

	if actualChangelists.HasAny(expectedNotToHaveChangelists...) {
		diff := expectedChangelistsSet.Difference(actualChangelists)
		t.Errorf("expected not to find changelist(s): %v", getChangelistsFromProwURLSet(diff))
	}
}

func assertNoWarnings(t *testing.T, warnings []string) {
	t.Helper()

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func assertWarningsEqual(t *testing.T, have, want []string) {
	t.Helper()

	haveSet := sets.NewString(have...)
	wantSet := sets.NewString(want...)

	if !haveSet.Equal(wantSet) {
		t.Errorf("expected to find warnings %v, got: %v", wantSet.List(), haveSet.List())
	}
}

func assertRawTestResultsEqual(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expected []testgridanalysisapi.RawTestResult) {
	t.Helper()

	if len(rawJobResult.TestResults) != len(expected) {
		t.Errorf("raw test results size mismatch, expected: %d, got: %d", len(expected), len(rawJobResult.TestResults))
	}

	for _, expectedResult := range expected {
		assertHasRawTestResults(t, rawJobResult, []string{expectedResult.Name})
		assertRawTestResultEqual(t, rawJobResult.TestResults[expectedResult.Name], expectedResult)
	}
}

func assertRawTestResultEqual(t *testing.T, have, want testgridanalysisapi.RawTestResult) {
	t.Helper()

	if have.Name != want.Name {
		t.Errorf("raw test result name mismatch, have: %s, want: %s", have.Name, want.Name)
	}

	if have.Successes != want.Successes {
		t.Errorf("success mismatch, have: %d, want: %d", have.Successes, want.Successes)
	}

	if have.Failures != want.Failures {
		t.Errorf("failure mismatch, have: %d, want: %d", have.Failures, want.Failures)
	}

	if have.Flakes != want.Flakes {
		t.Errorf("flake mismatch, have: %d, want: %d", have.Flakes, want.Flakes)
	}

	if len(have.Timestamps) != len(want.Timestamps) {
		t.Errorf("timestamp size mismatch, have: %d, want: %d", len(have.Timestamps), len(want.Timestamps))
	}

	for i := range have.Timestamps {
		if have.Timestamps[i] != want.Timestamps[i] {
			t.Errorf("timestamp mismatch, have: %d, want: %d", have.Timestamps[i], want.Timestamps[i])
		}
	}
}

func assertRawTestResultsNamesEqual(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, expectedTestNames []string) {
	t.Helper()

	wantSet := sets.NewString(expectedTestNames...)
	haveSet := sets.StringKeySet(rawJobResult.TestResults)

	if !wantSet.Equal(haveSet) {
		t.Errorf("raw test result names not equal: have: %v, want: %v", haveSet.List(), wantSet.List())
	}
}

func assertHasRawTestResults(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, testNames []string) {
	t.Helper()

	for _, testName := range testNames {
		if _, ok := rawJobResult.TestResults[testName]; !ok {
			t.Errorf("expected to find raw test result with name: %s", testName)
		}
	}
}

func assertNotHasRawTestResults(t *testing.T, rawJobResult testgridanalysisapi.RawJobResult, testNames []string) {
	t.Helper()

	for _, testName := range testNames {
		if _, ok := rawJobResult.TestResults[testName]; ok {
			t.Errorf("expected not to find raw test result with name: %s", testName)
		}
	}
}

func assertTestGridJobDetailsOK(t *testing.T, jobDetails []testgridv1.JobDetails, jobNum int) {
	t.Helper()

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

func assertNoErrors(t *testing.T, errs []error) {
	t.Helper()

	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func assertMissingOverallErrors(t *testing.T, errs []error) {
	t.Helper()

	for _, err := range errs {
		assertMissingOverallError(t, err)
	}
}

func assertMissingOverallError(t *testing.T, err error) {
	t.Helper()

	if !errors.As(err, &testgridconversion.MissingOverallError{}) {
		t.Errorf("expected error to be MissingOverallError, got unknown error type")
	}

	if !strings.Contains(err.Error(), jobName) {
		t.Errorf("expected error to have the job name (%s)", jobName)
	}
}

func assertErrorEqual(t *testing.T, err, expectedErr error) {
	t.Helper()

	// I know this isn't the preferred way to do this
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error to be %s, got: %s", expectedErr.Error(), err.Error())
	}
}

// Input generation funcs
func getTestGridJobDetailsFromTestNames(testNames []string, status testgridv1.TestStatus) []testgridv1.JobDetails {
	return getTestGridJobDetails(getTestGridTests(testNames, status))
}

func getTestGridJobDetails(tests []testgridv1.Test) []testgridv1.JobDetails {
	return []testgridv1.JobDetails{
		{
			Name:  jobName,
			Query: "origin-ci-test/logs/" + jobName,
			ChangeLists: []string{
				"0123456789",
			},
			Tests: tests,
			Timestamps: []int{
				int(time.Now().Unix() * 1000),
			},
		},
	}
}

func getTestGridJobDetailsForSkipped(testNames []string) []testgridv1.JobDetails {
	// We generate two TestGridJobDetails, one with an "Overall" test, and one
	// without.
	skippedJobName := "periodic-ci-openshift-release-master-nightly-4.9-e2e-gcp"

	// Get a set of TestGrid job details with an overall test
	testGridJobDetailWithOverall := getTestGridJobDetailsFromTestNames(
		append(testNames, overallTestName), testgridv1.TestStatusSuccess)[0]

	// Get a set of TestGrid job details without an overall test
	testGridJobDetailWithoutOverall := getTestGridJobDetailsFromTestNames(
		append(testNames, "failing-test"), testgridv1.TestStatusFailure)[0]

	// Change these values so they're unique
	testGridJobDetailWithoutOverall.ChangeLists[0] = "9876543210"
	testGridJobDetailWithoutOverall.Name = skippedJobName
	testGridJobDetailWithoutOverall.Query = "origin-ci-test/logs/" + skippedJobName

	return []testgridv1.JobDetails{
		testGridJobDetailWithOverall,
		testGridJobDetailWithoutOverall,
	}
}

func getTestGridJobDetailsForRunLength(tests []testgridv1.Test, changelists []string) []testgridv1.JobDetails {
	start := time.Now()

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

func getChangelistsFromProwURLSet(prowURLs sets.String) []string {
	results := []string{}

	for _, prowURL := range prowURLs.List() {
		results = append(results, filepath.Base(prowURL))
	}

	return results
}

func changelistsToProwURLSet(changelists []string) sets.String {
	changelistSet := sets.NewString()

	for _, changelist := range changelists {
		changelistSet.Insert(getProwURL(changelist))
	}

	return changelistSet
}
