package html

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"text/template"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

var (
	escapeRegex *regexp.Regexp = regexp.MustCompile(`\[.*?\]`)
)

const (
	BugSearchUrl = "https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search="
	up           = `<i class="fa fa-arrow-up" title="Increased %0.2f%%" style="font-size:28px;color:green"></i>`
	down         = `<i class="fa fa-arrow-down" title="Decreased %0.2f%%" style="font-size:28px;color:red"></i>`
	flatup       = `<i class="fa fa-arrows-h" title="Increased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flatdown     = `<i class="fa fa-arrows-h" title="Decreased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flat         = `<i class="fa fa-arrows-h" style="font-size:28px;color:darkgray"></i>`

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
  <h1>Warning: Bugzilla Lookup Error</h1>
  <p>At least one error was encountered looking up existing bugs for failing tests.  Some test failures may have
  associated bugs that are not listed below. Lookup error: %s</p>
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
	Jump to: <a href="#SummaryAcrossAllJobs">Summary Across All Jobs</a> | <a href="#FailureGroupings">Failure Groupings</a> | 
	         <a href="#JobPassRatesByVariant">Job Pass Rates By Variant</a> | <a href="#TopFailingTestsWithoutABug">Top Failing Tests Without a Bug</a>
	         <br> 
			 <a href="#TopFailingTestsWithABug">Top Failing Tests With a Bug</a> |
	         <a href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a> | <a href="#CanaryTestFailures">Canary Test Failures</a> |
	         <a href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a> | <a href="#TestImpactingBugs">Test Impacting Bugs</a> |
	         <a href="#TestImpactingComponents">Test Impacting Components</a>
</p>

{{ summaryAcrossAllJobs .Current.All .Prev.All .EndDay }}

{{ failureGroups .Current.FailureGroups .Prev.FailureGroups .EndDay }}

{{ summaryJobsByPlatform .Current .Prev .EndDay .JobTestCount .Release }}

{{ summaryTopFailingTests .Current.TopFailingTestsWithoutBug .Current.TopFailingTestsWithBug .Prev.All .EndDay .Release }}

{{ summaryJobPassRatesByJobName .Current .Prev .EndDay .JobTestCount }}

{{ canaryTestFailures .Current.All }}

{{ failureGroupList .Current }}

{{ testImpactingBugs .Current.BugsByFailureCount }}

{{ testImpactingComponents .Current.BugsByFailureCount }}

`

	// 1 encoded job name
	// 2 test name
	// 3 job name regex
	// 4 encoded test name
	// 5 bug list/bug search
	// 6 pass rate
	// 7 number of runs
	testGroupTemplate = `
		<tr class="collapse %s">
			<td colspan=2>
			%s
			<p>
			<a target="_blank" href="https://search.ci.openshift.org/?maxAge=168h&context=1&type=junit&maxMatches=5&maxBytes=20971520&groupBy=job&name=%[3]s&search=%[4]s">Job Search</a>
			%s
			</td>
			<td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`
)

func summaryAcrossAllJobs(result, resultPrev map[string]v1.SortedAggregateTestResult, endDay int) string {

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

func failureGroups(failureGroups, failureGroupsPrev []v1.JobRunResult, endDay int) string {

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

func summaryJobsByPlatform(report, reportPrev v1.TestReport, endDay, jobTestCount int, release string) string {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Aggregation of all job runs for a given variant, sorted by passing rate percentage.  Variants at the top of this list have unreliable CI jobs or the product is unreliable in those variants.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="JobPassRatesByVariant" href="#JobPassRatesByVariant">Job Pass Rates By Variant</a></th>
		</tr>
		<tr>
			<th>Variant</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	jobGroupTemplate := `
		<tr class="%s">
			<td>
				%[2]s
				<p>
				<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[2]s" aria-expanded="false" aria-controls="%[2]s">Expand Failing Tests</button>
			</td>
			<td>
				%0.2f%% (%0.2f%%) <span class="text-nowrap">(%d runs)</span>
			</td>
			<td>
				%s
			</td>
			<td>
				%0.2f%% (%0.2f%%) <span class="text-nowrap">(%d runs)</span>
			</td>
		</tr>
	`

	naTemplate := `
			<tr class="%s">
				<td>
					%[2]s
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[2]s" aria-expanded="false" aria-controls="%[2]s">Expand Failing Tests</button>
				</td>
				<td>
					%0.2f%% (%0.2f%%) <span class="text-nowrap">(%d runs)</span>
				</td>
				<td/>
				<td>
					NA
				</td>
			</tr>
		`

	for _, v := range jobsByPlatform {
		prev := util.GetPrevPlatform(v.Platform, jobsByPlatformPrev)
		p := v.PassPercentage
		rowColor := ""
		switch {
		case p > 75:
			rowColor = "table-success"
		case p > 30:
			rowColor = "table-warning"
		case p > 0:
			rowColor = "table-danger"
		default:
			rowColor = "error"
		}

		if prev != nil {
			pprev := prev.PassPercentage
			arrow := ""
			delta := 5.0
			if v.Successes+v.Failures > 80 {
				delta = 2
			}
			if p > pprev+delta {
				arrow = fmt.Sprintf(up, p-pprev)
			} else if p < pprev-delta {
				arrow = fmt.Sprintf(down, pprev-p)
			} else if p > pprev {
				arrow = fmt.Sprintf(flatup, p-pprev)
			} else {
				arrow = fmt.Sprintf(flatdown, pprev-p)
			}
			s = s + fmt.Sprintf(jobGroupTemplate, rowColor, v.Platform,
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
				arrow,
				prev.PassPercentage,
				prev.PassPercentageWithKnownFailures,
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate, rowColor, v.Platform,
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
			)
		}

		platformTests := report.ByPlatform[v.Platform]
		count := jobTestCount
		rowCount := 0
		rows := ""
		additionalMatches := 0
		for _, testResult := range platformTests.TestResults {
			if count == 0 {
				additionalMatches++
				continue
			}
			count--

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(testResult.Name))
			jobQuery := fmt.Sprintf("%s.*%s|%s.*%s", report.Release, v.Platform, v.Platform, report.Release)

			bugHTML := bugHTMLForTest(testResult.BugList, report.Release, "", testResult.Name)

			rows = rows + fmt.Sprintf(testGroupTemplate, strings.ReplaceAll(v.Platform, ".", ""),
				testResult.Name,
				jobQuery,
				encodedTestName,
				bugHTML,
				testResult.PassPercentage,
				testResult.Successes+testResult.Failures,
			)
			rowCount++
		}
		if additionalMatches > 0 {
			rows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2>Plus %d more tests</td></tr>`, v.Platform, additionalMatches)
		}
		if rowCount > 0 {
			s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 class="font-weight-bold">Test Name</td><td class="font-weight-bold">Test Pass Rate</td></tr>`, v.Platform)
			s = s + rows
		} else {
			s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 class="font-weight-bold">No Tests Matched Filters</td></tr>`, v.Platform)
		}
	}
	s = s + "</table>"
	return s
}

