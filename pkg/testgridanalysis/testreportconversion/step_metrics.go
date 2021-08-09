package testreportconversion

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func getTopLevelStepRegistryMetrics(allJobResults []sippyprocessingv1.JobResult) sippyprocessingv1.TopLevelStepRegistryMetrics {
	// Find all step registry metrics by the top-level multistage job name (e.g., e2e-aws)
	byMultistageName := getStepRegistryMetricsByMultistageName(allJobResults)

	return sippyprocessingv1.TopLevelStepRegistryMetrics{
		ByMultistageName: byMultistageName,
		ByStageName:      getStepRegistryMetricsByStageName(byMultistageName),
	}
}

func getStepRegistryMetricsByMultistageName(allJobResults []sippyprocessingv1.JobResult) map[string]sippyprocessingv1.StepRegistryMetrics {
	// Group step registry metrics according to the top-level multistage job name (e.g, "e2e-aws")
	results := map[string]sippyprocessingv1.StepRegistryMetrics{}

	for _, jobResult := range allJobResults {
		multistageName := jobResult.StepRegistryMetrics.MultistageName
		stepRegistryMetrics, ok := results[multistageName]
		if !ok {
			// We don't have a result, so insert what we have and continue so we
			// don't double-count the results
			results[multistageName] = jobResult.StepRegistryMetrics
			continue
		}

		// Update our existing step registry metrics with what we've just found.
		results[multistageName] = updateStepRegistryMetrics(stepRegistryMetrics, jobResult.StepRegistryMetrics)
	}

	return results
}

func updateStepRegistryMetrics(groupedStepRegistryMetrics, ungroupedStepRegistryMetrics sippyprocessingv1.StepRegistryMetrics) sippyprocessingv1.StepRegistryMetrics {
	// Visit each individual stage result and update accordingly
	for stageName, stageResult := range ungroupedStepRegistryMetrics.StageResults {
		groupedStepRegistryMetrics.StageResults[stageName] = addStageResult(groupedStepRegistryMetrics.StageResults[stageName], stageResult)
	}

	return groupedStepRegistryMetrics
}

func addStageResult(sr1, sr2 sippyprocessingv1.StageResult) sippyprocessingv1.StageResult {
	// Increment successes, failures and recalculate the pass percentage
	sr1.Successes += sr2.Successes
	sr1.Failures += sr2.Failures
	sr1.PassPercentage = percent(sr1.Successes, sr1.Failures)

	return sr1
}

func getStepRegistryMetricsByStageName(byMultistageName map[string]sippyprocessingv1.StepRegistryMetrics) map[string]sippyprocessingv1.ByStageName {
	// We've already aggregated by the top-level multistage name, so now we can aggregate by individual stage results.
	results := map[string]sippyprocessingv1.ByStageName{}

	for multistageName, stepRegistryMetrics := range byMultistageName {
		for stageName, stageResults := range stepRegistryMetrics.StageResults {
			byStageName, ok := results[stageName]
			if !ok {
				// We don't have any results for this stage name yet, so add and
				// continue to avoid double-counting the results.
				byStageName := sippyprocessingv1.ByStageName{
					Aggregated: stageResults,
					ByMultistageName: map[string]sippyprocessingv1.StageResult{
						multistageName: stageResults,
					},
				}
				// Clear the original test name value value since it's not applicable
				// in the aggregate case.
				byStageName.Aggregated.OriginalTestName = ""
				results[stageName] = byStageName
				continue
			}

			// Add our stage individual stage results to what we already have
			byStageName.Aggregated = addStageResult(byStageName.Aggregated, stageResults)

			// Add the individual stage results so we can refer to its results by the op-level multistage job name
			byStageName.ByMultistageName[multistageName] = stageResults

			results[stageName] = byStageName
		}
	}

	return results
}

func getStageResultFromRawStageState(rawStepRegistryState testgridanalysisapi.StageState) sippyprocessingv1.StageResult {
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name: rawStepRegistryState.Name,
		},
		OriginalTestName: rawStepRegistryState.OriginalTestName,
	}
}

func addRawStageResult(stageResult sippyprocessingv1.StageResult, rawStepRegistryState testgridanalysisapi.StageState) sippyprocessingv1.StageResult {
	// Increment successes, failures and update the pass percentage.
	if rawStepRegistryState.State == testgridanalysisapi.Success {
		stageResult.Successes++
	}

	if rawStepRegistryState.State == testgridanalysisapi.Failure {
		stageResult.Failures++
	}

	stageResult.PassPercentage = percent(stageResult.Successes, stageResult.Failures)

	return stageResult
}

func getStepRegistryMetrics(rawJobRunResults map[string]testgridanalysisapi.RawJobRunResult) sippyprocessingv1.StepRegistryMetrics {
	stepRegistryMetrics := sippyprocessingv1.StepRegistryMetrics{
		StageResults: map[string]sippyprocessingv1.StageResult{},
	}

	// Examine all of the raw job run results for step registry items
	for _, rawJRR := range rawJobRunResults {
		// We don't (yet) have a multistage result set up, so lets get that set up
		if stepRegistryMetrics.MultistageName == "" {
			stepRegistryMetrics.MultistageName = rawJRR.StepRegistryItemStates.MultistageName
		}

		// Visit each step registry result and create / update each one
		for _, stepRegistryResult := range rawJRR.StepRegistryItemStates.States {
			stageResult, ok := stepRegistryMetrics.StageResults[stepRegistryResult.Name]
			if !ok {
				stageResult = getStageResultFromRawStageState(stepRegistryResult)
			}

			// Write this back to our results map
			stepRegistryMetrics.StageResults[stepRegistryResult.Name] = addRawStageResult(stageResult, stepRegistryResult)
		}
	}

	return stepRegistryMetrics
}
