package testreportconversion

import (
	"sort"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

// convertRawDataToByVariant takes the raw data and produces a map of variant names to job and test results
func convertRawDataToByVariant(
	allJobResults []sippyprocessingv1.JobResult,
	testResultFilterFn TestResultFilterFunc,
	variantManager testidentification.VariantManager,
) []sippyprocessingv1.VariantResults {

	variantResults := []sippyprocessingv1.VariantResults{}
	for _, variant := range variantManager.AllVariants().List() {

		allVariantTestResults := []sippyprocessingv1.TestResult{}
		jobResults := []sippyprocessingv1.JobResult{}
		successfulJobRuns := 0
		failedJobRuns := 0
		knownFailureJobRuns := 0
		infraFailureJobRuns := 0

		// do this the expensive way until we have a unit test.  This allows us to build the full variant result all at once.
		for _, jobResult := range allJobResults {
			if !sets.NewString(variantManager.IdentifyVariants(jobResult.Name)...).Has(variant) {
				continue
			}

			successfulJobRuns += jobResult.Successes
			failedJobRuns += jobResult.Failures
			knownFailureJobRuns += jobResult.KnownFailures
			// only record infrastructure failures if the number of infrastructure failures makes sense in light of the number of overall runs
			// we've had issues where infrastructure failures for kubernetes are not properly recognized.
			if jobResult.InfrastructureFailures <= jobResult.Failures {
				infraFailureJobRuns += jobResult.InfrastructureFailures
			}

			// combined the test results *before* we filter them
			allVariantTestResults = combineTestResults(jobResult.TestResults, allVariantTestResults)

			jobResults = append(jobResults, jobResult)
		}

		filteredVariantTestResults := testResultFilterFn.FilterTestResults(allVariantTestResults)
		sort.Stable(jobsByPassPercentage(jobResults))

		variantResult := sippyprocessingv1.VariantResults{
			VariantName:                           variant,
			JobRunSuccesses:                       successfulJobRuns,
			JobRunFailures:                        failedJobRuns,
			JobRunKnownFailures:                   knownFailureJobRuns,
			JobRunInfrastructureFailures:          infraFailureJobRuns,
			JobRunPassPercentage:                  percent(successfulJobRuns, failedJobRuns),
			JobRunPassPercentageWithKnownFailures: percent(successfulJobRuns+knownFailureJobRuns, failedJobRuns-knownFailureJobRuns),
			JobRunPassPercentageWithoutInfrastructureFailures: percent(successfulJobRuns, failedJobRuns-infraFailureJobRuns),
			JobResults:     jobResults,
			AllTestResults: filteredVariantTestResults,
		}

		variantResults = append(variantResults, variantResult)
	}

	sort.Stable(variantByJobPassPercentage(variantResults))

	return variantResults
}

func convertVariantResultsToHealth(variants []sippyprocessingv1.VariantResults) sippyprocessingv1.VariantHealth {
	result := sippyprocessingv1.VariantHealth{}

	for _, variant := range variants {
		if variant.VariantName == "never-stable" || (variant.JobRunFailures+variant.JobRunSuccesses == 0) {
			continue
		}

		if variant.JobRunPassPercentage >= 80 {
			result.Success++
		} else if variant.JobRunPassPercentage >= 60 {
			result.Unstable++
		} else {
			result.Failed++
		}
	}

	return result
}

// variantByJobPassPercentage sorts from lowest to highest pass percentage
type variantByJobPassPercentage []sippyprocessingv1.VariantResults

func (a variantByJobPassPercentage) Len() int      { return len(a) }
func (a variantByJobPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a variantByJobPassPercentage) Less(i, j int) bool {
	return a[i].JobRunPassPercentage < a[j].JobRunPassPercentage
}
