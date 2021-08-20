package stepmetrics

import (
	"fmt"

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
	resp := Response{Request: req}

	if err := validateAPIRequest(s.current, s.previous, req); err != nil {
		return resp, err
	}

	if req.MultistageJobName != "" {
		return s.multistageQuery(req)
	}

	if req.StepName != "" {
		return s.stepQuery(req)
	}

	if req.JobName != "" {
		return s.jobQuery(req), nil
	}

	return resp, fmt.Errorf("bad step metrics query")
}

func (s StepMetricsAPI) multistageQuery(req Request) (Response, error) {
	resp := Response{
		Request: req,
	}

	if req.MultistageJobName == All {
		resp.MultistageDetails = s.allMultistages()
	} else {
		resp.MultistageDetails = map[string]MultistageDetails{
			req.MultistageJobName: s.getMultistageForName(req.MultistageJobName),
		}
	}

	return resp, nil

}

func (s StepMetricsAPI) allMultistages() map[string]MultistageDetails {
	resp := map[string]MultistageDetails{}
	currStepRegistryMetrics := s.current.TopLevelStepRegistryMetrics.ByMultistageName

	for multistageJobName := range currStepRegistryMetrics {
		resp[multistageJobName] = s.getMultistageForName(multistageJobName)
	}

	return resp
}

func (s StepMetricsAPI) stepQuery(req Request) (Response, error) {
	resp := Response{
		Request: req,
	}

	if req.StepName == All {
		resp.StepDetails = s.allStages()
	} else {
		resp.StepDetails = map[string]StepDetails{
			req.StepName: s.getStage(req),
		}
	}

	return resp, nil
}

func (s StepMetricsAPI) allStages() map[string]StepDetails {
	resp := map[string]StepDetails{}

	for stageName := range s.current.TopLevelStepRegistryMetrics.ByStageName {
		resp[stageName] = s.getStageForName(stageName)
	}

	return resp
}

func (s StepMetricsAPI) jobQuery(req Request) Response {
	resp := Response{
		Request: req,
	}

	if req.JobName == All {
		resp.JobDetails = s.getAllJobs(req)
		return resp
	}

	resp.JobDetails = map[string]JobDetails{
		req.JobName: s.getJobDetails(req),
	}

	return resp
}

func (s StepMetricsAPI) getAllJobs(req Request) map[string]JobDetails {
	currByJobName := s.current.TopLevelStepRegistryMetrics.ByJobName

	jobDetails := map[string]JobDetails{}

	for jobName := range currByJobName {
		jobDetails[jobName] = JobDetails{
			JobName: jobName,
			MultistageDetails: s.getJobMultistageDetails(Request{
				Release: req.Release,
				JobName: jobName,
			}),
		}
	}

	return jobDetails
}

func (s StepMetricsAPI) getJobMultistageDetails(req Request) MultistageDetails {
	currByJobName := s.current.TopLevelStepRegistryMetrics.ByJobName[req.JobName]
	prevByJobName := s.previous.TopLevelStepRegistryMetrics.ByJobName[req.JobName]

	multistageName := currByJobName.MultistageName

	stepDetails := map[string]StepDetail{}

	for stageName := range currByJobName.StageResults {
		stepDetails[stageName] = NewStepDetail(
			currByJobName.StageResults[stageName],
			prevByJobName.StageResults[stageName],
		)
	}

	return MultistageDetails{
		Name: multistageName,
		Trend: newTrend(
			currByJobName.StepRegistryMetrics.Aggregated,
			prevByJobName.StepRegistryMetrics.Aggregated,
		),
		StepDetails: stepDetails,
	}
}

func (s StepMetricsAPI) getJobDetails(req Request) JobDetails {
	return JobDetails{
		JobName:           req.JobName,
		MultistageDetails: s.getJobMultistageDetails(req),
	}
}

func (s StepMetricsAPI) getStage(req Request) StepDetails {
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
		d.ByMultistage[multistageName] = NewStepDetail(
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
		d.StepDetails[stageName] = NewStepDetail(
			currStepRegistryMetrics.StageResults[stageName],
			prevStepRegistryMetrics.StageResults[stageName],
		)
	}

	return d
}
