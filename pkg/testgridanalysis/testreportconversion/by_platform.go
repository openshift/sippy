package testreportconversion

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

// convertRawDataToByPlatform takes the raw data and produces a map of platform names to results
func convertRawDataToByPlatform(
	rawJobResults map[string]testgridanalysisapi.RawJobResult,
	bugCache buganalysis.BugCache, // required to associate tests with bug
	release string, // required to limit bugs to those that apply to the release in question
	minRuns int, // indicates how many runs are required for a test is included in overall percentages
	// TODO deads2k wants to eliminate the successThreshold
	successThreshold float64, // indicates an upper bound on how successful a test can be before it is excluded
) map[string]sippyprocessingv1.SortedAggregateTestsResult {

	resultsByPlatform := make(map[string]sippyprocessingv1.SortedAggregateTestsResult)
	for _, platform := range testidentification.AllPlatforms.List() {

		allPlatformTestResults := []sippyprocessingv1.TestResult{}
		successfulJobRuns := 0
		failedJobRuns := 0

		// do this the expensive way until we have a unit test.  This allows us to build the full platform result all at once.
		// TODO if we are too slow, switch this to only build the job results once
		for _, rawJobResult := range rawJobResults {
			if !sets.NewString(testidentification.FindPlatform(rawJobResult.JobName)...).Has(platform) {
				continue
			}

			testResults := convertRawTestResultsToProcessedTestResults(rawJobResult.TestResults, bugCache, release)
			allPlatformTestResults = combineTestResults(testResults, allPlatformTestResults)

			for _, rawJobRunResult := range rawJobResult.JobRunResults {
				if rawJobRunResult.Succeeded {
					successfulJobRuns++
				}
				if rawJobRunResult.Failed {
					failedJobRuns++
				}
			}
		}

		filteredPlatformTestResults := filterTestResults(allPlatformTestResults, minRuns, successThreshold)

		// TODO we should set the successful and failed job
		resultsByPlatform[platform] = sippyprocessingv1.SortedAggregateTestsResult{
			Successes:          0, // not used
			Failures:           0, // not used
			TestPassPercentage: 0, // not used
			TestResults:        filteredPlatformTestResults,
		}
	}

	return resultsByPlatform
}
