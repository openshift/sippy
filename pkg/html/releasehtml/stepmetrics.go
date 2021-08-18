package releasehtml

import (
	"github.com/openshift/sippy/pkg/api/stepmetrics"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
	"k8s.io/klog"
)

func allMultistageJobs(report, reportPrev sippyprocessingv1.TestReport) string {
	api := stepmetrics.NewStepMetricsAPI(report, reportPrev)

	resp, err := api.Fetch(stepmetrics.Request{
		Release:           report.Release,
		MultistageJobName: stepmetrics.All,
	})

	if err != nil {
		klog.Error(err)
		return ""
	}

	table, err := stepmetricshtml.AllMultistages(resp)
	if err != nil {
		klog.Error(err)
		return ""
	}

	return table
}

func allSteps(report, reportPrev sippyprocessingv1.TestReport) string {
	api := stepmetrics.NewStepMetricsAPI(report, reportPrev)

	resp, err := api.Fetch(stepmetrics.Request{
		Release:  report.Release,
		StepName: stepmetrics.All,
	})

	table, err := stepmetricshtml.AllSteps(resp)
	if err != nil {
		klog.Error(err)
		return ""
	}

	return table
}
