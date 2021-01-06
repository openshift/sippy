package releasehtml

import (
	"text/template"

	"github.com/openshift/sippy/pkg/html/generichtml"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func topLevelIndicatorsHaveResults(in sippyprocessingv1.TopLevelIndicators) bool {
	if generichtml.FailingTestResultHasResults(in.Infrastructure) {
		return true
	}
	if generichtml.FailingTestResultHasResults(in.FinalOperatorHealth) {
		return true
	}
	if generichtml.FailingTestResultHasResults(in.Install) {
		return true
	}
	if generichtml.FailingTestResultHasResults(in.Upgrade) {
		return true
	}
	return false
}

func topLevelIndicators(report, reportPrev sippyprocessingv1.TestReport, release string) string {
	if !topLevelIndicatorsHaveResults(report.TopLevelIndicators) {
		return ""
	}

	tableHTML := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark"  id="TopLevelReleaseIndicators" href="#TopLevelReleaseIndicators">Top Level Release Indicators</a>
				<i class="fa fa-info-circle" title="Top level release indicators showing product health."></i>
			</th>
		</tr>
		<tr>
			<th title="How often we get to the point of running the installer.  This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close." class="text-center {{ .infraColor }}">Infrastructure</th>
			<th title="How often the install completes successfully." class="text-center {{ .installColor }}"><a href="/install?release={{ .release }}">Install</a></th>
			<th title="How often an upgrade that is started completes successfully." class="text-center {{ .upgradeColor }}"><a href="/upgrade?release={{ .release }}">Upgrade</a></th>
			<!-- Operator health at the end of a CI run provides information about how stable our operators actually are in use.  Operators can successfully install and subsequently go unhealthy.  This is a bad situation, but one that we are facing here at the end of 4.6. -->
			<!-- <th title="How often CI job runs finish with all operators healthy." class="text-center {{ .finalColor }}"><a href="/operator-health?release={{ .release }}">Operator Health</a></th> -->
		</tr>
		<tr>
			<td class="text-center {{ .infraColor }}">{{ .infraHTML }}</td>
			<td class="text-center {{ .installColor }}">{{ .installHTML }}</td>
			<td class="text-center {{ .upgradeColor }}">{{ .upgradeHTML }}</td>
			<!-- <td class="text-center {{ .finalColor }}">{{ .finalHTML }}</td> -->
		</tr>
	</table>
	`
	tableHTMLTemplate := template.Must(template.New("tableHTML").Parse(tableHTML))

	res := report.TopLevelIndicators.Infrastructure.TestResultAcrossAllJobs
	passPercent := res.PassPercentage
	total := res.Successes + res.Failures + res.Flakes
	infraColor := generichtml.OverallInstallUpgradeColors.GetColor(passPercent, total)

	res = report.TopLevelIndicators.Install.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	installColor := generichtml.OverallInstallUpgradeColors.GetColor(passPercent, total)

	res = report.TopLevelIndicators.Upgrade.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	upgradeColor := generichtml.OverallInstallUpgradeColors.GetColor(passPercent, total)

	res = report.TopLevelIndicators.FinalOperatorHealth.TestResultAcrossAllJobs
	passPercent = res.PassPercentage
	total = res.Successes + res.Failures + res.Flakes
	finalColor := generichtml.OverallInstallUpgradeColors.GetColor(passPercent, total)

	infraHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Infrastructure, &reportPrev.TopLevelIndicators.Infrastructure)
	installHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Install, &reportPrev.TopLevelIndicators.Install)
	upgradeHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.Upgrade, &reportPrev.TopLevelIndicators.Upgrade)
	finalHTML := getTopLevelIndicateFailedTestHTML(report.TopLevelIndicators.FinalOperatorHealth, &reportPrev.TopLevelIndicators.FinalOperatorHealth)

	return generichtml.MustSubstitute(tableHTMLTemplate, map[string]string{
		"release":      release,
		"infraColor":   infraColor,
		"installColor": installColor,
		"upgradeColor": upgradeColor,
		"finalColor":   finalColor,
		"infraHTML":    infraHTML,
		"installHTML":  installHTML,
		"upgradeHTML":  upgradeHTML,
		"finalHTML":    finalHTML,
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
