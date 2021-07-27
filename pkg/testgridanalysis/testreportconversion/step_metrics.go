package testreportconversion

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

func getTopLevelStepRegistryMetrics(allJobResults []sippyprocessingv1.JobResult) sippyprocessingv1.TopLevelStepRegistryMetrics {
	return sippyprocessingv1.TopLevelStepRegistryMetrics{
		ByMultistageName: getStepRegistryMetricsByMultistageName(allJobResults),
		ByStageName:      getStepRegistryMetricsByStageName(allJobResults),
	}
}

func getStepRegistryMetricsByMultistageName(allJobResults []sippyprocessingv1.JobResult) map[string]map[string]sippyprocessingv1.StageResult {
	results := map[string]map[string]sippyprocessingv1.StageResult{}

	for _, jobResult := range allJobResults {
		multistageFoundResult, ok := results[jobResult.StepRegistryMetrics.MultistageName]
		if !ok {
			multistageFoundResult = map[string]sippyprocessingv1.StageResult{}
		}

		multistageFoundResult = addStageResults(jobResult.StepRegistryMetrics.StageResults, multistageFoundResult)

		results[jobResult.StepRegistryMetrics.MultistageName] = multistageFoundResult
	}

	return results
}

func getStepRegistryMetricsByStageName(allJobResults []sippyprocessingv1.JobResult) map[string]sippyprocessingv1.StageResult {
	results := map[string]sippyprocessingv1.StageResult{}

	for _, jobResult := range allJobResults {
		results = addStageResults(jobResult.StepRegistryMetrics.StageResults, results)
	}

	return results
}

func addStageResults(stageResults []sippyprocessingv1.StageResult, preexistingResults map[string]sippyprocessingv1.StageResult) map[string]sippyprocessingv1.StageResult {
	for _, stageResult := range stageResults {
		stageFoundResult, ok := preexistingResults[stageResult.Name]
		if ok {
			stageFoundResult.Successes += stageResult.Successes
			stageFoundResult.Failures += stageResult.Failures
		} else {
			stageFoundResult = stageResult
		}

		stageFoundResult.PassPercentage = percent(stageFoundResult.Successes, stageFoundResult.Failures)
		preexistingResults[stageResult.Name] = stageFoundResult
	}

	return preexistingResults
}

func getStageResultFromRawStageState(rawStepRegistryState testgridanalysisapi.StageState) sippyprocessingv1.StageResult {
	return sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name: rawStepRegistryState.Name,
		},
		OriginalTestName: rawStepRegistryState.OriginalTestName,
	}
}

func updateStageResult(stageResult sippyprocessingv1.StageResult, rawStepRegistryState testgridanalysisapi.StageState) sippyprocessingv1.StageResult {
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
	stepRegistryMetrics := sippyprocessingv1.StepRegistryMetrics{}
	stageResults := map[string]sippyprocessingv1.StageResult{}

	for _, rawJRR := range rawJobRunResults {
		if stepRegistryMetrics.MultistageName == "" {
			stepRegistryMetrics.MultistageName = rawJRR.StepRegistryItemStates.MultistageName
			stepRegistryMetrics.MultistageResult = getStageResultFromRawStageState(rawJRR.StepRegistryItemStates.MultistageState)
		}

		stepRegistryMetrics.MultistageResult = updateStageResult(stepRegistryMetrics.MultistageResult, rawJRR.StepRegistryItemStates.MultistageState)

		for _, stepRegistryResult := range rawJRR.StepRegistryItemStates.States {
			stageResult, ok := stageResults[stepRegistryResult.Name]
			if !ok {
				stageResult = getStageResultFromRawStageState(stepRegistryResult)
			}

			stageResults[stepRegistryResult.Name] = updateStageResult(stageResult, stepRegistryResult)
		}
	}

	for _, stageResult := range stageResults {
		stageResult.PassPercentage = percent(stageResult.Successes, stageResult.Failures)
		stepRegistryMetrics.StageResults = append(stepRegistryMetrics.StageResults, stageResult)
	}

	return stepRegistryMetrics
}