// testName is the non-encoded test.Name
func testToSearchURL(testName string) string {
	encodedTestName := url.QueryEscape(regexp.QuoteMeta(testName))
	return fmt.Sprintf("https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)
}

func summaryTopFailingTests(topFailingTestsWithoutBug, topFailingTestsWithBug []*v1.TestResult, resultPrev map[string]v1.SortedAggregateTestResult, endDay int, release string) string {
	allPrev := resultPrev["all"]

	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Most frequently failing tests without a known bug, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="TopFailingTestsWithoutABug" href="#TopFailingTestsWithoutABug">Top Failing Tests Without A Bug</a></th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Bug</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, endDay)

	template := `
		<tr>
			<td>%s</td><td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td><td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`
	naTemplate := `
		<tr>
			<td>%s</td><td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td><td/><td>NA</td>
		</tr>
	`

	for _, testResult := range topFailingTestsWithoutBug {
		// if we only have one failure, don't show it on the glass.  Keep it in the actual data so we can choose how to handle it,
		// but don't bother creating the noise in the UI for a one-off/long tail.
		if (testResult.Failures + testResult.Flakes) == 1 {
			continue
		}

		encodedTestName := url.QueryEscape(regexp.QuoteMeta(testResult.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=%s&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", release, encodedTestName, testResult.Name)
		testPrev := util.GetPrevTest(testResult.Name, allPrev.TestResults)

		bugHTML := bugHTMLForTest(testResult.BugList, release, "", testResult.Name)

		if testPrev != nil {
			arrow := ""
			delta := 5.0
			if testResult.Successes+testResult.Failures > 80 {
				delta = 2
			}

			if testResult.PassPercentage > testPrev.PassPercentage+delta {
				arrow = fmt.Sprintf(up, testResult.PassPercentage-testPrev.PassPercentage)
			} else if testResult.PassPercentage < testPrev.PassPercentage-delta {
				arrow = fmt.Sprintf(down, testPrev.PassPercentage-testResult.PassPercentage)
			} else if testResult.PassPercentage > testPrev.PassPercentage {
				arrow = fmt.Sprintf(flatup, testResult.PassPercentage-testPrev.PassPercentage)
			} else {
				arrow = fmt.Sprintf(flatdown, testPrev.PassPercentage-testResult.PassPercentage)
			}

			s += fmt.Sprintf(template, testLink, bugHTML, testResult.PassPercentage, testResult.Successes+testResult.Failures, arrow, testPrev.PassPercentage, testPrev.Successes+testPrev.Failures)
		} else {
			s += fmt.Sprintf(naTemplate, testLink, bugHTML, testResult.PassPercentage, testResult.Successes+testResult.Failures)
		}
	}

	s += fmt.Sprintf(`<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Most frequently failing tests with a known bug, sorted by passing rate." id="TopFailingTestsWithABug" href="#TopFailingTestsWithABug">Top Failing Tests With A Bug</a></th>
		  </tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>BZ</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>`, endDay)

	for _, testResult := range topFailingTestsWithBug {
		// if we only have one failure, don't show it on the glass.  Keep it in the actual data so we can choose how to handle it,
		// but don't bother creating the noise in the UI for a one-off/long tail.
		if (testResult.Failures + testResult.Flakes) == 1 {
			continue
		}

		encodedTestName := url.QueryEscape(regexp.QuoteMeta(testResult.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=%s&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", release, encodedTestName, testResult.Name)
		testPrev := util.GetPrevTest(testResult.Name, allPrev.TestResults)

		klog.V(2).Infof("processing top failing tests with bug %s, bugs: %v", testResult.Name, testResult.BugList)
		bugHTML := bugHTMLForTest(testResult.BugList, release, "", testResult.Name)
		if testPrev != nil {
			arrow := ""
			delta := 5.0
			if testResult.Successes+testResult.Failures > 80 {
				delta = 2
			}
			if testResult.PassPercentage > testPrev.PassPercentage+delta {
				arrow = up
			} else if testResult.PassPercentage < testPrev.PassPercentage-delta {
				arrow = down
			}

			if testResult.PassPercentage > testPrev.PassPercentage+delta {
				arrow = fmt.Sprintf(up, testResult.PassPercentage-testPrev.PassPercentage)
			} else if testResult.PassPercentage < testPrev.PassPercentage-delta {
				arrow = fmt.Sprintf(down, testPrev.PassPercentage-testResult.PassPercentage)
			} else if testResult.PassPercentage > testPrev.PassPercentage {
				arrow = fmt.Sprintf(flatup, testResult.PassPercentage-testPrev.PassPercentage)
			} else {
				arrow = fmt.Sprintf(flatdown, testPrev.PassPercentage-testResult.PassPercentage)
			}

			s += fmt.Sprintf(template, testLink, bugHTML, testResult.PassPercentage, testResult.Successes+testResult.Failures, arrow, testPrev.PassPercentage, testPrev.Successes+testPrev.Failures)
		} else {
			s += fmt.Sprintf(naTemplate, testLink, bugHTML, testResult.PassPercentage, testResult.Successes+testResult.Failures)
		}
	}

	s = s + "</table>"
	return s
}

func summaryJobPassRatesByJobName(report, reportPrev v1.TestReport, endDay, jobTestCount int) string {
	jobRunsByName := util.SummarizeJobsByName(report)
	jobRunsByNamePrev := util.SummarizeJobsByName(reportPrev)

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Passing rate for each job definition, sorted by passing percentage.  Jobs at the top of this list are unreliable or represent environments where the product is not stable and should be investigated.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="JobPassRatesByJobName" href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a></th>
		</tr>
		<tr>
			<th>Name</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	template := `
			<tr class="%s">
				<td>
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[4]s" aria-expanded="false" aria-controls="%[4]s">Expand Failing Tests</button>
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
				<td>
					%s
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
			</tr>
		`

	naTemplate := `
			<tr class="%s">
				<td>
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[4]s" aria-expanded="false" aria-controls="%[4]s">Expand Failing Tests</button>
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
				<td/>
				<td>
					NA
				</td>
			</tr>
		`

	for _, v := range jobRunsByName {
		prev := util.GetPrevJob(v.Name, jobRunsByNamePrev)
		rowColor := ""
		switch {
		case v.PassPercentage > 75:
			rowColor = "table-success"
		case v.PassPercentage > 30:
			rowColor = "table-warning"
		case v.PassPercentage > 0:
			rowColor = "table-danger"
		default:
			rowColor = "error"
		}

		if prev != nil {
			arrow := ""
			delta := 5.0
			if v.Successes+v.Failures > 80 {
				delta = 2
			}

			if v.PassPercentage > prev.PassPercentage+delta {
				arrow = fmt.Sprintf(up, v.PassPercentage-prev.PassPercentage)
			} else if v.PassPercentage < prev.PassPercentage-delta {
				arrow = fmt.Sprintf(down, prev.PassPercentage-v.PassPercentage)
			} else if v.PassPercentage > prev.PassPercentage {
				arrow = fmt.Sprintf(flatup, v.PassPercentage-prev.PassPercentage)
			} else {
				arrow = fmt.Sprintf(flatdown, prev.PassPercentage-v.PassPercentage)
			}
			s = s + fmt.Sprintf(template, rowColor, v.TestGridUrl, v.Name, strings.ReplaceAll(v.Name, ".", ""),
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
				arrow,
				prev.PassPercentage,
				prev.PassPercentageWithKnownFailures,
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate, rowColor, v.TestGridUrl, v.Name, strings.ReplaceAll(v.Name, ".", ""),
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
			)
		}

		jobTests := report.ByJob[v.Name]
		count := jobTestCount
		rowCount := 0
		rows := ""
		additionalMatches := 0
		for _, test := range jobTests.TestResults {
			if count == 0 {
				additionalMatches++
				continue
			}
			count--

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
			bugHTML := bugHTMLForTest(test.BugList, report.Release, "", test.Name)

			rows = rows + fmt.Sprintf(testGroupTemplate, strings.ReplaceAll(v.Name, ".", ""),
				test.Name,
				v.Name,
				encodedTestName,
				bugHTML,
				test.PassPercentage,
				test.Successes+test.Failures,
			)
			rowCount++
		}

		if additionalMatches > 0 {
			rows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2>Plus %d more tests</td></tr>`, strings.ReplaceAll(v.Name, ".", ""), additionalMatches)
		}
		if rowCount > 0 {
			s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 class="font-weight-bold">Test Name</td><td class="font-weight-bold">Test Pass Rate</td></tr>`, strings.ReplaceAll(v.Name, ".", ""))
			s = s + rows
		} else {
			s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 class="font-weight-bold">No Tests Matched Filters</td></tr>`, strings.ReplaceAll(v.Name, ".", ""))
		}

	}

	s = s + "</table>"
	return s
}

func canaryTestFailures(result map[string]v1.SortedAggregateTestResult) string {
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
func failureGroupList(report v1.TestReport) string {
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

type TestReports struct {
	Current      v1.TestReport
	Prev         v1.TestReport
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

func PrintHtmlReport(w http.ResponseWriter, req *http.Request, report, prevReport v1.TestReport, endDay, jobTestCount int) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, htmlPageStart, "Release CI Health Dashboard")

	var dashboardPage = template.Must(template.New("dashboardPage").Funcs(
		template.FuncMap{
			"summaryAcrossAllJobs":         summaryAcrossAllJobs,
			"failureGroups":                failureGroups,
			"summaryJobsByPlatform":        summaryJobsByPlatform,
			"summaryTopFailingTests":       summaryTopFailingTests,
			"summaryJobPassRatesByJobName": summaryJobPassRatesByJobName,
			"canaryTestFailures":           canaryTestFailures,
			"failureGroupList":             failureGroupList,
			"testImpactingBugs":            testImpactingBugs,
			"testImpactingComponents":      testImpactingComponents,
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
