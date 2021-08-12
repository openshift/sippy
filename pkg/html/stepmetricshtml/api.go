package stepmetricshtml

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

type StepMetricsAPI struct {
	StepMetricsHTTPQuery
}

func NewStepMetricsAPI(q StepMetricsHTTPQuery) StepMetricsAPI {
	return StepMetricsAPI{q}
}

func (s StepMetricsAPI) AllMultistages() []MultistageDetails {
	resp := []MultistageDetails{}
	currStepRegistryMetrics := s.Current.TopLevelStepRegistryMetrics.ByMultistageName

	for multistageJobName := range currStepRegistryMetrics {
		resp = append(resp, s.getMultistageForName(multistageJobName))
	}

	return resp
}

func (s StepMetricsAPI) GetMultistage(multistageName string) MultistageDetails {
	return s.getMultistageForName(multistageName)
}

func (s StepMetricsAPI) AllStages() []StepDetails {
	resp := []StepDetails{}

	for stageName := range s.Current.TopLevelStepRegistryMetrics.ByStageName {
		resp = append(resp, s.getStageForName(stageName))
	}

	return resp
}

func (s StepMetricsAPI) GetStage(stageName string) StepDetails {
	return s.getStageForName(stageName)
}

func (s StepMetricsAPI) getStageForName(stageName string) StepDetails {
	currByStageName := s.Current.TopLevelStepRegistryMetrics.ByStageName[stageName]
	prevByStageName := s.Current.TopLevelStepRegistryMetrics.ByStageName[stageName]

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
	currStepRegistryMetrics := s.Current.TopLevelStepRegistryMetrics.ByMultistageName[multistageName]
	prevStepRegistryMetrics := s.Previous.TopLevelStepRegistryMetrics.ByMultistageName[multistageName]

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

func (s StepMetricsAPI) StageNameDetail() generichtml.HTMLTable {
	return generichtml.HTMLTable{}
}

func (s StepMetricsAPI) MultistageDetail() generichtml.HTMLTable {
	return generichtml.HTMLTable{}
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
