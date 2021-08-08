package releasehtml

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/openshift/sippy/pkg/html/generichtml"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

const (
	BugSearchURL = "https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search="

	landingHTMLPageEnd = `
</div>
<p>
<script src="https://code.jquery.com/jquery-3.2.1.slim.min.js" integrity="sha384-KJ3o2DKtIkvYIK3UENzmM7KCkRr/rE9/Qpg6aAZGJwFDMVNA/GpGFF93hXpG5KkN" crossorigin="anonymous"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.12.9/umd/popper.min.js" integrity="sha384-ApNbgh9B+Y1QKtv3Rn7W3mgPxhU9K/ScQsAP7hUibX39j7fakFPskvXusvfa0b4Q" crossorigin="anonymous"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/js/bootstrap.min.js" integrity="sha384-JZR6Spejh4U02d8jOt6vLEHfe/JQGiRRSQQxSfFWpi1MquVdAyjUar5+76PVCmYl" crossorigin="anonymous"></script>
</body>
</html>
`

	dashboardPageHTML = `
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<style>
#table td, #table th {
	border: 
}
</style>

<h1 class=text-center>CI Release {{ .Release }} Health Summary</h1>

{{ releasesList .ReportNames }}

<p class="small mb-3 text-nowrap">
	Jump to: <a href="#JobPassRatesByVariant">Job Pass Rates By Variant</a> | <a href="#CuratedTRTTests">Curated TRT Tests</a> | <a href="#TopFailingTestsWithoutABug">Top Failing Tests Without a Bug</a> | <a href="#TopFailingTestsWithABug">Top Failing Tests With a Bug</a> | <a href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a> |
			 <br/>	          
	         <a href="#JobByMostReducedPassRate">Job Pass Rates By Most Reduced Pass Rate</a> | <a href="#InfrequentJobPassRatesByJobName">Infrequent Job Pass Rates By Job Name</a> | <a href="#CanaryTestFailures">Canary Test Failures</a> | <a href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a> | <a href="#TestImpactingBugs">Test Impacting Bugs</a> |
	         <br/>
             <a href="#TestImpactingComponents">Test Impacting Components</a> | <a href="#JobImpactingBZComponents">Job Impacting BZ Components</a>
</p>

{{ topLevelIndicators .Current .Prev .Release }}

{{ summaryJobsByVariant .Current .Prev .NumDays .JobTestCount .Release }}

{{ summaryCuratedTests .Current .Prev .NumDays .Release }} 

{{ summaryTopFailingTestsWithoutBug .Current.TopFailingTestsWithoutBug .Prev.ByTest .NumDays .Release }}

{{ summaryTopFailingTestsWithBug .Current.TopFailingTestsWithBug .Prev.ByTest .NumDays .Release }}

{{ summaryTopNegativelyMovingJobs .TwoDay.ByJob .Prev.ByJob .JobTestCount .Release }}

{{ summaryFrequentJobPassRatesByJobName .Current .Prev .Release .NumDays .JobTestCount }}

{{ summaryInfrequentJobPassRatesByJobName .Current .Prev .Release .NumDays .JobTestCount }}

{{ canaryTestFailures .Current.ByTest .Prev.ByTest }}

{{ failureGroupList .Current }}

{{ testImpactingBugs .Current.BugsByFailureCount }}

{{ testImpactingComponents .Current.BugsByFailureCount }}

{{ summaryJobsFailuresByBugzillaComponent .Current .Prev .NumDays .Release }}

`
)

