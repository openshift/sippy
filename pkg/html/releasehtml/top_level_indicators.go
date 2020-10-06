package releasehtml

import (
	"bytes"
	"text/template"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

var overallInstallUpgradeColors = generichtml.colorizationCriteria{
	minRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
	minYellowPercent: 85, // at risk.  In this range, there is a systemic problem that needs to be addressed.
	minGreenPercent:  90, // no action required.  TODO this should be closer to 95, but we need to ratchet there
}

func topLevelIndicators(report, reportPrev sippyprocessingv1.TestReport) string {
	tableHTML := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Top level release indicators showing platform health." id="TopLevelReleaseIndicators" href="#TopLevelReleaseIndicators">Top Level Release Indicators</a></th>
		</tr>
		<tr>
			<th title="How often we get to the point of running the installer.  This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close." class="text-center {{ .infraColor }}">Infrastructure</th>
			<th title="How often the install completes successfully." class="text-center {{ .installColor }}">Install</th>
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

	infraColor := overallInstallUpgradeColors.getColor(report.TopLevelIndicators.Infrastructure.TestResultAcrossAllJobs.PassPercentage)
	installColor := overallInstallUpgradeColors.getColor(report.TopLevelIndicators.Install.TestResultAcrossAllJobs.PassPercentage)
	upgradeColor := overallInstallUpgradeColors.getColor(report.TopLevelIndicators.Upgrade.TestResultAcrossAllJobs.PassPercentage)

	infraHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Infrastructure, &reportPrev.TopLevelIndicators.Infrastructure)
	installHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Install, &reportPrev.TopLevelIndicators.Install)
	upgradeHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Upgrade, &reportPrev.TopLevelIndicators.Upgrade)

	return mustSubstitute(tableHTMLTemplate, map[string]string{
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

	currHTML := mustSubstitute(failedTestResultTemplate, failedTestResultToFailTestTemplate(currFailingTest))
	arrow := generichtml.flatdown
	prevHTML := "NA"

	if prevFailingTest != nil {
		prevHTML = mustSubstitute(failedTestResultTemplate, failedTestResultToFailTestTemplate(*prevFailingTest))
		arrow = generichtml.getArrow(
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

func mustSubstitute(tmpl *template.Template, data interface{}) string {
	buf := &bytes.Buffer{}
	err := tmpl.Execute(buf, data)
	if err != nil {
		panic(err)
	}
	return buf.String()
}
