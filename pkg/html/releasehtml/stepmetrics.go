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

	return stepmetricshtml.NewStepMetricsHTMLTable(report.Release, report.Timestamp).AllMultistages(resp).ToHTML()
}

func allSteps(report, reportPrev sippyprocessingv1.TestReport) string {
	api := stepmetrics.NewStepMetricsAPI(report, reportPrev)

	resp, err := api.Fetch(stepmetrics.Request{
		Release:  report.Release,
		StepName: stepmetrics.All,
	})

	if err != nil {
		klog.Error(err)
		return ""
	}

	return stepmetricshtml.NewStepMetricsHTMLTable(report.Release, report.Timestamp).AllStages(resp).ToHTML()
}
