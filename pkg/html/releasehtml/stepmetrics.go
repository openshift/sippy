package releasehtml

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"k8s.io/klog"
)

func getStepMetricsByName(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	htmlTemplate := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">
				<a class="text-dark" id="StepMetrics" href="#StepMetrics">Step Metrics By Stage Name</a>
				<i class="fa fa-info-circle" title="Frequency of passes / failures for Step Registry items, grouped by stage name."></i>
			</th>
		</tr>
		<tr>
			<th>Name</th>
			<th>Latest 7 days</th>
			<th>Previous 7 days</th>
		</tr>
		{{ range $stageName, $result := .TopLevelStepRegistryMetrics.ByStageName }}
		<tr>
			<td>
				<td><a href="https://steps.ci.openshift.org/reference/{{.Name}}">{{.Name}}</a></td>
			</td>
			<td>{{printf "%.2f" .PassPercentage}}% (Overall: {{runs .}}, Passes: {{.Successes}}, Failures: {{.Failures}}, Flakes: {{.Flakes}})</td>
			<td></td>
		</tr>
		{{ end }}
	</table>
	`

	tmplFuncs := template.FuncMap{
		"runs": runs,
	}

	stepMetricsTemplate := template.Must(template.New("step-metric-template").Funcs(tmplFuncs).Parse(htmlTemplate))

	out := bytes.NewBuffer([]byte{})
	if err := stepMetricsTemplate.Execute(out, report); err != nil {
		klog.Error(fmt.Errorf("could not render step metrics template: %w", err))
		return ""
	}

	return out.String()
}

func getStepMetricsByMultistageNameAndStageName(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	htmlTemplate := `
	{{ range $multistageName, $result := .TopLevelStepRegistryMetrics.ByMultistageName }}
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">
				<a class="text-dark" id="StepMetricsMultistage-{{$multistageName}}" href="#StepMetricsMultistage{{$multistageName}}">Multistage {{$multistageName}}</a>
				<i class="fa fa-info-circle" title="Frequency of passes / failures for Step Registry items for multistage {{$multistageName}}."></i>
			</th>
		</tr>
		<tr>
			<th>Name</th>
			<th>Latest 7 days</th>
			<th>Previous 7 days</th>
		</tr>
		{{ range $stageName, $stageRresult := $result }}
		<tr>
		<td><a href="https://steps.ci.openshift.org/reference/{{.Name}}">{{.Name}}</a></td>
			<td>{{printf "%.2f" .PassPercentage}}% (Overall: {{runs .}}, Passes: {{.Successes}}, Failures: {{.Failures}}, Flakes: {{.Flakes}})</td>
			<td></td>
		</tr>
		{{ end }}
	</table>
	{{ end }}
	`

	tmplFuncs := template.FuncMap{
		"runs": runs,
	}

	stepMetricsTemplate := template.Must(template.New("step-metric-template").Funcs(tmplFuncs).Parse(htmlTemplate))

	out := bytes.NewBuffer([]byte{})
	if err := stepMetricsTemplate.Execute(out, report); err != nil {
		klog.Error(fmt.Errorf("could not render step metrics template: %w", err))
		return ""
	}

	return out.String()
}

func runs(stageResult sippyprocessingv1.StageResult) int {
	return stageResult.Successes + stageResult.Failures + stageResult.Flakes
}

func stepMetrics(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	sb := strings.Builder{}
	sb.WriteString(getStepMetricsByName(report, reportPrev, numDays, release))
	sb.WriteString("\n")
	sb.WriteString(getStepMetricsByMultistageNameAndStageName(report, reportPrev, numDays, release))
	return sb.String()
}
