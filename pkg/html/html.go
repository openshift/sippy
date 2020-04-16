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
<h1>CI Release Health Summary</h1>

<p>================== Summary Across All Jobs ==================
<p>
{{ summaryAcrossAllJobs .All }}

<p>================== Clustered Test Failures ==================
<p>
{{ failureGroups .FailureGroups }}

<p>================== Summary By Platform ==================
<p>
{{ summaryJobsByPlatform . }}

<p>================== Top Failing Tests ==================
<p>
{{ summaryTopFailingTests .All }}


<p>================== Top Failing Jobs ==================
<p>
{{ summaryTopFailingJobs . }}
`

func summaryAcrossAllJobs(result map[string]util.SortedAggregateTestResult) string {

	all := result["all"]
	s := fmt.Sprintf("Total test runs: %d\n", all.Successes+all.Failures)
	s = s + fmt.Sprintf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)
	return s
}

func failureGroups(failureGroups []util.JobRunResult) string {
	count := 0
	s := ""
	for _, group := range failureGroups {
		count += group.TestFailures
	}
	if len(failureGroups) != 0 {
		s = fmt.Sprintf("%d Clustered Test Failures with an average size of %d and median of %d\n", len(failureGroups), count/len(failureGroups), failureGroups[len(failureGroups)/2].TestFailures)
	} else {
		s = fmt.Sprintf("No clustered test failures observed")
	}
	return s
}

func summaryJobsByPlatform(report util.TestReport) string {
	jobsByPlatform := util.SummarizeJobsByPlatform(report)
	s := ""
	for _, v := range jobsByPlatform {
		s += fmt.Sprintf("<p>")
		s += fmt.Sprintf("Platform: %s\n", v.Platform)
		s += fmt.Sprintf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			s += fmt.Sprintf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
	}
	return s
}

func summaryTopFailingTests(result map[string]util.SortedAggregateTestResult) string {
	all := result["all"]
	count := 0
	s := ""
	for i := 0; count < 10 && i < len(all.TestResults); i++ {
		test := all.TestResults[i]
		if !util.IgnoreTestRegex.MatchString(test.Name) {
			s += fmt.Sprintf("<p>")
			s += fmt.Sprintf("Test Name: %s\n", test.Name)
			s += fmt.Sprintf("Test Pass Percentage: %0.2f (%d runs)\n", test.PassPercentage, test.Successes+test.Failures)
			if test.Successes+test.Failures < 10 {
				s += fmt.Sprintf("WARNING: Only %d runs for this test\n", test.Successes+test.Failures)
			}
			count++
		}
	}
	return s
}

func summaryTopFailingJobs(report util.TestReport) string {
	jobRunsByName := util.SummarizeJobsByName(report)

	s := ""
	for i, v := range jobRunsByName {
		s += fmt.Sprintf("<p>")
		s += fmt.Sprintf("Job: %s\n", v.Name)
		s += fmt.Sprintf("Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			s += fmt.Sprintf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		if i == 9 {
			break
		}
	}
	return s
}
func PrintHtmlReport(w http.ResponseWriter, req *http.Request, report util.TestReport) {

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

	if err := dashboardPage.Execute(w, report); err != nil {
		klog.Errorf("Unable to render page: %v", err)
	}

	//w.Write(result)
	fmt.Fprintf(w, htmlPageEnd, report.Timestamp.Format("Jan 2 15:04 2006 MST"))
}
