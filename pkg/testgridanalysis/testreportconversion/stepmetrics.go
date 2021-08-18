package testreportconversion

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func getTopLevelStepRegistryMetrics(allJobResults []sippyprocessingv1.JobResult) sippyprocessingv1.TopLevelStepRegistryMetrics {
	// Find all step registry metrics by the top-level multistage job name (e.g., e2e-aws)
	byMultistageName, byJobName := getStepRegistryMetricsByMultistageName(allJobResults)

	return sippyprocessingv1.TopLevelStepRegistryMetrics{
		ByMultistageName: byMultistageName,
		ByStageName:      getStepRegistryMetricsByStageName(byMultistageName),
		ByJobName:        byJobName,
	}
}

func getStepRegistryMetricsByMultistageName(allJobResults []sippyprocessingv1.JobResult) (map[string]sippyprocessingv1.StepRegistryMetrics, map[string]sippyprocessingv1.ByJobName) {
	// Group step registry metrics according to the top-level multistage job name (e.g, "e2e-aws")
	byMultistageName := map[string]sippyprocessingv1.StepRegistryMetrics{}

	// Group step registry metrics according to the job name (e.g., periodic-ci-openshift-release-master-ci-4.9-e2e-*)
	byJobName := map[string]sippyprocessingv1.ByJobName{}

	// It is possible that multiple jobs can use the same multistage job. For
	// example, the periodic-ci-openshift-release-master-ci-4.9-e2e-* and
	// periodic-ci-openshift-release-master-nightly-4.9-e2e-* series do this.
	for _, jobResult := range allJobResults {
		// This job does not use multistage, skip it
		if !isMultistageJob(jobResult.StepRegistryMetrics) {
			continue
		}

		multistageName := jobResult.StepRegistryMetrics.MultistageName

		srm, ok := byMultistageName[multistageName]
		if ok {
			// We already have a multistage job with this name, so combine it with
			// what we already have.
			byMultistageName[multistageName] = combineStepRegistryMetrics(jobResult.StepRegistryMetrics, srm)
		} else {
			// We don't (yet) have a multistage job with this name, so copy what's
			// attached to the job so we don't mutate the per-job metrics.
			byMultistageName[multistageName] = copyStepRegistryMetrics(jobResult.StepRegistryMetrics)
		}

		byJobName[jobResult.Name] = sippyprocessingv1.ByJobName{
			JobName:             jobResult.Name,
			StepRegistryMetrics: copyStepRegistryMetrics(jobResult.StepRegistryMetrics),
		}
	}

	return byMultistageName, byJobName
}

func combineStepRegistryMetrics(srm1, srm2 sippyprocessingv1.StepRegistryMetrics) sippyprocessingv1.StepRegistryMetrics {
	srm := sippyprocessingv1.StepRegistryMetrics{
		MultistageName: srm1.MultistageName,
		StageResults:   map[string]sippyprocessingv1.StageResult{},
	}

	// Combine the stage results from each of the supplied step registry metrics
	for stageName := range srm1.StageResults {
		srm.StageResults[stageName] = addStageResult(srm1.StageResults[stageName], srm2.StageResults[stageName])
	}

	// Update our top-level aggregation
	srm.Aggregated = getAggregation(srm)

	return srm
}

func copyStepRegistryMetrics(stepRegistryMetrics sippyprocessingv1.StepRegistryMetrics) sippyprocessingv1.StepRegistryMetrics {
	srm := sippyprocessingv1.StepRegistryMetrics{
		MultistageName: stepRegistryMetrics.MultistageName,
		StageResults:   map[string]sippyprocessingv1.StageResult{},
	}

	for stageName, stageResult := range stepRegistryMetrics.StageResults {
		srm.StageResults[stageName] = stageResult
	}

	srm.Aggregated = stepRegistryMetrics.Aggregated

	return srm
}

