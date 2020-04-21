package html

import (
	"fmt"
	"net/http"
	"text/template"

	"k8s.io/klog"

	"github.com/bparees/sippy/pkg/util"
)

const htmlPageStart = `
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8"><title>%s</title>
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
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

const htmlPageEnd = `
</div>
Data current as of: %s
</body>
</html>
`

const dashboardPageHtml = `
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<style>
#table td, #table th {
	border: 
}
</style>

<h1>CI Release Health Summary</h1>

{{ summaryAcrossAllJobs .Current.All .Prev.All }}

{{ failureGroups .Current.FailureGroups .Prev.FailureGroups }}

{{ summaryJobsByPlatform .Current .Prev }}

{{ summaryTopFailingTests .Current.All .Prev.All }}

{{ summaryTopFailingJobs .Current .Prev }}
`

func summaryAcrossAllJobs(result, resultPrev map[string]util.SortedAggregateTestResult) string {

	all := result["all"]
	allPrev := resultPrev["all"]
	summary := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">Summary Across All Jobs</th>			
		</tr>
		<tr>
			<th/><th>Latest 7 days</th><th>Previous 7 days</th>
		</tr>
		<tr>
			<td>Test executions: </td><td>%d</td><td>%d</td>
		</tr>
		<tr>
			<td>Test Pass Percentage: </td><td>%0.2f</td><td>%0.2f</td>
		</tr>
	</table>`
	s := fmt.Sprintf(summary, all.Successes+all.Failures, allPrev.Successes+allPrev.Failures, all.TestPassPercentage, allPrev.TestPassPercentage)
	return s
}

func failureGroups(failureGroups, failureGroupsPrev []util.JobRunResult) string {
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
			<th colspan=3 class="text-center">Failure Groupings</th>			
		</tr>
		<tr>
			<th/><th>Latest 7 days</th><th>Previous 7 days</th>
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
	s := fmt.Sprintf(groups, len(failureGroups), len(failureGroupsPrev), avg, avgPrev, median, medianPrev)
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

func summaryJobsByPlatform(report, reportPrev util.TestReport) string {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	jobsByPlatformPrev := util.SummarizeJobsByPlatform(reportPrev)

	s := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">Job Pass Rates By Platform</th>
		</tr>
		<tr>
			<th>Platform</th><th>Latest 7 days</th><th>Previous 7 days</th>
		</tr>
	`
	template := `
		<tr>
			<td>%s</td><td>%0.2f%% (%d runs)</td><td>%0.2f%% (%d runs)</td>
		</tr>
	`
	for _, v := range jobsByPlatform {
		prev := getPrevPlatform(v.Platform, jobsByPlatformPrev)
		if prev != nil {
			s = s + fmt.Sprintf(template, v.Platform,
				util.Percent(v.Successes, v.Failures),
				v.Successes+v.Failures,
				util.Percent(prev.Successes, prev.Failures),
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(template, v.Platform,
				util.Percent(v.Successes, v.Failures),
				v.Successes+v.Failures,
				-1, -1,
			)
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

func summaryTopFailingTests(result, resultPrev map[string]util.SortedAggregateTestResult) string {
	all := result["all"]
	allPrev := resultPrev["all"]

	s := `
	<table class="table">
		<tr>
			<th colspan=4 class="text-center">Top Failing Tests</th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest 7 Days</th><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>Known Issue</th><th>Pass Rate</th><th>Pass Rate</th>
		</tr>
	`
	template := `
		<tr>
			<td>%s</td><td>%s</td><td>%0.2f%% (%d runs)</td><td>%0.2f%% (%d runs)</td>
		</tr>
	`

	count := 0
	for i := 0; count < 10 && i < len(all.TestResults); i++ {
		test := all.TestResults[i]
		if !util.IgnoreTestRegex.MatchString(test.Name) {
			known := "Yes"
			if !util.KnownIssueTestRegex.MatchString(test.Name) {
				count++
				known = "No"
			}
			testPrev := getPrevTest(test.Name, allPrev.TestResults)
			if testPrev != nil {
				s += fmt.Sprintf(template, test.Name, known, test.PassPercentage, test.Successes+test.Failures, testPrev.PassPercentage, testPrev.Successes+testPrev.Failures)
			} else {
				s += fmt.Sprintf(template, test.Name, known, test.PassPercentage, test.Successes+test.Failures, -1.0, -1)
			}
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

func summaryTopFailingJobs(report, reportPrev util.TestReport) string {
	jobRunsByName := util.SummarizeJobsByName(report)
	jobRunsByNamePrev := util.SummarizeJobsByName(reportPrev)

	s := `
	<table class="table">
		<tr>
			<th colspan=3 class="text-center">Job Pass Rates By Job Name</th>
		</tr>
		<tr>
			<th>Name</th><th>Latest 7 days</th><th>Previous 7 days</th>
		</tr>
	`
	template := `
		<tr>
			<td>%s</td><td>%0.2f%% (%d runs)</td><td>%0.2f%% (%d runs)</td>
		</tr>
	`
	for _, v := range jobRunsByName {
		prev := getPrevJob(v.Name, jobRunsByNamePrev)
		if prev != nil {
			s = s + fmt.Sprintf(template, v.Name,
				util.Percent(v.Successes, v.Failures),
				v.Successes+v.Failures,
				util.Percent(prev.Successes, prev.Failures),
				prev.Successes+prev.Failures,
			)
		} else {
			s = s + fmt.Sprintf(template, v.Name,
				util.Percent(v.Successes, v.Failures),
				v.Successes+v.Failures,
				-1, -1,
			)
		}
	}
	s = s + "</table>"
	return s
}

type TestReports struct {
	Current util.TestReport
	Prev    util.TestReport
}

func PrintHtmlReport(w http.ResponseWriter, req *http.Request, report, prevReport util.TestReport) {

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	fmt.Fprintf(w, htmlPageStart, "Release CI Health Dashboard")

	var dashboardPage = template.Must(template.New("dashboardPage").Funcs(
		template.FuncMap{
			"summaryAcrossAllJobs":   summaryAcrossAllJobs,
			"failureGroups":          failureGroups,
			"summaryJobsByPlatform":  summaryJobsByPlatform,
			"summaryTopFailingTests": summaryTopFailingTests,
			"summaryTopFailingJobs":  summaryTopFailingJobs,
		},
	).Parse(dashboardPageHtml))

	if err := dashboardPage.Execute(w, TestReports{report, prevReport}); err != nil {
		klog.Errorf("Unable to render page: %v", err)
	}

	//w.Write(result)
	fmt.Fprintf(w, htmlPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
