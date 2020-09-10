package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func summarizeJobRunResults(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question,
	numberOfDaysOfData int, // number of days included in report.
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
) (jobs []sippyprocessingv1.JobResult, infrequentJobs []sippyprocessingv1.JobResult) {

	for _, rawJobResult := range rawJobResults {
		job := convertRawJobResultToProcessedJobResult(rawJobResult, bugCache, release)

		if job.Successes+job.Failures > numberOfDaysOfData*3/2 /*time 1.5*/ {
			job.TestResults = filterTestResults(job.TestResults, minRuns, successThreshold)
			jobs = append(jobs, job)
		} else {
			// for infrequent jobs, we do not filter out on minimum runs
			job.TestResults = filterTestResults(job.TestResults, 1, successThreshold)
			infrequentJobs = append(infrequentJobs, job)
		}
	}

	sort.Stable(jobsByPassPercentage(jobs))
	sort.Stable(jobsByPassPercentage(infrequentJobs))

	return jobs, infrequentJobs
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
	}

	job.PassPercentage = percent(job.Successes, job.Failures)
	job.PassPercentageWithKnownFailures = percent(job.Successes+job.KnownFailures, job.Failures-job.KnownFailures)

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
