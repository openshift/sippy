package installhtml

import (
	"fmt"
	"net/http"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/generichtml"
)

var (
	upgradeTopPageHTML = `
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<style>
#table td, #table th {
	border:
}
</style>

<h1 class=text-center>Release %s Upgrade Dashboard</h1>

<p class="small mb-3 text-nowrap">
	Jump to: <a href="#UpgradeRatesByOperator">Upgrade Rates by Operator</a> | <a href="#UpgradeRelatedTests">Upgrade Related Tests</a> | <a href="#UpgradeJobs">Upgrade Jobs</a> 
</p>

`
)

func PrintUpgradeHTMLReport(w http.ResponseWriter, req *http.Request, report, prevReport sippyprocessingv1.TestReport, numDays int, release string) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, generichtml.HTMLPageStart, "Release "+release+" Upgrade Dashboard")
	if len(prevReport.AnalysisWarnings)+len(report.AnalysisWarnings) > 0 {
		warningsHTML := ""
		for _, analysisWarning := range prevReport.AnalysisWarnings {
			warningsHTML += "<p>" + analysisWarning + "</p>\n"
		}
		for _, analysisWarning := range report.AnalysisWarnings {
			warningsHTML += "<p>" + analysisWarning + "</p>\n"
		}
		fmt.Fprintf(w, generichtml.WarningHeader, warningsHTML)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, upgradeTopPageHTML, release)
	fmt.Fprintln(w)

	fmt.Fprintln(w)
	fmt.Fprint(w, UpgradeOperatorTests(HTML, report, prevReport))
	fmt.Fprintln(w)

	fmt.Fprintln(w)
	fmt.Fprint(w, summaryUpgradeRelatedTests(report, prevReport, numDays, release))
	fmt.Fprintln(w)

	fmt.Fprintln(w)
	fmt.Fprint(w, summaryUpgradeRelatedJobs(report, prevReport, numDays, release))
	fmt.Fprintln(w)

	fmt.Fprintf(w, generichtml.HTMLPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