func failureGroups(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult, endDay int) string {

	stats := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

	groups := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">
				<a class="text-dark" id="FailureGroupings" href="#FailureGroupings">Failure Groupings</a>
				<i class="fa fa-info-circle" title="Statistics on how often we see a cluster of test failures in a single run.  Such clusters are indicative of cluster infrastructure problems that impact many tests and should be investigated.  See below for a link to specific jobs that show large clusters of test failures."></i>
			</th>
		</tr>
		<tr>
			<th/><th>Latest %d days</th><th>Previous 7 days</th>
		</tr>
		<tr>
			<td>Job Runs with a Failure Group: </td><td>%d</td><td>%d</td>
		</tr>
		<tr>
			<td>Average Failure Group Size: </td><td>%d</td><td>%d</td>
		</tr>
		<tr>
			<td>Median Failure Group Size: </td><td>%d</td><td>%d</td>
		</tr>
	</table>`
	s := fmt.Sprintf(groups, endDay, len(failureGroups), len(failureGroupsPrev), stats.Avg, stats.AvgPrev, stats.Median, stats.MedianPrev)
	return s
}

func summaryJobsByVariant(report, reportPrev sippyprocessingv1.TestReport, numDays, jobTestCount int, release string) string {
	if len(report.ByVariant) == 0 {
		return ""
	}

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark"  id="JobPassRatesByVariant" href="#JobPassRatesByVariant">Job Pass Rates By Variant</a>
				<i class="fa fa-info-circle" title="Aggregation of all job runs for a given variant, sorted by passing rate percentage.  Variants at the top of this list have unreliable CI jobs or the product is unreliable in those variants.  The pass rate in parenthesis is the pass rate for jobs that started to run the installer and got at least the bootstrap kube-apiserver up and running."></i>
			</th>
		</tr>
		<tr>
			<th>Variant</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, numDays)

	for _, currVariant := range report.ByVariant {
		variantHTML := generichtml.NewJobAggregationResultRendererFromVariantResults("by-variant", currVariant, release).
			WithMaxTestResultsToShow(jobTestCount).
			WithPreviousVariantResults(util.FindVariantResultsForName(currVariant.VariantName, reportPrev.ByVariant)).
			ToHTML()

		s += variantHTML
	}

	s += "</table>"
	return s
}

func summaryFrequentJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport, release string, numDays, jobTestCount int) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark" id="JobPassRatesByJobName" href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a>
				<i class="fa fa-info-circle" title="Passing rate for each job definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the pass rate for jobs that started to run the installer and got at least the bootstrap kube-apiserver up and running."></i>
			</th>
		</tr>
		<tr>
			<th>Name</th><th>Bug</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, numDays)

	for _, currJobResult := range report.FrequentJobResults {
		prevJobResult := util.FindJobResultForJobName(currJobResult.Name, reportPrev.FrequentJobResults)
		jobHTML := generichtml.NewJobResultRendererFromJobResult("by-job-name", currJobResult, release).
			WithMaxTestResultsToShow(jobTestCount).
			WithPreviousJobResult(prevJobResult).
			ToHTML()

		s += jobHTML
	}

	s += "</table>"
	return s
}

func summaryInfrequentJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport, release string, numDays, jobTestCount int) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark" id="InfrequentJobPassRatesByJobName" href="#InfrequentJobPassRatesByJobName">Infrequent Job Pass Rates By Job Name</a>
				<i class="fa fa-info-circle" title="Passing rate for each job infrequent definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the pass rate for jobs that started to run the installer and got at least the bootstrap kube-apiserver up and running."></i>
			</th>
		</tr>
		<tr>
			<th>Name</th><th>Bug</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, numDays)

	for _, currJobResult := range report.InfrequentJobResults {
		prevJobResult := util.FindJobResultForJobName(currJobResult.Name, reportPrev.InfrequentJobResults)
		jobHTML := generichtml.NewJobResultRendererFromJobResult("by-infrequent-job-name", currJobResult, release).
			WithMaxTestResultsToShow(jobTestCount).
			WithPreviousJobResult(prevJobResult).
			ToHTML()

		s += jobHTML
	}

	s += "</table>"
	return s
}

func canaryTestFailures(all, prevAll []sippyprocessingv1.FailingTestResult) string {

	// test name | bug | pass rate | higher/lower | pass rate
	s := `
	<table class="table">
		<tr>
			<th colspan=2 class="text-center">
				<a class="text-dark" id="CanaryTestFailures" href="#CanaryTestFailures">Canary Test Failures</a>
				<i class="fa fa-info-circle" title="Tests which historically pass but failed in a job run.  Job run should be investigated because these historically stable tests were probably disrupted by a major cluster bug."></i>
			</th>
		</tr>
		<tr>
			<th>Test Name</th><th>Pass Rate</th>
		</tr>
	`
	tmpl := `
		<tr>
			<td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`

	foundCount := 0
	for i := len(all) - 1; i >= 0; i-- {
		test := all[i]
		if test.TestResultAcrossAllJobs.Failures == 0 {
			continue
		}
		foundCount++
		if foundCount > 10 {
			break
		}

		// TODO use a standard presentation for the failed test
		util.FindFailedTestResult(test.TestName, prevAll)

		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.TestName))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", encodedTestName, test.TestName)

		s += fmt.Sprintf(tmpl, testLink, test.TestResultAcrossAllJobs.PassPercentage, test.TestResultAcrossAllJobs.Successes+test.TestResultAcrossAllJobs.Failures)
	}
	s += "</table>"
	return s
}
func failureGroupList(report sippyprocessingv1.TestReport) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=2 class="text-center">
				<a class="text-dark" id="JobRunsWithFailureGroups" href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a>
				<i class="fa fa-info-circle" title="Job runs where a large number of tests failed.  This is usually indicative of a cluster infrastructure problem, not a test issue, and should be investigated as such."></i>
			</th>
		</tr>
		<tr>
			<th>Job</th><th>Failed Test Count</th>
		</tr>
	`

	tmpl := `
	<tr>
		<td><a target="_blank" href=%s>%s</a></td><td>%d</td>
	</tr>`
	for _, fg := range report.FailureGroups {
		s += fmt.Sprintf(tmpl, fg.URL, fg.Job, fg.TestFailures)
	}
	s += "</table>"
	return s
}

