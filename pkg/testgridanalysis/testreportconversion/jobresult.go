package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func FilterJobResultTests(jobResult *sippyprocessingv1.JobResult, testFilterFn TestResultFilterFunc) *sippyprocessingv1.JobResult {
	if jobResult == nil {
		return nil
	}

	out := *jobResult
	testResults := []sippyprocessingv1.TestResult{}
	for i := range out.TestResults {
		testResult := out.TestResults[i]
		if testFilterFn(testResult) {
			testResults = append(testResults, testResult)
		}
	}
	out.TestResults = testResults
	return &out
}

func filterPertinentFrequentJobResults(
	in []sippyprocessingv1.JobResult,
	numberOfDaysOfData int, // number of days included in report.
	testResultFilterFn TestResultFilterFunc,
) []sippyprocessingv1.JobResult {
	filtered := []sippyprocessingv1.JobResult{}

	for _, job := range in {
		if job.Successes+job.Failures > numberOfDaysOfData*3/2 /*time 1.5*/ {
			job.TestResults = testResultFilterFn.FilterTestResults(job.TestResults)
			filtered = append(filtered, job)
		}
	}

	return filtered
}

func filterPertinentInfrequentJobResults(
	in []sippyprocessingv1.JobResult,
	numberOfDaysOfData int, // number of days included in report.
	testResultFilterFn TestResultFilterFunc,
) []sippyprocessingv1.JobResult {
	filtered := []sippyprocessingv1.JobResult{}

	for _, job := range in {
		if job.Successes+job.Failures <= numberOfDaysOfData*3/2 /*time 1.5*/ {
			job.TestResults = testResultFilterFn.FilterTestResults(job.TestResults)
			filtered = append(filtered, job)
		}
	}

	return filtered
}

// convertRawJobResultsToProcessedJobResults performs no filtering
func convertRawJobResultsToProcessedJobResults(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
) []sippyprocessingv1.JobResult {
	jobs := []sippyprocessingv1.JobResult{}

	for _, rawJobResult := range rawJobResults {
		job := convertRawJobResultToProcessedJobResult(rawJobResult, bugCache, release)
		jobs = append(jobs, job)
	}

	sort.Stable(jobsByPassPercentage(jobs))

	return jobs
}

func convertRawJobResultToProcessedJobResult(
	rawJobResult testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
) sippyprocessingv1.JobResult {

	job := sippyprocessingv1.JobResult{
		Name:        rawJobResult.JobName,
		TestGridUrl: rawJobResult.TestGridJobUrl,
		TestResults: convertRawTestResultsToProcessedTestResults(rawJobResult.JobName, rawJobResult.TestResults, bugCache, release),
	}

	for _, rawJRR := range rawJobResult.JobRunResults {
		if rawJRR.Failed {
			job.Failures++
		} else if rawJRR.Succeeded {
			job.Successes++
		}
		if rawJRR.Failed && areAllFailuresKnown(rawJRR, job.TestResults) {
			job.KnownFailures++
		}
		if rawJRR.SetupStatus != testgridanalysisapi.Success {
			job.InfrastructureFailures++
		}
	}

	job.PassPercentage = percent(job.Successes, job.Failures)
	job.PassPercentageWithKnownFailures = percent(job.Successes+job.KnownFailures, job.Failures-job.KnownFailures)
	job.PassPercentageWithoutInfrastructureFailures = percent(job.Successes, job.Failures-job.InfrastructureFailures)

	return job
}

func areAllFailuresKnown(
	rawJRR testgridanalysisapi.RawJobRunResult,
	allTestResults []sippyprocessingv1.TestResult,
) bool {
	// check if all the test failures in the run can be attributed to
	// known bugs.  If not, the job run was an "unknown failure" that we cannot pretend
	// would have passed if all our bugs were fixed.
	for _, testName := range rawJRR.FailedTestNames {
		for _, testResult := range allTestResults {
			if testResult.Name == testName && len(testResult.BugList) == 0 {
				return false
			}
		}
	}
	return true
}

func areAllFailuresKnownFromProcessedResults(
	rawJRR testgridanalysisapi.RawJobRunResult,
	allTestResultsByName testResultsByName,
) bool {
	// check if all the test failures in the run can be attributed to
	// known bugs.  If not, the job run was an "unknown failure" that we cannot pretend
	// would have passed if all our bugs were fixed.
	for _, testName := range rawJRR.FailedTestNames {
		isKnownFailure := len(allTestResultsByName[testName].TestResultAcrossAllJobs.BugList) > 0
		if !isKnownFailure {
			return false
		}
	}
	return true
}

// jobsByPassPercentage sorts from lowest to highest pass percentage
type jobsByPassPercentage []sippyprocessingv1.JobResult

func (a jobsByPassPercentage) Len() int           { return len(a) }
func (a jobsByPassPercentage) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a jobsByPassPercentage) Less(i, j int) bool { return a[i].PassPercentage < a[j].PassPercentage }
