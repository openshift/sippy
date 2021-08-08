package testreportconversion

import (
	"sort"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

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
	rawData testgridanalysisapi.RawData,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	bugzillaRelease string, // required to limit bugs to those that apply to the release in question,
	manager testidentification.VariantManager,
) []sippyprocessingv1.JobResult {
	jobs := []sippyprocessingv1.JobResult{}
	rawJobResults := rawData.JobResults

	for _, rawJobResult := range rawJobResults {
		job := convertRawJobResultToProcessedJobResult(rawJobResult, bugCache, bugzillaRelease, manager)
		jobs = append(jobs, job)
	}

	sort.Stable(jobsByPassPercentage(jobs))

	return jobs
}

func convertRawJobResultToProcessedJobResult(
	rawJobResult testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	bugzillaRelease string, // required to limit bugs to those that apply to the release in question,
	manager testidentification.VariantManager,
) sippyprocessingv1.JobResult {
	job := sippyprocessingv1.JobResult{
		Name:              rawJobResult.JobName,
		Variants:          manager.IdentifyVariants(rawJobResult.JobName),
		TestGridURL:       rawJobResult.TestGridJobURL,
		TestResults:       convertRawTestResultsToProcessedTestResults(rawJobResult.JobName, rawJobResult.TestResults, bugCache, bugzillaRelease),
		BugList:           bugCache.ListBugs(bugzillaRelease, rawJobResult.JobName, ""),
		AssociatedBugList: bugCache.ListAssociatedBugs(bugzillaRelease, rawJobResult.JobName, ""),
	}

	for url, rawJRR := range rawJobResult.JobRunResults {
		buildStatus := sippyprocessingv1.BuildResult{
			URL:       url,
			Timestamp: rawJRR.Timestamp,
			Result:    rawJRR.OverallStatus,
		}

		job.BuildResults = append(job.BuildResults, buildStatus)

		if rawJRR.Failed {
			job.Failures++
		} else if rawJRR.Succeeded {
			job.Successes++
		}
		if rawJRR.Failed && areAllFailuresKnown(rawJRR, job.TestResults) {
			job.KnownFailures++
		}
		// success - we saw the setup/infra test result, it succeeded (or the whole job succeeeded)
		// failure - we saw the test result, it failed
		// unknown - we know this job doesn't have a setup test, and the job didn't succeed, so we don't know if it
		//           failed due to infra issues or not.  probably not infra.
		// emptystring - we expected to see a test result for a setup test but we didn't and the overall job failed, probably infra
		if rawJRR.SetupStatus != testgridanalysisapi.Success && rawJRR.SetupStatus != testgridanalysisapi.Unknown {
			job.InfrastructureFailures++
		}

	}

	job.PassPercentage = percent(job.Successes, job.Failures)
	job.PassPercentageWithKnownFailures = percent(job.Successes+job.KnownFailures, job.Failures-job.KnownFailures)
	job.PassPercentageWithoutInfrastructureFailures = percent(job.Successes, job.Failures-job.InfrastructureFailures)

	// if there are more infrastructure failures than overall failures, then something is wrong with our accounting and
	// we should make it clear this is an invalid value.
	// TODO wire a warning. This is strictly better than nothing at the moment though.
	if job.InfrastructureFailures > job.Failures {
		job.PassPercentageWithoutInfrastructureFailures = -1
	}

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