func testImpactingBugs(testImpactingBugs []bugsv1.Bug) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">
				<a class="text-dark" id="TestImpactingBugs" href="#TestImpactingBugs">Test Impacting Bugs</a>
				<i class="fa fa-info-circle" title="Bugs which contain references to one or more failing tests, sorted by number of times the referenced tests failed."></i>
			</th>
		</tr>
		<tr>
			<th>Bug</th><th>Failure Count</th><th>Flake Count</th>
		</tr>
	`

	for _, bug := range testImpactingBugs {
		s += fmt.Sprintf("<tr><td><a target=\"_blank\" href=%s>%d: %s</a></td><td>%d</td><td>%d</td></tr> ", bug.URL, bug.ID, bug.Summary, bug.FailureCount, bug.FlakeCount)
	}

	s += "</table>"
	return s
}

func testImpactingComponents(testImpactingBugs []bugsv1.Bug) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">
				<a class="text-dark" id="TestImpactingComponents" href="#TestImpactingComponents">Test Impacting Components</a>
				<i class="fa fa-info-circle" title="Bugzilla Components which have bugs associated with one or more test failures, with a count of how many test failures the bug(s) are associated with."></i>
			</th>
		</tr>
		<tr>
			<th>Component</th><th>Failure Count</th><th>Flake Count</th><th>Bug Count</th>
		</tr>
	`

	type Component struct {
		name         string
		bugCount     int
		failureCount int
		flakeCount   int
		bugIds       []int64
		bugURLs      []string
	}
	components := make(map[string]Component)
	for _, bug := range testImpactingBugs {
		for _, component := range bug.Component {
			if c, found := components[component]; !found {
				components[component] = Component{component, 1, bug.FailureCount, bug.FlakeCount, []int64{bug.ID}, []string{bug.URL}}
			} else {
				c.bugCount++
				c.failureCount += bug.FailureCount
				c.flakeCount += bug.FlakeCount
				c.bugURLs = append(c.bugURLs, bug.URL)
				c.bugIds = append(c.bugIds, bug.ID)
				components[component] = c
			}
		}
	}

	sorted := []Component{}
	for _, v := range components {
		sorted = append(sorted, v)
	}

	// sort highest to lowest
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].failureCount > sorted[j].failureCount
	})

	for _, c := range sorted {

		links := ""
		for i, bugURL := range c.bugURLs {
			links += fmt.Sprintf("<a target=\"_blank\" href=%s>%d</a> ", bugURL, c.bugIds[i])
		}

		s += fmt.Sprintf("<tr><td>%s</td><td>%d</td><td>%d</td><td>%d: %s</td></tr> ", c.name, c.failureCount, c.flakeCount, c.bugCount, links)
	}

	s += "</table>"
	return s
}

type TestReports struct {
	Current      sippyprocessingv1.TestReport
	TwoDay       sippyprocessingv1.TestReport
	Prev         sippyprocessingv1.TestReport
	NumDays      int
	JobTestCount int
	Release      string
	ReportNames  []string
}

func WriteLandingPage(w http.ResponseWriter, displayNames []string) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, generichtml.HTMLPageStart, "Release CI Health Dashboard")
	releaseLinks := make([]string, len(displayNames))
	for i := range displayNames {
		releaseLinks[i] = fmt.Sprintf(`<li><a href="?release=%s">release-%[1]s</a></li>`, displayNames[i])
	}
	fmt.Fprintf(w, "<h1 class='text-center'>CI Release Health Summary</h1><p><ul>%s</ul></p>", strings.Join(releaseLinks, "\n"))
	fmt.Fprint(w, landingHTMLPageEnd)
}

func PrintHTMLReport(w http.ResponseWriter, req *http.Request, report, twoDayReport, prevReport sippyprocessingv1.TestReport, numDays, jobTestCount int, allReportNames []string) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, generichtml.HTMLPageStart, "Release CI Health Dashboard")
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

	var dashboardPage = template.Must(template.New("dashboardPage").Funcs(
		template.FuncMap{
			"failureGroups":                          failureGroups,
			"summaryJobsByVariant":                   summaryJobsByVariant,
			"summaryTopFailingTestsWithBug":          summaryTopFailingTestsWithBug,
			"summaryTopFailingTestsWithoutBug":       summaryTopFailingTestsWithoutBug,
			"summaryCuratedTests":                    summaryCuratedTests,
			"summaryFrequentJobPassRatesByJobName":   summaryFrequentJobPassRatesByJobName,
			"summaryInfrequentJobPassRatesByJobName": summaryInfrequentJobPassRatesByJobName,
			"canaryTestFailures":                     canaryTestFailures,
			"failureGroupList":                       failureGroupList,
			"testImpactingBugs":                      testImpactingBugs,
			"testImpactingComponents":                testImpactingComponents,
			"summaryJobsFailuresByBugzillaComponent": summaryJobsFailuresByBugzillaComponent,
			"summaryTopNegativelyMovingJobs":         summaryTopNegativelyMovingJobs,
			"topLevelIndicators":                     topLevelIndicators,
			"releasesList":                           releasesList,
		},
	).Parse(dashboardPageHTML))

	if err := dashboardPage.Execute(w, TestReports{
		Current:      report,
		TwoDay:       twoDayReport,
		Prev:         prevReport,
		NumDays:      numDays,
		JobTestCount: jobTestCount,
		Release:      report.Release,
		ReportNames:  allReportNames,
	}); err != nil {
		klog.Errorf("Unable to render page: %v", err)
	}

	fmt.Fprintf(w, generichtml.HTMLPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
