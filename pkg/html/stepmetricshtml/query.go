package stepmetricshtml

import (
	"fmt"
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

type StepMetricsHTTPQuery struct {
	request Request
}

type Request struct {
	Release           string
	MultistageJobName string
	StepName          string
	Variant           string
}

func NewStepMetricsHTTPQuery(req *http.Request) StepMetricsHTTPQuery {
	return StepMetricsHTTPQuery{
		request: Request{
			Release:           req.URL.Query().Get("release"),
			MultistageJobName: req.URL.Query().Get("multistageJobName"),
			StepName:          req.URL.Query().Get("stepName"),
			Variant:           req.URL.Query().Get("variant"),
		},
	}
}

func (q StepMetricsHTTPQuery) Validate(knownReleases sets.String) error {
	if q.request.Release == "" {
		return fmt.Errorf("missing release")
	}

	if q.request.MultistageJobName == "" && q.request.StepName == "" {
		return fmt.Errorf("missing multistage job name and step name")
	}

	if !knownReleases.Has(q.request.Release) {
		return fmt.Errorf("invalid release %s", q.request.Release)
	}

	if q.request.Variant != "" {
		return q.validateVariant()
	}

	return nil
}

func (q *StepMetricsHTTPQuery) ValidateFromReports(curr, prev sippyprocessingv1.TestReport) error {
	if q.isMultistageQuery() {
		return q.validateMultistageQuery(curr, prev)
	}

	if q.isStepQuery() {
		return q.validateStepQuery(curr, prev)
	}

	return nil
}

func (q StepMetricsHTTPQuery) Request() Request {
	return Request{
		Release:           q.request.Release,
		MultistageJobName: q.request.MultistageJobName,
		StepName:          q.request.StepName,
		Variant:           q.request.Variant,
	}
}

func (q *StepMetricsHTTPQuery) validateMultistageQuery(curr, prev sippyprocessingv1.TestReport) error {
	if q.request.MultistageJobName != "" && q.request.MultistageJobName != "All" {
		knownMultistageJobNames := sets.StringKeySet(curr.TopLevelStepRegistryMetrics.ByMultistageName)
		knownMultistageJobNames = knownMultistageJobNames.Union(sets.StringKeySet(prev.TopLevelStepRegistryMetrics.ByMultistageName))

		if !knownMultistageJobNames.Has(q.request.MultistageJobName) {
			return fmt.Errorf("invalid multistage job name %s", q.request.MultistageJobName)
		}
	}

	return nil
}

func (q *StepMetricsHTTPQuery) validateStepQuery(curr, prev sippyprocessingv1.TestReport) error {
	if q.request.StepName != "" && q.request.StepName != "All" {
		knownStepNames := sets.StringKeySet(curr.TopLevelStepRegistryMetrics.ByStageName)
		knownStepNames = knownStepNames.Union(sets.StringKeySet(prev.TopLevelStepRegistryMetrics.ByStageName))

		if !knownStepNames.Has(q.request.StepName) {
			return fmt.Errorf("unknown step name %s", q.request.StepName)
		}
	}

	return nil
}

func (q *StepMetricsHTTPQuery) validateVariant() error {
	variants := testidentification.NewOpenshiftVariantManager().AllVariants()

	if !variants.Has(q.request.Variant) {
		return fmt.Errorf("unknown variant %s", q.request.Variant)
	}

	return nil
}

func (q *StepMetricsHTTPQuery) isMultistageQuery() bool {
	return q.request.MultistageJobName != "" && q.request.StepName == ""
}

func (q *StepMetricsHTTPQuery) isStepQuery() bool {
	return q.request.StepName != "" && q.request.MultistageJobName == ""
}
