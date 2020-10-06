package releasehtml

import (
	"text/template"

	"github.com/openshift/sippy/pkg/html/generichtml"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func topLevelIndicators(report, reportPrev sippyprocessingv1.TestReport, release string) string {
	tableHTML := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Top level release indicators showing platform health." id="TopLevelReleaseIndicators" href="#TopLevelReleaseIndicators">Top Level Release Indicators</a></th>
		</tr>
		<tr>
			<th title="How often we get to the point of running the installer.  This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close." class="text-center {{ .infraColor }}">Infrastructure</th>
			<th title="How often the install completes successfully." class="text-center {{ .installColor }}"><a href="/install?release={{ .release }}">Install</a></th>
			<th title="How often an upgrade that is started completes successfully." class="text-center {{ .upgradeColor }}">Upgrade</th>
		</tr>
		<tr>
			<td class="text-center {{ .infraColor }}">{{ .infraHTML }}</td>
			<td class="text-center {{ .installColor }}">{{ .installHTML }}</td>
			<td class="text-center {{ .upgradeColor }}">{{ .upgradeHTML }}</td>
		</tr>
	</table>
	`
	tableHTMLTemplate := template.Must(template.New("tableHTML").Parse(tableHTML))

	infraColor := generichtml.OverallInstallUpgradeColors.GetColor(report.TopLevelIndicators.Infrastructure.TestResultAcrossAllJobs.PassPercentage)
	installColor := generichtml.OverallInstallUpgradeColors.GetColor(report.TopLevelIndicators.Install.TestResultAcrossAllJobs.PassPercentage)
	upgradeColor := generichtml.OverallInstallUpgradeColors.GetColor(report.TopLevelIndicators.Upgrade.TestResultAcrossAllJobs.PassPercentage)

	infraHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Infrastructure, &reportPrev.TopLevelIndicators.Infrastructure)
	installHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Install, &reportPrev.TopLevelIndicators.Install)
	upgradeHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Upgrade, &reportPrev.TopLevelIndicators.Upgrade)

	return generichtml.MustSubstitute(tableHTMLTemplate, map[string]string{
		"release":      release,
		"infraColor":   infraColor,
		"installColor": installColor,
		"upgradeColor": upgradeColor,
		"infraHTML":    infraHTML,
		"installHTML":  installHTML,
		"upgradeHTML":  upgradeHTML,
	})
}

func getTopLevelIndicateFailedTestHTML(currFailingTest sippyprocessingv1.FailingTestResult, prevFailingTest *sippyprocessingv1.FailingTestResult) string {
	failedTestResultTemplateString := `{{ printf "%.2f" .PassPercentage }}% ({{ .TotalRuns }} runs)`
	failedTestResultTemplate := template.Must(template.New("failed-test-result").Parse(failedTestResultTemplateString))

	currHTML := generichtml.MustSubstitute(failedTestResultTemplate, failedTestResultToFailTestTemplate(currFailingTest))
	arrow := generichtml.Flat
	prevHTML := "NA"

	if prevFailingTest != nil {
		prevHTML = generichtml.MustSubstitute(failedTestResultTemplate, failedTestResultToFailTestTemplate(*prevFailingTest))
		arrow = generichtml.GetArrow(
			currFailingTest.TestResultAcrossAllJobs.Successes+currFailingTest.TestResultAcrossAllJobs.Failures,
			currFailingTest.TestResultAcrossAllJobs.PassPercentage,
			prevFailingTest.TestResultAcrossAllJobs.PassPercentage)
	}

	return currHTML + " " + arrow + " " + prevHTML
}

type failTestTemplate struct {
	TotalRuns      int
	PassPercentage float64
}

func failedTestResultToFailTestTemplate(in sippyprocessingv1.FailingTestResult) failTestTemplate {
	return failTestTemplate{
		TotalRuns:      in.TestResultAcrossAllJobs.Successes + in.TestResultAcrossAllJobs.Failures,
		PassPercentage: in.TestResultAcrossAllJobs.PassPercentage,
	}
}
