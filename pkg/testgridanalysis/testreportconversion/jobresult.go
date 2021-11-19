package testreportconversion

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/montanaflynn/stats"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
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

func isNeverStableOrTechPreview(result sippyprocessingv1.JobResult) bool {
	for _, variant := range result.Variants {
		if variant == "never-stable" || variant == "techpreview" {
			return true
		}
	}

	return false
}

// The zero-value of a float64 in go is NaN, however you cannot marshal that
// value to JSON. This converts a NaN to zero.
func convertNaNToZero(f float64) float64 {
	if math.IsNaN(f) {
		return 0.0
	}

	return f
}

func calculateJobResultStatistics(results []sippyprocessingv1.JobResult) sippyprocessingv1.Statistics {
	jobStatistics := sippyprocessingv1.Statistics{}
	percentages := []float64{}
	jobStatistics.Histogram = make([]int, 10)

	sort.Slice(results, func(i, j int) bool {
		return results[i].PassPercentage > results[j].PassPercentage
	})

	for _, result := range results {
		if isNeverStableOrTechPreview(result) {
			continue
		}
		index := int(math.Floor(result.PassPercentage / 10))
		if index == 10 { // 100% gets bucketed in the 10th bucket
			index = 9
		}
		jobStatistics.Histogram[index]++

		percentages = append(percentages, result.PassPercentage)
	}

	data := stats.LoadRawData(percentages)
	mean, _ := stats.Mean(data)
	sd, _ := stats.StandardDeviation(data)
	quartiles, _ := stats.Quartile(data)
	p95, _ := stats.Percentile(data, 95)

	jobStatistics.Mean = mean
	jobStatistics.StandardDeviation = sd
	jobStatistics.Quartiles = []float64{
		convertNaNToZero(quartiles.Q1),
		convertNaNToZero(quartiles.Q2),
		convertNaNToZero(quartiles.Q3),
	}
	jobStatistics.P95 = p95

	return jobStatistics
}

// convertRawJobResultsToProcessedJobResults performs no filtering
func convertRawJobResultsToProcessedJobResults(
	reportName string, // technically the release, i.e. "4.10"
	rawData testgridanalysisapi.RawData,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	bugzillaRelease string, // required to limit bugs to those that apply to the release in question,
	manager testidentification.VariantManager,
) []sippyprocessingv1.JobResult {
	jobs := []sippyprocessingv1.JobResult{}
	rawJobResults := rawData.JobResults

	for _, rawJobResult := range rawJobResults {
		job := convertRawJobResultToProcessedJobResult(reportName, rawJobResult, bugCache, bugzillaRelease, manager)
		jobs = append(jobs, job)
	}

	sort.Stable(jobsByPassPercentage(jobs))

	return jobs
}

func convertRawJobResultToProcessedJobResult(
	reportName string,
	rawJobResult testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	bugzillaRelease string, // required to limit bugs to those that apply to the release in question,
	manager testidentification.VariantManager,
) sippyprocessingv1.JobResult {
	job := sippyprocessingv1.JobResult{
		Name:              rawJobResult.JobName,
		Release:           reportName,
		Variants:          manager.IdentifyVariants(rawJobResult.JobName),
		TestGridURL:       rawJobResult.TestGridJobURL,
		TestResults:       convertRawTestResultsToProcessedTestResults(rawJobResult.JobName, rawJobResult.TestResults, bugCache, bugzillaRelease),
		BugList:           bugCache.ListBugs(bugzillaRelease, rawJobResult.JobName, ""),
		AssociatedBugList: bugCache.ListAssociatedBugs(bugzillaRelease, rawJobResult.JobName, ""),
	}

	for _, rawJRR := range rawJobResult.JobRunResults {
		job.AllRuns = append(job.AllRuns, convertRawToJobRunResult(rawJRR))

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

	// Ensure jobs are ordered by timestamp
	sort.Slice(job.AllRuns, func(i, j int) bool {
		return job.AllRuns[i].Timestamp > job.AllRuns[j].Timestamp
	})

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

func convertRawToJobRunResult(jrr testgridanalysisapi.RawJobRunResult) sippyprocessingv1.JobRunResult {
	// Add a ProwID we can use as a key in our db, by extracting from the end of the URL:
	tokens := strings.Split(jrr.JobRunURL, "/")
	prowID, _ := strconv.ParseInt(tokens[len(tokens)-1], 10, 64)
	return sippyprocessingv1.JobRunResult{
		ProwID:          prowID,
		Job:             jrr.Job,
		URL:             jrr.JobRunURL,
		TestFailures:    jrr.TestFailures,
		FailedTestNames: jrr.FailedTestNames,
		Failed:          jrr.Failed,
		Succeeded:       jrr.Succeeded,
		Timestamp:       jrr.Timestamp,
		OverallResult:   jrr.OverallResult,
	}
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

// jobsByPassPercentage sorts from lowest to highest pass percentage
type jobsByPassPercentage []sippyprocessingv1.JobResult

func (a jobsByPassPercentage) Len() int           { return len(a) }
func (a jobsByPassPercentage) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a jobsByPassPercentage) Less(i, j int) bool { return a[i].PassPercentage < a[j].PassPercentage }
