package releasehtml

import (
	"strings"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func stepMetrics(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	sb := strings.Builder{}
	sb.WriteString(`
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">
				<a class="text-dark" id="StepMetrics" href="#StepMetrics">Step Metrics</a>
				<i class="fa fa-info-circle" title="Frequency of passes / failures for individual Step Registry items."></i>
			</th>
		</tr>
		<tr>
			<th>Bug</th><th>Failure Count</th><th>Flake Count</th>
		</tr>
	`)

	sb.WriteString("</table>")

	return sb.String()
}