func getAggregation(stepRegistryMetrics sippyprocessingv1.StepRegistryMetrics) sippyprocessingv1.StageResult {
	// It doesn't make sense to add each of the individual steps belonging to a
	// given multistage job to get an aggregated number of runs, etc. as this
	// would lead to an inflated number of multistage runs.
	//
	// For example, if a multistage job has 4 stages and each stage runs 2 times,
	// this would result in our top-level run count being 8, which is not true
	// since the multistage job itself was ran two times.
	//
	// Instead, we get the maximum number of successes and failures from each of
	// the stage results and use that to calculate a top-level aggregation value
	// as this is a much closer approximation.
	aggregated := sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name: stepRegistryMetrics.MultistageName,
		},
	}

	for _, stageResult := range stepRegistryMetrics.StageResults {
		if stageResult.Successes > aggregated.Successes {
			aggregated.Successes = stageResult.Successes
		}

		if stageResult.Failures > aggregated.Failures {
			aggregated.Failures = stageResult.Failures
		}
	}

	aggregated.PassPercentage = percent(aggregated.Successes, aggregated.Failures)
	aggregated.Runs = aggregated.Successes + aggregated.Failures

	return aggregated
}

func addStageResult(sr1, sr2 sippyprocessingv1.StageResult) sippyprocessingv1.StageResult {
	// Increment successes, failures and recalculate the pass percentage / run count
	sr1.Successes += sr2.Successes
	sr1.Failures += sr2.Failures
	sr1.PassPercentage = percent(sr1.Successes, sr1.Failures)
	sr1.Runs = sr1.Successes + sr1.Failures

	return sr1
}

func getStepRegistryMetricsByStageName(byMultistageName map[string]sippyprocessingv1.StepRegistryMetrics) map[string]sippyprocessingv1.ByStageName {
	// We've already aggregated by the top-level multistage name, so now we can
	// aggregate by individual stage results (e.g., "openshift-e2e-test",
	// "ipi-install", etc.)
	results := map[string]sippyprocessingv1.ByStageName{}

	for multistageName, stepRegistryMetrics := range byMultistageName {
		for stageName, stageResults := range stepRegistryMetrics.StageResults {
			byStageName, ok := results[stageName]
			if !ok {
				// We don't have any results for this stage name yet, so add and
				// continue to avoid double-counting results.
				byStageName = sippyprocessingv1.ByStageName{
					Aggregated: stageResults,
					ByMultistageName: map[string]sippyprocessingv1.StageResult{
						multistageName: stageResults,
					},
				}
				// Clear the original test name value value since it's not applicable
				// in the aggregate case.
				byStageName.Aggregated.OriginalTestName = ""
				results[stageName] = byStageName
			} else {
				// We have results for this stage name, so update what we have

				// Recompute our aggregation
				byStageName.Aggregated = addStageResult(byStageName.Aggregated, stageResults)

				// Add the individual stage results so we can refer to its results by the
				// top-level multistage job name (e.g., e2e-aws)
				byStageName.ByMultistageName[multistageName] = stageResults

				results[stageName] = byStageName
			}
		}
	}

	return results
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
	stageResult.Runs = stageResult.Successes + stageResult.Failures

	return stageResult
}

func getStepRegistryMetrics(rawJobRunResults map[string]testgridanalysisapi.RawJobRunResult) sippyprocessingv1.StepRegistryMetrics {
	// Get the step registry metrics for each individual job
	srm := sippyprocessingv1.StepRegistryMetrics{
		StageResults: map[string]sippyprocessingv1.StageResult{},
	}

	// Examine all of the raw job run results for step registry items
	for _, rawJRR := range rawJobRunResults {
		// We don't (yet) have a multistage result, so lets set that up
		if srm.MultistageName == "" {
			srm.MultistageName = rawJRR.StepRegistryItemStates.MultistageName
			srm.Aggregated.Name = rawJRR.StepRegistryItemStates.MultistageName
		}

		// Visit each step registry result and create / update each one
		for _, stepRegistryResult := range rawJRR.StepRegistryItemStates.States {
			stageResult, ok := srm.StageResults[stepRegistryResult.Name]
			if !ok {
				// We don't yet have a stage result, so create one
				stageResult = sippyprocessingv1.StageResult{
					TestResult: sippyprocessingv1.TestResult{
						Name: stepRegistryResult.Name,
					},
					OriginalTestName: stepRegistryResult.OriginalTestName,
				}
			}

			// Write this back to our results map
			srm.StageResults[stepRegistryResult.Name] = addRawStageResult(stageResult, stepRegistryResult)
		}
	}

	// Get our top-level aggregation
	srm.Aggregated = getAggregation(srm)

	return srm
}

func isMultistageJob(srm sippyprocessingv1.StepRegistryMetrics) bool {
	return srm.MultistageName != "" && len(srm.StageResults) != 0
}
