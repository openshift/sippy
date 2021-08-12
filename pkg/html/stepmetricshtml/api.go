package stepmetricshtml

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type StepMetricsAPI struct {
	current  sippyprocessingv1.TestReport
	previous sippyprocessingv1.TestReport
}

func NewStepMetricsAPI(curr, prev sippyprocessingv1.TestReport) StepMetricsAPI {
	return StepMetricsAPI{
		current:  curr,
		previous: prev,
	}
}

func (s StepMetricsAPI) AllMultistages() []MultistageDetails {
	resp := []MultistageDetails{}
	currStepRegistryMetrics := s.current.TopLevelStepRegistryMetrics.ByMultistageName

	for multistageJobName := range currStepRegistryMetrics {
		resp = append(resp, s.getMultistageForName(multistageJobName))
	}

	return resp
}

func (s StepMetricsAPI) GetMultistage(req Request) MultistageDetails {
	return s.getMultistageForName(req.MultistageJobName)
}

func (s StepMetricsAPI) AllStages() []StepDetails {
	resp := []StepDetails{}

	for stageName := range s.current.TopLevelStepRegistryMetrics.ByStageName {
		resp = append(resp, s.getStageForName(stageName))
	}

	return resp
}

func (s StepMetricsAPI) GetStage(req Request) StepDetails {
	return s.getStageForName(req.StepName)
}

func (s StepMetricsAPI) getStageForName(stageName string) StepDetails {
	currByStageName := s.current.TopLevelStepRegistryMetrics.ByStageName[stageName]
	prevByStageName := s.current.TopLevelStepRegistryMetrics.ByStageName[stageName]

	d := StepDetails{
		Name: stageName,
		Trend: newTrend(
			currByStageName.Aggregated,
			prevByStageName.Aggregated,
		),
		ByMultistage: map[string]StepDetail{},
	}

	for multistageName := range currByStageName.ByMultistageName {
		d.ByMultistage[multistageName] = newStepDetail(
			currByStageName.ByMultistageName[multistageName],
			prevByStageName.ByMultistageName[multistageName],
		)
	}

	return d
}

func (s StepMetricsAPI) getMultistageForName(multistageName string) MultistageDetails {
	currStepRegistryMetrics := s.current.TopLevelStepRegistryMetrics.ByMultistageName[multistageName]
	prevStepRegistryMetrics := s.previous.TopLevelStepRegistryMetrics.ByMultistageName[multistageName]

	d := MultistageDetails{
		Name: multistageName,
		Trend: newTrend(
			getTopLevelMultistageResultAggregation(currStepRegistryMetrics),
			getTopLevelMultistageResultAggregation(prevStepRegistryMetrics),
		),
		StepDetails: map[string]StepDetail{},
	}

	for stageName := range currStepRegistryMetrics.StageResults {
		d.StepDetails[stageName] = newStepDetail(
			currStepRegistryMetrics.StageResults[stageName],
			prevStepRegistryMetrics.StageResults[stageName],
		)
	}

	return d
}

func getTopLevelMultistageResultAggregation(byMultistageName sippyprocessingv1.StepRegistryMetrics) sippyprocessingv1.StageResult {
	results := sippyprocessingv1.StageResult{
		TestResult: sippyprocessingv1.TestResult{
			Name: byMultistageName.MultistageName,
		},
	}

	for _, result := range byMultistageName.StageResults {
		results.Successes += result.Successes
		results.Failures += result.Failures
	}

	if results.Successes+results.Failures != 0 {
		results.PassPercentage = float64(results.Successes) / float64(results.Successes+results.Failures) * 100.0
	}

	return results
}
