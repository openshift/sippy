package html

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"text/template"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

var (
	escapeRegex *regexp.Regexp = regexp.MustCompile(`\[.*?\]`)
)

const (
	BugSearchUrl = "https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search="

	htmlPageStart = `
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8"><title>%s</title>
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css">
<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
<style>
@media (max-width: 992px) {
  .container {
    width: 100%%;
    max-width: none;
  }
}

.error {
	background-color: #f5969b;
}
</style>
</head>

<body>
<div class="container">
`

	landingHtmlPageEnd = `
</div>
<p>
<script src="https://code.jquery.com/jquery-3.2.1.slim.min.js" integrity="sha384-KJ3o2DKtIkvYIK3UENzmM7KCkRr/rE9/Qpg6aAZGJwFDMVNA/GpGFF93hXpG5KkN" crossorigin="anonymous"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.12.9/umd/popper.min.js" integrity="sha384-ApNbgh9B+Y1QKtv3Rn7W3mgPxhU9K/ScQsAP7hUibX39j7fakFPskvXusvfa0b4Q" crossorigin="anonymous"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/js/bootstrap.min.js" integrity="sha384-JZR6Spejh4U02d8jOt6vLEHfe/JQGiRRSQQxSfFWpi1MquVdAyjUar5+76PVCmYl" crossorigin="anonymous"></script>
</body>
</html>
`

	htmlPageEnd = `
</div>
Data current as of: %s
<p>
<a href="https://github.com/openshift/sippy">Source Code</a>
<script src="https://code.jquery.com/jquery-3.2.1.slim.min.js" integrity="sha384-KJ3o2DKtIkvYIK3UENzmM7KCkRr/rE9/Qpg6aAZGJwFDMVNA/GpGFF93hXpG5KkN" crossorigin="anonymous"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.12.9/umd/popper.min.js" integrity="sha384-ApNbgh9B+Y1QKtv3Rn7W3mgPxhU9K/ScQsAP7hUibX39j7fakFPskvXusvfa0b4Q" crossorigin="anonymous"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/js/bootstrap.min.js" integrity="sha384-JZR6Spejh4U02d8jOt6vLEHfe/JQGiRRSQQxSfFWpi1MquVdAyjUar5+76PVCmYl" crossorigin="anonymous"></script>
</body>
</html>
`

	bugLookupWarning = `
<div  style="background-color:pink" class="jumbotron">
  <h1>Warning: Analysis Error</h1>
  <p>%s</p>
</div>
`
	dashboardPageHtml = `
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<style>
#table td, #table th {
	border: 
}
</style>

<h1 class=text-center>CI Release {{ .Release }} Health Summary</h1>

<p class="small mb-3 text-nowrap">
	Jump to: <a href="#JobPassRatesByVariant">Job Pass Rates By Variant</a> | <a href="#TopFailingTestsWithoutABug">Top Failing Tests Without a Bug</a> | <a href="#TopFailingTestsWithABug">Top Failing Tests With a Bug</a> | <a href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a> |
			 <br/>	          
	         <a href="#InfrequentJobPassRatesByJobName">Infrequent Job Pass Rates By Job Name</a> | <a href="#CanaryTestFailures">Canary Test Failures</a> | <a href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a> | <a href="#TestImpactingBugs">Test Impacting Bugs</a> |
	         <br/>
             <a href="#TestImpactingComponents">Test Impacting Components</a> | <a href="#JobImpactingBZComponents">Job Impacting BZ Components</a>
</p>

{{ summaryJobsByPlatform .Current .Prev .EndDay .JobTestCount .Release }}

{{ summaryTopFailingTestsWithoutBug .Current.TopFailingTestsWithoutBug .Prev.TopFailingTestsWithoutBug .EndDay .Release }}

{{ summaryTopFailingTestsWithBug .Current.TopFailingTestsWithBug .Prev.TopFailingTestsWithBug .EndDay .Release }}

{{ summaryJobPassRatesByJobName .Current .Prev .Release .EndDay .JobTestCount }}

{{ summaryInfrequentJobPassRatesByJobName .Current .Prev .Release .EndDay .JobTestCount }}

{{ canaryTestFailures .Current.All }}

{{ failureGroupList .Current }}

{{ testImpactingBugs .Current.BugsByFailureCount }}

{{ testImpactingComponents .Current.BugsByFailureCount }}

{{ summaryJobsFailuresByBugzillaComponent .Current .Prev .EndDay .Release }}

`
)

