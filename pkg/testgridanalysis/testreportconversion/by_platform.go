package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

// convertRawDataToByPlatform takes the raw data and produces a map of platform names to job and test results
func convertRawDataToByPlatform(
	allJobResults []sippyprocessingv1.JobResult,
	testResultFilterFn testResultFilterFunc,
) []sippyprocessingv1.PlatformResults {

	platformResults := []sippyprocessingv1.PlatformResults{}
	for _, platform := range testidentification.AllPlatforms.List() {

		allPlatformTestResults := []sippyprocessingv1.TestResult{}
		jobResults := []sippyprocessingv1.JobResult{}
		successfulJobRuns := 0
		failedJobRuns := 0
		knownFailureJobRuns := 0
		infraFailureJobRuns := 0

		// do this the expensive way until we have a unit test.  This allows us to build the full platform result all at once.
		for _, jobResult := range allJobResults {
			if !sets.NewString(testidentification.FindPlatform(jobResult.Name)...).Has(platform) {
				continue
			}

			successfulJobRuns += jobResult.Successes
			failedJobRuns += jobResult.Failures
			knownFailureJobRuns += jobResult.KnownFailures
			infraFailureJobRuns += jobResult.InfrastructureFailures

			// combined the test results *before* we filter them
			allPlatformTestResults = combineTestResults(jobResult.TestResults, allPlatformTestResults)

			jobResults = append(jobResults, jobResult)
		}

		filteredPlatformTestResults := testResultFilterFn.filterTestResults(allPlatformTestResults)
		sort.Stable(jobsByPassPercentage(jobResults))

		platformResults = append(platformResults, sippyprocessingv1.PlatformResults{
			PlatformName:                          platform,
			JobRunSuccesses:                       successfulJobRuns,
			JobRunFailures:                        failedJobRuns,
			JobRunKnownFailures:                   knownFailureJobRuns,
			JobRunInfrastructureFailures:          infraFailureJobRuns,
			JobRunPassPercentage:                  percent(successfulJobRuns, failedJobRuns),
			JobRunPassPercentageWithKnownFailures: percent(successfulJobRuns+knownFailureJobRuns, failedJobRuns-knownFailureJobRuns),
			JobRunPassPercentageWithoutInfrastructureFailures: percent(successfulJobRuns, failedJobRuns-infraFailureJobRuns),
			JobResults:     jobResults,
			AllTestResults: filteredPlatformTestResults,
		})
	}

	sort.Stable(platformsByJobPassPercentage(platformResults))

	return platformResults
}

// platformsByJobPassPercentage sorts from lowest to highest pass percentage
type platformsByJobPassPercentage []sippyprocessingv1.PlatformResults

func (a platformsByJobPassPercentage) Len() int      { return len(a) }
func (a platformsByJobPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a platformsByJobPassPercentage) Less(i, j int) bool {
	return a[i].JobRunPassPercentage < a[j].JobRunPassPercentage
}
