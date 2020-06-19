package html

import (
	"fmt"

	//"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	"k8s.io/klog"

	"github.com/openshift/sippy/pkg/util"
)

var (
	escapeRegex *regexp.Regexp = regexp.MustCompile(`\[.*?\]`)
)

const (
	up       = `<i class="fa fa-arrow-up" title="Increased %0.2f%%" style="font-size:28px;color:green"></i>`
	down     = `<i class="fa fa-arrow-down" title="Decreased %0.2f%%" style="font-size:28px;color:red"></i>`
	flatup   = `<i class="fa fa-arrows-h" title="Increased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flatdown = `<i class="fa fa-arrows-h" title="Decreased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flat     = `<i class="fa fa-arrows-h" style="font-size:28px;color:darkgray"></i>`

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

	dashboardPageHtml = `
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<style>
#table td, #table th {
	border: 
}
</style>

<h1 class=text-center>CI Release Health Summary</h1>

<p class="small mb-3 text-nowrap">
	Jump to: <a href="#SummaryAcrossAllJobs">Summary Across All Jobs</a> | <a href="#FailureGroupings">Failure Groupings</a> | 
	         <a href="#JobPassRatesByPlatform">Job Pass Rates By Platform</a> | <a href="#TopFailingTests">Top Failing Tests</a>
	         <br> 
	         <a href="#JobPassRatesByJobName">Job Pass Rates By Job Name</a> | <a href="#CanaryTestFailures">Canary Test Failures</a> |
	         <a href="#JobRunsWithFailureGroups">Job Runs With Failure Groups</a> | <a href="#TestImpactingBugs">Test Impacting Bugs</a>
</p>

{{ summaryAcrossAllJobs .Current.All .Prev.All .EndDay }}

{{ failureGroups .Current.FailureGroups .Prev.FailureGroups .EndDay }}

{{ summaryJobsByPlatform .Current .Prev .EndDay .JobTestCount }}

{{ summaryTopFailingTests .Current.TopFailingTestsWithoutBug .Current.TopFailingTestsWithBug .Prev.All .EndDay }}

{{ summaryJobPassRatesByJobName .Current .Prev .EndDay .JobTestCount }}

{{ canaryTestFailures .Current.All }}

{{ failureGroupList .Current }}

{{ testImpactingBugs .Current.BugsByFailureCount }}
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
			<a target="_blank" href="https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=junit&maxMatches=5&maxBytes=20971520&groupBy=job&name=%[3]s&search=%[4]s">Job Search</a>
			%s
			</td>
			<td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`
)

