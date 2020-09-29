package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func filterPertinentFrequentJobResults(
	in []sippyprocessingv1.JobResult,
	numberOfDaysOfData int, // number of days included in report.
	testResultFilterFn testResultFilterFunc,
) []sippyprocessingv1.JobResult {
	filtered := []sippyprocessingv1.JobResult{}

	for _, job := range in {
		if job.Successes+job.Failures > numberOfDaysOfData*3/2 /*time 1.5*/ {
			job.TestResults = testResultFilterFn.filterTestResults(job.TestResults)
			filtered = append(filtered, job)
		}
	}

	return filtered
}

func filterPertinentInfrequentJobResults(
	in []sippyprocessingv1.JobResult,
	numberOfDaysOfData int, // number of days included in report.
	testResultFilterFn testResultFilterFunc,
) []sippyprocessingv1.JobResult {
	filtered := []sippyprocessingv1.JobResult{}

	for _, job := range in {
		if job.Successes+job.Failures <= numberOfDaysOfData*3/2 /*time 1.5*/ {
			job.TestResults = testResultFilterFn.filterTestResults(job.TestResults)
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
		TestResults: convertRawTestResultsToProcessedTestResults(rawJobResult.TestResults, bugCache, release),
	}

	for _, rawJRR := range rawJobResult.JobRunResults {
		if rawJRR.Failed {
			job.Failures++
		} else if rawJRR.Succeeded {
			job.Successes++
		}
		if rawJRR.Failed && areAllFailuresKnown(rawJRR, bugCache, release) {
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
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
) bool {
	// check if all the test failures in the run can be attributed to
	// known bugs.  If not, the job run was an "unknown failure" that we cannot pretend
	// would have passed if all our bugs were fixed.
	allFailuresKnown := true
	for _, testName := range rawJRR.FailedTestNames {
		bugs := bugCache.ListBugs(release, "", testName)
		isKnownFailure := len(bugs) > 0
		if !isKnownFailure {
			allFailuresKnown = false
			break
		}
	}
	return allFailuresKnown
}

// jobsByPassPercentage sorts from lowest to highest pass percentage
type jobsByPassPercentage []sippyprocessingv1.JobResult

func (a jobsByPassPercentage) Len() int           { return len(a) }
func (a jobsByPassPercentage) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a jobsByPassPercentage) Less(i, j int) bool { return a[i].PassPercentage < a[j].PassPercentage }