func summaryAcrossAllJobs(result, resultPrev map[string]sippyprocessingv1.SortedAggregateTestsResult, endDay int) string {

	all := result["all"]
	allPrev := resultPrev["all"]

	summary := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center"><a class="text-dark" id="SummaryAcrossAllJobs" href="#SummaryAcrossAllJobs">Summary Across All Jobs</a></th>			
		</tr>
		<tr>
			<th/><th>Latest %d days</th><th>Previous 7 days</th>
		</tr>
		<tr>
			<td>Test executions: </td><td>%d</td><td>%d</td>
		</tr>
		<tr>
			<td>Test Pass Percentage: </td><td>%0.2f</td><td>%0.2f</td>
		</tr>
	</table>`
	s := fmt.Sprintf(summary, endDay, all.Successes+all.Failures, allPrev.Successes+allPrev.Failures, all.TestPassPercentage, allPrev.TestPassPercentage)
	return s
}

func failureGroups(failureGroups, failureGroupsPrev []sippyprocessingv1.JobRunResult, endDay int) string {

	_, _, median, medianPrev, avg, avgPrev := util.ComputeFailureGroupStats(failureGroups, failureGroupsPrev)

	groups := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center"><a class="text-dark" title="Statistics on how often we see a cluster of test failures in a single run.  Such clusters are indicative of cluster infrastructure problems that impact many tests and should be investigated.  See below for a link to specific jobs that show large clusters of test failures."  id="FailureGroupings" href="#FailureGroupings">Failure Groupings</a></th>
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
	s := fmt.Sprintf(groups, endDay, len(failureGroups), len(failureGroupsPrev), avg, avgPrev, median, medianPrev)
	return s
}

func summaryJobsByPlatform(report, reportPrev sippyprocessingv1.TestReport, endDay, jobTestCount int, release string) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Aggregation of all job runs for a given variant, sorted by passing rate percentage.  Variants at the top of this list have unreliable CI jobs or the product is unreliable in those variants.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="JobPassRatesByVariant" href="#JobPassRatesByVariant">Job Pass Rates By Variant</a></th>
		</tr>
		<tr>
			<th>Variant</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	for _, currPlatform := range report.ByPlatform {
		jobHTML := newJobAggregationResultRenderer("by-variant", *convertPlatformToAggregationResult(&currPlatform), release).
			withMaxTestResultsToShow(jobTestCount).
			withPrevious(convertPlatformToAggregationResult(util.GetPlatform(currPlatform.PlatformName, reportPrev.ByPlatform))).
			toHTML()

		s += jobHTML
	}

	s = s + "</table>"
	return s
}

// testName is the non-encoded test.Name
func testToSearchURL(testName string) string {
	encodedTestName := url.QueryEscape(regexp.QuoteMeta(testName))
	return fmt.Sprintf("https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)
}

func summaryJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport, release string, endDay, jobTestCount int) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Passing rate for each job definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="JobPassRatesByJobName" href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a></th>
		</tr>
		<tr>
			<th>Name</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	for _, currJobResult := range report.JobResults {
		prevJobResult := util.GetJobResultForJobName(currJobResult.Name, reportPrev.JobResults)
		jobHTML := newJobResultRenderer("by-job-name", currJobResult, release).
			withMaxTestResultsToShow(jobTestCount).
			withPrevious(prevJobResult).
			toHTML()

		s += jobHTML
	}

	s = s + "</table>"
	return s
}

func summaryInfrequentJobPassRatesByJobName(report, reportPrev sippyprocessingv1.TestReport, release string, endDay, jobTestCount int) string {
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Passing rate for each job infrequent definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="InfrequentJobPassRatesByJobName" href="#InfrequentJobPassRatesByJobName">Infrequent Job Pass Rates By Job Name</a></th>
		</tr>
		<tr>
			<th>Name</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	for _, currJobResult := range report.InfrequentJobResults {
		prevJobResult := util.GetJobResultForJobName(currJobResult.Name, reportPrev.InfrequentJobResults)
		jobHTML := newJobResultRenderer("by-infrequent-job-name", currJobResult, release).
			withMaxTestResultsToShow(jobTestCount).
			withPrevious(prevJobResult).
			toHTML()

		s += jobHTML
	}

	s = s + "</table>"
	return s
}

func canaryTestFailures(result map[string]sippyprocessingv1.SortedAggregateTestsResult) string {
	all := result["all"].TestResults

	// test name | bug | pass rate | higher/lower | pass rate
	s := `
	<table class="table">
		<tr>
			<th colspan=2 class="text-center"><a class="text-dark" title="Tests which historically pass but failed in a job run.  Job run should be investigated because these historically stable tests were probably disrupted by a major cluster bug." id="CanaryTestFailures" href="#CanaryTestFailures">Canary Test Failures</a></th>
		</tr>
		<tr>
			<th>Test Name</th><th>Pass Rate</th>
		</tr>
	`
	template := `
		<tr>
			<td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`

	for i := len(all) - 1; i >= 0 && i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", encodedTestName, test.Name)

		s += fmt.Sprintf(template, testLink, test.PassPercentage, test.Successes+test.Failures)
	}
	s = s + "</table>"
	return s
}
func failureGroupList(report sippyprocessingv1.TestReport) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=2 class="text-center"><a class="text-dark" title="Job runs where a large number of tests failed.  This is usually indicative of a cluster infrastructure problem, not a test issue, and should be investigated as such." id="JobRunsWithFailureGroups" href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a></th>
		</tr>
		<tr>
			<th>Job</th><th>Failed Test Count</th>
		</tr>
	`

	template := `
	<tr>
		<td><a target="_blank" href=%s>%s</a></td><td>%d</td>
	</tr>`
	for _, fg := range report.FailureGroups {
		s += fmt.Sprintf(template, fg.Url, fg.Job, fg.TestFailures)
	}
	s = s + "</table>"
	return s
}