func summaryAcrossAllJobs(result, resultPrev map[string]util.SortedAggregateTestResult, endDay int) string {

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

func failureGroups(failureGroups, failureGroupsPrev []util.JobRunResult, endDay int) string {
	count, countPrev, median, medianPrev, avg, avgPrev := 0, 0, 0, 0, 0, 0
	for _, group := range failureGroups {
		count += group.TestFailures
	}
	for _, group := range failureGroupsPrev {
		countPrev += group.TestFailures
	}
	if len(failureGroups) != 0 {
		median = failureGroups[len(failureGroups)/2].TestFailures
		avg = count / len(failureGroups)
	}
	if len(failureGroupsPrev) != 0 {
		medianPrev = failureGroupsPrev[len(failureGroupsPrev)/2].TestFailures
		avgPrev = count / len(failureGroupsPrev)
	}

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

func getPrevPlatform(platform string, jobsByPlatform []util.JobResult) *util.JobResult {
	for _, v := range jobsByPlatform {
		if v.Platform == platform {
			return &v
		}
	}
	return nil
}

func summaryJobsByPlatform(report, reportPrev util.TestReport, endDay, jobTestCount int) string {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Aggregation of all job runs for a given platform, sorted by passing rate percentage.  Platforms at the top of this list have unreliable CI jobs or the product is unreliable on those platforms.  The pass rate in parenthesis is the projected pass rate ignoring runs which failed only due to tests with associated bugs." id="JobPassRatesByPlatform" href="#JobPassRatesByPlatform">Job Pass Rates By Platform</a></th>
		</tr>
		<tr>
			<th>Platform</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, endDay)

	jobGroupTemplate := `
		<tr>
			<td>
				%[1]s
				<p>
				<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[1]s" aria-expanded="false" aria-controls="%[1]s">Expand Failing Tests</button>
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
			<tr>
				<td>
					%[1]s
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[1]s" aria-expanded="false" aria-controls="%[1]s">Expand Failing Tests</button>
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
		prev := getPrevPlatform(v.Platform, jobsByPlatformPrev)
		p := v.PassPercentage
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
			s = s + fmt.Sprintf(jobGroupTemplate, v.Platform,
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
				arrow,
				prev.PassPercentage,
				prev.PassPercentageWithKnownFailures,
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate, v.Platform,
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
		for _, test := range platformTests.TestResults {
			if util.IgnoreTestRegex.MatchString(test.Name) {
				continue
			}
			if count == 0 {
				additionalMatches++
				continue
			}
			count--

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
			jobQuery := fmt.Sprintf("%s.*%s|%s.*%s", report.Release, v.Platform, v.Platform, report.Release)

			bug := "Associated Bugs: "
			bugList := util.TestBugCache[test.Name]
			for _, b := range bugList {
				bug += fmt.Sprintf("<a target=\"_blank\" href=%s>%s</a> ", b.Url, b.ID)
			}
			if len(bugList) == 0 {
				bug = `<a style="padding-left: 20px" target="_blank" href="https://search.svc.ci.openshift.org/?context=1&type=bug&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s">Bug Search</a>`
				bug = fmt.Sprintf(bug, encodedTestName)
			}

			rows = rows + fmt.Sprintf(testGroupTemplate, strings.ReplaceAll(v.Platform, ".", ""),
				test.Name,
				jobQuery,
				encodedTestName,
				bug,
				test.PassPercentage,
				test.Successes+test.Failures,
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

func getPrevTest(test string, testResults []util.TestResult) *util.TestResult {
	for _, v := range testResults {
		if v.Name == test {
			return &v
		}
	}
	return nil
}

func summaryTopFailingTests(topFailingTestsWithoutBug, topFailingTestsWithBug []*util.TestResult, resultPrev map[string]util.SortedAggregateTestResult, endDay int) string {
	allPrev := resultPrev["all"]

	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Most frequently failing tests without a known bug, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="TopFailingTests" href="#TopFailingTests">Top Failing Tests Without A Bug</a></th>
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

	for _, test := range topFailingTestsWithoutBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", encodedTestName, test.Name)
		testPrev := getPrevTest(test.Name, allPrev.TestResults)

		bug := ""
		if test.BugErr != nil {
			bug = "Search Failed"
		} else {
			searchUrl := fmt.Sprintf("https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s", encodedTestName)
			exampleJob := "FIXME: Replace this paragraph with a particular job URI from the search results to ground discussion.  A given test may fail for several reasons, and this bug should be scoped to one of those reasons.  Ideally you'd pick a job showing the most-common reason, but since that's hard to determine, you may also chose to pick a job at random.  Release-gating jobs (release-openshift-...) should be preferred over presubmits (pull-ci-...) because they are closer to the released product and less likely to have in-flight code changes that complicate analysis."
			bug = fmt.Sprintf("<a target=\"_blank\" href=https://bugzilla.redhat.com/enter_bug.cgi?classification=Red%%20Hat&product=OpenShift%%20Container%%20Platform&cf_internal_whiteboard=buildcop&short_desc=%[1]s&cf_environment=%[1]s&comment=test:%%0A%[1]s%%20%%0A%%0Ais%%20failing%%20frequently%%20in%%20CI,%%20see%%20search%%20results:%%0A%s%%0A%%0A%s>Open a bug</a>", url.QueryEscape(test.Name), url.QueryEscape(searchUrl), url.QueryEscape(exampleJob))
		}

		if testPrev != nil {
			arrow := ""
			delta := 5.0
			if test.Successes+test.Failures > 80 {
				delta = 2
			}

			if test.PassPercentage > testPrev.PassPercentage+delta {
				arrow = fmt.Sprintf(up, test.PassPercentage-testPrev.PassPercentage)
			} else if test.PassPercentage < testPrev.PassPercentage-delta {
				arrow = fmt.Sprintf(down, testPrev.PassPercentage-test.PassPercentage)
			} else if test.PassPercentage > testPrev.PassPercentage {
				arrow = fmt.Sprintf(flatup, test.PassPercentage-testPrev.PassPercentage)
			} else {
				arrow = fmt.Sprintf(flatdown, testPrev.PassPercentage-test.PassPercentage)
			}

			s += fmt.Sprintf(template, testLink, bug, test.PassPercentage, test.Successes+test.Failures, arrow, testPrev.PassPercentage, testPrev.Successes+testPrev.Failures)
		} else {
			s += fmt.Sprintf(naTemplate, testLink, bug, test.PassPercentage, test.Successes+test.Failures)
		}
	}

	s += `<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Most frequently failing tests with a known bug, sorted by passing rate.">Top Failing Tests With A Bug</a></th>
		  </tr>
		<tr>
			<th>Test Name</th><th>BZ</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>`

	for _, test := range topFailingTestsWithBug {
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", encodedTestName, test.Name)
		testPrev := getPrevTest(test.Name, allPrev.TestResults)

		klog.V(2).Infof("processing top failing tests with bug %s, bugs: %v", test.Name, test.BugList)
		bug := ""
		for _, b := range test.BugList {
			bug += fmt.Sprintf("<a target=\"_blank\" href=%s>%s</a> ", b.Url, b.ID)
		}
		if testPrev != nil {
			arrow := ""
			delta := 5.0
			if test.Successes+test.Failures > 80 {
				delta = 2
			}
			if test.PassPercentage > testPrev.PassPercentage+delta {
				arrow = up
			} else if test.PassPercentage < testPrev.PassPercentage-delta {
				arrow = down
			}

			if test.PassPercentage > testPrev.PassPercentage+delta {
				arrow = fmt.Sprintf(up, test.PassPercentage-testPrev.PassPercentage)
			} else if test.PassPercentage < testPrev.PassPercentage-delta {
				arrow = fmt.Sprintf(down, testPrev.PassPercentage-test.PassPercentage)
			} else if test.PassPercentage > testPrev.PassPercentage {
				arrow = fmt.Sprintf(flatup, test.PassPercentage-testPrev.PassPercentage)
			} else {
				arrow = fmt.Sprintf(flatdown, testPrev.PassPercentage-test.PassPercentage)
			}

			s += fmt.Sprintf(template, testLink, bug, test.PassPercentage, test.Successes+test.Failures, arrow, testPrev.PassPercentage, testPrev.Successes+testPrev.Failures)
		} else {
			s += fmt.Sprintf(naTemplate, testLink, bug, test.PassPercentage, test.Successes+test.Failures)
		}
	}

	s = s + "</table>"
	return s
}

func getPrevJob(job string, jobRunsByJob []util.JobResult) *util.JobResult {
	for _, v := range jobRunsByJob {
		if v.Name == job {
			return &v
		}
	}
	return nil
}

func summaryJobPassRatesByJobName(report, reportPrev util.TestReport, endDay, jobTestCount int) string {
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
			<tr>
				<td>
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[3]s" aria-expanded="false" aria-controls="%[3]s">Expand Failing Tests</button>
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
			<tr>
				<td>
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[3]s" aria-expanded="false" aria-controls="%[3]s">Expand Failing Tests</button>
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
		prev := getPrevJob(v.Name, jobRunsByNamePrev)
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

			s = s + fmt.Sprintf(template, v.TestGridUrl, v.Name, strings.ReplaceAll(v.Name, ".", ""),
				v.PassPercentage,
				v.PassPercentageWithKnownFailures,
				v.Successes+v.Failures,
				arrow,
				prev.PassPercentage,
				prev.PassPercentageWithKnownFailures,
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate, v.TestGridUrl, v.Name, strings.ReplaceAll(v.Name, ".", ""),
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
			if util.IgnoreTestRegex.MatchString(test.Name) {
				continue
			}
			if count == 0 {
				additionalMatches++
				continue
			}
			count--

			encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

			bug := "Associated Bugs: "
			bugList := util.TestBugCache[test.Name]
			for _, b := range bugList {
				bug += fmt.Sprintf("<a target=\"_blank\" href=%s>%s</a> ", b.Url, b.ID)
			}
			if len(bugList) == 0 {
				bug = `<a style="padding-left: 20px" target="_blank" href="https://search.svc.ci.openshift.org/?context=1&type=bug&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s">Bug Search</a>`
				bug = fmt.Sprintf(bug, encodedTestName)
			}

			rows = rows + fmt.Sprintf(testGroupTemplate, strings.ReplaceAll(v.Name, ".", ""),
				test.Name,
				v.Name,
				encodedTestName,
				bug,
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

func canaryTestFailures(result map[string]util.SortedAggregateTestResult) string {
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

	for i := len(all) - 1; i > len(all)-10; i-- {
		test := all[i]
		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))

		testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.svc.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", encodedTestName, test.Name)

		s += fmt.Sprintf(template, testLink, test.PassPercentage, test.Successes+test.Failures)
	}
	s = s + "</table>"
	return s
}
func failureGroupList(report util.TestReport) string {
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

func testImpactingBugs(testImpactingBugs []util.Bug) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := `
	<table class="table">
		<tr>
			<th colspan=2 class="text-center"><a class="text-dark" title="Bugs which contain references to one or more failing tests, sorted by number of times the referenced tests failed." id="TestImpactingBugs" href="#TestImpactingBugs">Test Impacting Bugs</a></th>
		</tr>
		<tr>
			<th>Bug</th><th>Failure Count</th>
		</tr>
	`

	for _, bug := range testImpactingBugs {
		s += fmt.Sprintf("<tr><td><a target=\"_blank\" href=%s>%s</a></td><td>%d</td></tr> ", bug.Url, bug.Summary, bug.FailureCount)
	}

	s = s + "</table>"
	return s
}

type TestReports struct {
	Current      util.TestReport
	Prev         util.TestReport
	EndDay       int
	JobTestCount int
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

func PrintHtmlReport(w http.ResponseWriter, req *http.Request, report, prevReport util.TestReport, endDay, jobTestCount int) {

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
		},
	).Parse(dashboardPageHtml))

	if err := dashboardPage.Execute(w, TestReports{report, prevReport, endDay, jobTestCount}); err != nil {
		klog.Errorf("Unable to render page: %v", err)
	}

	//w.Write(result)
	fmt.Fprintf(w, htmlPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
