package stepmetrics

import (
	"fmt"
	"net/url"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

type stepMetricsQuery struct {
	request     Request
	requestOpts RequestOpts
}

type Request struct {
	Release           string `json:"release"`
	JobName           string `json:"jobName"`
	MultistageJobName string `json:"multistageJobName"`
	StepName          string `json:"stepName"`
	Variant           string `json:"variant"`
}

type RequestOpts struct {
	Current   sippyprocessingv1.TestReport
	Previous  sippyprocessingv1.TestReport
	URLValues url.Values
}

func validateAPIRequest(curr, prev sippyprocessingv1.TestReport, req Request) error {
	q := stepMetricsQuery{
		request: req,
		requestOpts: RequestOpts{
			Current:  curr,
			Previous: prev,
		},
	}

	return q.validate()
}

func ValidateRequest(opts RequestOpts) (Request, error) {
	q := stepMetricsQuery{
		request: Request{
			Release:           opts.URLValues.Get("release"),
			JobName:           opts.URLValues.Get("jobName"),
			MultistageJobName: opts.URLValues.Get("multistageJobName"),
			StepName:          opts.URLValues.Get("stepName"),
			Variant:           opts.URLValues.Get("variant"),
		},
		requestOpts: opts,
	}

	err := q.validate()

	return q.request, err
}

func (q stepMetricsQuery) validate() error {
	if q.isJobQuery() {
		return q.validateJobQuery()
	}

	if q.request.MultistageJobName == "" && q.request.StepName == "" {
		return fmt.Errorf("missing multistage job name and step name")
	}

	if q.request.Variant != "" {
		return q.validateVariant()
	}

	if q.isMultistageQuery() {
		return q.validateMultistageQuery()
	}

	if q.isStepQuery() {
		return q.validateStepQuery()
	}

	return nil
}

func (q *stepMetricsQuery) validateMultistageQuery() error {
	if q.request.MultistageJobName != "" && q.request.MultistageJobName != All {
		if !has(
			q.requestOpts.Current.TopLevelStepRegistryMetrics.ByMultistageName,
			q.requestOpts.Previous.TopLevelStepRegistryMetrics.ByMultistageName,
			q.request.MultistageJobName) {
			return fmt.Errorf("invalid multistage job name %s", q.request.MultistageJobName)
		}
	}

	return nil
}

func (q *stepMetricsQuery) validateStepQuery() error {
	if q.request.StepName != "" && q.request.StepName != "All" {
		if !has(
			q.requestOpts.Current.TopLevelStepRegistryMetrics.ByStageName,
			q.requestOpts.Previous.TopLevelStepRegistryMetrics.ByStageName,
			q.request.StepName) {
			return fmt.Errorf("unknown step name %s", q.request.StepName)
		}
	}

	return nil
}

func (q *stepMetricsQuery) validateVariant() error {
	variants := testidentification.NewOpenshiftVariantManager().AllVariants()

	if !variants.Has(q.request.Variant) {
		return fmt.Errorf("unknown variant %s", q.request.Variant)
	}

	return nil
}

func (q *stepMetricsQuery) validateJobQuery() error {
	if !has(
		q.requestOpts.Current.TopLevelStepRegistryMetrics.ByJobName,
		q.requestOpts.Previous.TopLevelStepRegistryMetrics.ByJobName,
		q.request.JobName) {
		return fmt.Errorf("unknown job name %s", q.request.JobName)
	}

	return nil
}

func (q *stepMetricsQuery) isMultistageQuery() bool {
	return q.request.MultistageJobName != "" && q.request.StepName == ""
}

func (q *stepMetricsQuery) isJobQuery() bool {
	return q.request.JobName != ""
}

func (q *stepMetricsQuery) isStepQuery() bool {
	return q.request.StepName != "" && q.request.MultistageJobName == ""
}

func has(curr, prev interface{}, item string) bool {
	return sets.StringKeySet(curr).Union(sets.StringKeySet(prev)).Has(item)
}
