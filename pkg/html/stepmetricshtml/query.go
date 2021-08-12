package stepmetricshtml

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
	MultistageJobName string `json:"multistageJobName"`
	StepName          string `json:"stepName"`
	Variant           string `json:"variant"`
}

type RequestOpts struct {
	Current   sippyprocessingv1.TestReport
	Previous  sippyprocessingv1.TestReport
	URLValues url.Values
}

func ValidateRequest(opts RequestOpts) (Request, error) {
	q := stepMetricsQuery{
		request: Request{
			Release:           opts.URLValues.Get("release"),
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
		knownMultistageJobNames := getAllKeys(
			q.requestOpts.Current.TopLevelStepRegistryMetrics.ByMultistageName,
			q.requestOpts.Previous.TopLevelStepRegistryMetrics.ByMultistageName)

		if !knownMultistageJobNames.Has(q.request.MultistageJobName) {
			return fmt.Errorf("invalid multistage job name %s", q.request.MultistageJobName)
		}
	}

	return nil
}

func (q *stepMetricsQuery) validateStepQuery() error {
	if q.request.StepName != "" && q.request.StepName != "All" {
		knownStepNames := getAllKeys(
			q.requestOpts.Current.TopLevelStepRegistryMetrics.ByStageName,
			q.requestOpts.Previous.TopLevelStepRegistryMetrics.ByStageName)

		if !knownStepNames.Has(q.request.StepName) {
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

func (q *stepMetricsQuery) isMultistageQuery() bool {
	return q.request.MultistageJobName != "" && q.request.StepName == ""
}

func (q *stepMetricsQuery) isStepQuery() bool {
	return q.request.StepName != "" && q.request.MultistageJobName == ""
}

func getAllKeys(curr, prev interface{}) sets.String {
	return sets.StringKeySet(curr).Union(sets.StringKeySet(prev))
}
