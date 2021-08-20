package releasehtml

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/stepmetricshtml"
)

func allMultistageJobs(report, reportPrev sippyprocessingv1.TestReport) string {
	return stepmetricshtml.AllMultistages(report, reportPrev)
}

func allSteps(report, reportPrev sippyprocessingv1.TestReport) string {
	return stepmetricshtml.AllSteps(report, reportPrev)
}