func testImpactingBugs(testImpactingBugs []bugsv1.Bug) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center"><a class="text-dark" title="Bugs which contain references to one or more failing tests, sorted by number of times the referenced tests failed." id="TestImpactingBugs" href="#TestImpactingBugs">Test Impacting Bugs</a></th>
		</tr>
		<tr>
			<th>Bug</th><th>Failure Count</th><th>Flake Count</th>
		</tr>
	`

	for _, bug := range testImpactingBugs {
		s += fmt.Sprintf("<tr><td><a target=\"_blank\" href=%s>%d: %s</a></td><td>%d</td><td>%d</td></tr> ", bug.Url, bug.ID, bug.Summary, bug.FailureCount, bug.FlakeCount)
	}

	s = s + "</table>"
	return s
}

func testImpactingComponents(testImpactingBugs []bugsv1.Bug) string {
	s := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Bugzilla Components which have bugs associated with one or more test failures, with a count of how many test failures the bug(s) are associated with." id="TestImpactingComponents" href="#TestImpactingComponents">Test Impacting Components</a></th>
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
		bugUrls      []string
	}
	components := make(map[string]Component)
	for _, bug := range testImpactingBugs {
		for _, component := range bug.Component {
			if c, found := components[component]; !found {
				components[component] = Component{component, 1, bug.FailureCount, bug.FlakeCount, []int64{bug.ID}, []string{bug.Url}}
			} else {
				c.bugCount++
				c.failureCount += bug.FailureCount
				c.flakeCount += bug.FlakeCount
				c.bugUrls = append(c.bugUrls, bug.Url)
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
		for i, url := range c.bugUrls {
			links += fmt.Sprintf("<a target=\"_blank\" href=%s>%d</a> ", url, c.bugIds[i])
		}

		s += fmt.Sprintf("<tr><td>%s</td><td>%d</td><td>%d</td><td>%d: %s</td></tr> ", c.name, c.failureCount, c.flakeCount, c.bugCount, links)
	}

	s = s + "</table>"
	return s
}

func summaryJobsFailuresByBugzillaComponent(report, reportPrev sippyprocessingv1.TestReport, endDay int, release string) string {
	failuresByBugzillaComponent := util.SummarizeJobsFailuresByBugzillaComponent(report)
	failuresByBugzillaComponentPrev := util.SummarizeJobsFailuresByBugzillaComponent(reportPrev)

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Bugzilla components ranked by maximum fail percentage of any job" id="JobImpactingBZComponents" href="#JobImpactingBZComponents">Job Impacting BZ Components</a></th>
		</tr>
		<tr>
			<th>Component</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	bzGroupTemplate := `
		<tr class="%s">
			<td>
				%[2]s
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[3]s" aria-expanded="false" aria-controls="%[3]s">Expand Failing Jobs</button>
			</td>
			<td>
				%0.2f%% <span class="text-nowrap">(%d runs)</span>
			</td>
			<td>
				%s
			</td>
			<td>
				%0.2f%% <span class="text-nowrap">(%d runs)</span>
			</td>
		</tr>
	`

	naTemplate := `
			<tr class="%s">
				<td>
					%[2]s
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[3]s" aria-expanded="false" aria-controls="%[3]s">Expand Failing Jobs</button>
				</td>
				<td>
					%0.2f%% <span class="text-nowrap">(%d runs)</span>
				</td>
				<td/>
				<td>
					NA
				</td>
			</tr>
		`

	colors := colorizationCriteria{
		minRedPercent:    0,
		minYellowPercent: 90,
		minGreenPercent:  95,
	}

	for _, v := range failuresByBugzillaComponent {
		safeBZJob := fmt.Sprintf("%s---component", v.Name)
		safeBZJob = strings.ReplaceAll(safeBZJob, ".", "")
		safeBZJob = strings.ReplaceAll(safeBZJob, " ", "")

		prev := util.GetPrevBugzillaJobFailures(v.Name, failuresByBugzillaComponentPrev)
		highestFailPercentage := v.JobsFailed[0].FailPercentage
		lowestPassPercentage := 100 - highestFailPercentage
		rowColor := ""
		switch {
		case lowestPassPercentage > colors.minGreenPercent:
			rowColor = "table-success"
		case lowestPassPercentage > colors.minYellowPercent:
			rowColor = "table-warning"
		case lowestPassPercentage > colors.minRedPercent:
			rowColor = "table-danger"
		default:
			rowColor = "error"
		}

		if prev != nil && len(prev.JobsFailed) > 0 {
			previousHighestFailPercentage := prev.JobsFailed[0].FailPercentage
			previousLowestPassPercentage := 100 - previousHighestFailPercentage

			arrow := getArrow(v.JobsFailed[0].TotalRuns, lowestPassPercentage, previousLowestPassPercentage)

			s = s + fmt.Sprintf(bzGroupTemplate,
				rowColor,
				v.Name,
				safeBZJob,
				lowestPassPercentage,
				v.JobsFailed[0].TotalRuns, // this is the total runs for the current, worst job which matches the pass percentage
				arrow,
				previousLowestPassPercentage,
				prev.JobsFailed[0].TotalRuns,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate,
				rowColor,
				v.Name,
				safeBZJob,
				lowestPassPercentage,
				v.JobsFailed[0].TotalRuns, // this is the total runs for the current, worst job which matches the pass percentage
			)
		}

		count := 0
		for _, failingJob := range v.JobsFailed {
			if count > 4 { // only show five
				break
			}
			count++

			bzJobTuple := fmt.Sprintf("%s---%s", v.Name, failingJob.JobName)
			bzJobTuple = strings.ReplaceAll(bzJobTuple, ".", "")
			bzJobTuple = strings.ReplaceAll(bzJobTuple, " ", "")

			// given the name, we can actually look up the original JobResult.  There aren't that many, just iterate.
			fullJobResult := util.GetJobResultForJobName(failingJob.JobName, report.JobResults)

			// create the synthetic JobResult for display purposes.
			// TODO with another refactor, we'll be able to tighten this up later.
			currJobResult := sippyprocessingv1.JobResult{
				Name:                            failingJob.JobName,
				Platform:                        "",
				Failures:                        failingJob.NumberOfJobRunsFailed,
				KnownFailures:                   0,
				Successes:                       failingJob.TotalRuns - failingJob.NumberOfJobRunsFailed,
				PassPercentage:                  100.0 - failingJob.FailPercentage,
				PassPercentageWithKnownFailures: 0,
				TestGridUrl:                     fullJobResult.TestGridUrl,
				TestResults:                     failingJob.Failures,
			}
			var prevJobResult *sippyprocessingv1.JobResult
			if prev != nil {
				var prevJob *sippyprocessingv1.BugzillaJobResult
				for _, prevJobI := range prev.JobsFailed {
					if prevJobI.JobName == failingJob.JobName {
						prevJob = &prevJobI
						break
					}
				}
				if prevJob != nil {
					prevJobResult = &sippyprocessingv1.JobResult{
						Name:                            prevJob.JobName,
						Platform:                        "",
						Failures:                        prevJob.NumberOfJobRunsFailed,
						KnownFailures:                   0,
						Successes:                       prevJob.TotalRuns - prevJob.NumberOfJobRunsFailed,
						PassPercentage:                  100.0 - prevJob.FailPercentage,
						PassPercentageWithKnownFailures: 0,
						TestGridUrl:                     fullJobResult.TestGridUrl,
						TestResults:                     prevJob.Failures,
					}
				}
			}

			jobHTML := newJobResultRenderer("by-bz-component-"+bzJobTuple, currJobResult, release).
				withPrevious(prevJobResult).
				withColors(colors).
				startCollapsedAs(safeBZJob).
				withIndent(1).
				toHTML()
			s += jobHTML
		}
	}
	s = s + "</table>"
	return s
}

type TestReports struct {
	Current      sippyprocessingv1.TestReport
	Prev         sippyprocessingv1.TestReport
	EndDay       int
	JobTestCount int
	Release      string
}

func WriteLandingPage(w http.ResponseWriter, releases []string) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, htmlPageStart, "Release CI Health Dashboard")
	releaseLinks := make([]string, len(releases))
	for i := range releases {
		releaseLinks[i] = fmt.Sprintf(`<li><a href="?release=%s">release-%[1]s</a></li>`, releases[i])
	}
	fmt.Fprintf(w, "<h1 class='text-center'>CI Release Health Summary</h1><p><ul>%s</ul></p>", strings.Join(releaseLinks, "\n"))
	fmt.Fprintf(w, landingHtmlPageEnd)
}

func PrintHtmlReport(w http.ResponseWriter, req *http.Request, report, prevReport sippyprocessingv1.TestReport, endDay, jobTestCount int) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, htmlPageStart, "Release CI Health Dashboard")
	for _, analysisWarning := range prevReport.AnalysisWarnings {
		fmt.Fprintf(w, bugLookupWarning, analysisWarning)
	}
	for _, analysisWarning := range report.AnalysisWarnings {
		fmt.Fprintf(w, bugLookupWarning, analysisWarning)
	}

	var dashboardPage = template.Must(template.New("dashboardPage").Funcs(
		template.FuncMap{
			"summaryAcrossAllJobs":                   summaryAcrossAllJobs,
			"failureGroups":                          failureGroups,
			"summaryJobsByPlatform":                  summaryJobsByPlatform,
			"summaryTopFailingTestsWithBug":          summaryTopFailingTestsWithBug,
			"summaryTopFailingTestsWithoutBug":       summaryTopFailingTestsWithoutBug,
			"summaryJobPassRatesByJobName":           summaryJobPassRatesByJobName,
			"summaryInfrequentJobPassRatesByJobName": summaryInfrequentJobPassRatesByJobName,
			"canaryTestFailures":                     canaryTestFailures,
			"failureGroupList":                       failureGroupList,
			"testImpactingBugs":                      testImpactingBugs,
			"testImpactingComponents":                testImpactingComponents,
			"summaryJobsFailuresByBugzillaComponent": summaryJobsFailuresByBugzillaComponent,
		},
	).Parse(dashboardPageHtml))

	if err := dashboardPage.Execute(w, TestReports{
		Current:      report,
		Prev:         prevReport,
		EndDay:       endDay,
		JobTestCount: jobTestCount,
		Release:      report.Release,
	}); err != nil {
		klog.Errorf("Unable to render page: %v", err)
	}

	//w.Write(result)
	fmt.Fprintf(w, htmlPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
