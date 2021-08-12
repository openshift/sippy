package stepmetricshtml

import (
	"fmt"
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

type StepMetricsHTTPQuery struct {
	SippyURL
	Current  sippyprocessingv1.TestReport
	Previous sippyprocessingv1.TestReport
}

func NewStepMetricsHTTPQuery(req *http.Request) StepMetricsHTTPQuery {
	return StepMetricsHTTPQuery{
		SippyURL: SippyURL{
			Release:           req.URL.Query().Get("release"),
			MultistageJobName: req.URL.Query().Get("multistageJobName"),
			StepName:          req.URL.Query().Get("stepName"),
			Variant:           req.URL.Query().Get("variant"),
		},
	}
}

func (q StepMetricsHTTPQuery) Validate(knownReleases sets.String) error {
	if q.Release == "" {
		return fmt.Errorf("missing release")
	}

	if q.MultistageJobName == "" && q.StepName == "" {
		return fmt.Errorf("missing multistage job name and step name")
	}

	if !knownReleases.Has(q.Release) {
		return fmt.Errorf("invalid release %s", q.Release)
	}

	if q.Variant != "" {
		return q.validateVariant()
	}

	return nil
}

func (q *StepMetricsHTTPQuery) ValidateFromReports(curr, prev sippyprocessingv1.TestReport) error {
	q.Current = curr
	q.Previous = prev

	if q.isMultistageQuery() {
		return q.validateMultistageQuery()
	}

	if q.isStepQuery() {
		return q.validateStepQuery()
	}

	return nil
}

func (q *StepMetricsHTTPQuery) validateMultistageQuery() error {
	if q.MultistageJobName != "" && q.MultistageJobName != "All" {
		knownMultistageJobNames := sets.StringKeySet(q.Current.TopLevelStepRegistryMetrics.ByMultistageName)

		if !knownMultistageJobNames.Has(q.MultistageJobName) {
			return fmt.Errorf("invalid multistage job name %s", q.MultistageJobName)
		}
	}

	return nil
}

func (q *StepMetricsHTTPQuery) validateStepQuery() error {
	if q.StepName != "" && q.StepName != "All" {
		knownStepNames := sets.StringKeySet(q.Current.TopLevelStepRegistryMetrics.ByStageName)

		if !knownStepNames.Has(q.StepName) {
			return fmt.Errorf("unknown step name %s", q.StepName)
		}
	}

	return nil
}

func (q *StepMetricsHTTPQuery) validateVariant() error {
	variants := testidentification.NewOpenshiftVariantManager().AllVariants()

	if !variants.Has(q.Variant) {
		return fmt.Errorf("unknown variant %s", q.Variant)
	}

	return nil
}

func (q *StepMetricsHTTPQuery) isMultistageQuery() bool {
	return q.MultistageJobName != "" && q.StepName == ""
}

func (q *StepMetricsHTTPQuery) isStepQuery() bool {
	return q.StepName != "" && q.MultistageJobName == ""
}
