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

func (s StepMetricsAPI) Fetch(req Request) (Response, error) {
	resp := Response{
		Request: req,
	}

	if err := validateAPIRequest(s.current, s.previous, req); err != nil {
		return resp, err
	}

	if req.MultistageJobName != "" {
		if req.MultistageJobName == All {
			resp.MultistageDetails = s.AllMultistages()
		} else {
			resp.MultistageDetails = map[string]MultistageDetails{
				req.MultistageJobName: s.GetMultistage(req),
			}
		}

		return resp, nil
	}

	if req.StepName != "" {
		if req.StepName == All {
			resp.StepDetails = s.AllStages()
		} else {
			resp.StepDetails = map[string]StepDetails{
				req.StepName: s.GetStage(req),
			}
		}

		return resp, nil
	}

	if req.JobName != "" {
		return s.GetJob(req), nil
	}

	return resp, nil
}

func (s StepMetricsAPI) AllMultistages() map[string]MultistageDetails {
	resp := map[string]MultistageDetails{}
	currStepRegistryMetrics := s.current.TopLevelStepRegistryMetrics.ByMultistageName

	for multistageJobName := range currStepRegistryMetrics {
		resp[multistageJobName] = s.getMultistageForName(multistageJobName)
	}

	return resp
}

func (s StepMetricsAPI) GetMultistage(req Request) MultistageDetails {
	return s.getMultistageForName(req.MultistageJobName)
}

func (s StepMetricsAPI) AllStages() map[string]StepDetails {
	resp := map[string]StepDetails{}

	for stageName := range s.current.TopLevelStepRegistryMetrics.ByStageName {
		resp[stageName] = s.getStageForName(stageName)
	}

	return resp
}

func (s StepMetricsAPI) GetJob(req Request) Response {
	currByJobName := s.current.TopLevelStepRegistryMetrics.ByJobName[req.JobName]
	prevByJobName := s.previous.TopLevelStepRegistryMetrics.ByJobName[req.JobName]

	multistageName := currByJobName.MultistageName

	stepDetails := map[string]StepDetail{}

	for stageName := range currByJobName.StageResults {
		stepDetails[stageName] = newStepDetail(
			currByJobName.StageResults[stageName],
			prevByJobName.StageResults[stageName],
		)
	}

	return Response{
		Request: req,
		MultistageDetails: map[string]MultistageDetails{
			multistageName: MultistageDetails{
				Name: multistageName,
				Trend: newTrend(
					currByJobName.StepRegistryMetrics.Aggregated,
					prevByJobName.StepRegistryMetrics.Aggregated,
				),
				StepDetails: stepDetails,
			},
		},
	}
}

func (s StepMetricsAPI) GetStage(req Request) StepDetails {
	return s.getStageForName(req.StepName)
}

func (s StepMetricsAPI) getStageForName(stageName string) StepDetails {
	currByStageName := s.current.TopLevelStepRegistryMetrics.ByStageName[stageName]
	prevByStageName := s.previous.TopLevelStepRegistryMetrics.ByStageName[stageName]

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
			currStepRegistryMetrics.Aggregated,
			prevStepRegistryMetrics.Aggregated,
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
