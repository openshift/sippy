package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/html"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

var (
	dashboardTemplate = "redhat-openshift-ocp-release-%s-%s"
	TagStripRegex     = regexp.MustCompile(`\[Skipped:.*?\]|\[Suite:.*\]`)
)

type Analyzer struct {
	// TestGridJobInfo contains the data consumed from testgrid
	TestGridJobInfo []testgridv1.JobDetails

	RawData        testgridanalysisapi.RawData
	Options        *Options
	Report         sippyprocessingv1.TestReport
	LastUpdateTime time.Time
	Release        string

	BugCache         buganalysis.BugCache
	analysisWarnings []string
}

func loadJobSummaries(dashboard string, storagePath string) (map[string]testgridv1.JobSummary, time.Time, error) {
	jobs := make(map[string]testgridv1.JobSummary)
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	var buf *bytes.Buffer
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return jobs, time.Time{}, fmt.Errorf("Could not read local data file %s: %v", filename, err)
	}
	buf = bytes.NewBuffer(b)
	f, _ := os.Stat(filename)
	f.ModTime()

	err = json.NewDecoder(buf).Decode(&jobs)
	if err != nil {
		return nil, time.Time{}, err
	}

	return jobs, f.ModTime(), nil

}

func downloadJobSummaries(dashboard string, storagePath string) error {
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/summary", dashboard)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Non-200 response code fetching job summary: %v", resp)
	}
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err
}

func loadJobDetails(dashboard, jobName, storagePath string) (testgridv1.JobDetails, error) {
	details := testgridv1.JobDetails{
		Name: jobName,
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s", dashboard, jobName)

	var buf *bytes.Buffer
	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return details, fmt.Errorf("Could not read local data file %s: %v", filename, err)
	}
	buf = bytes.NewBuffer(b)

	err = json.NewDecoder(buf).Decode(&details)
	if err != nil {
		return details, err
	}
	details.TestGridUrl = fmt.Sprintf("https://testgrid.k8s.io/%s#%s", dashboard, jobName)
	return details, nil
}

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?&show-stale-tests=&tab=release-openshift-origin-installer-e2e-azure-compact-4.4

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing#release-openshift-origin-installer-e2e-azure-compact-4.4&show-stale-tests=&sort-by-failures=

func downloadJobDetails(dashboard, jobName, storagePath string) error {
	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s", dashboard, jobName)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Non-200 response code fetching job details: %v", resp)
	}

	filename := storagePath + "/" + "\"" + strings.ReplaceAll(url, "/", "-") + "\""
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err

}

// ignoreTestRegex is used to strip o ut tests that don't have predictive or diagnostic value.  We don't want to show these in our data.
var ignoreTestRegex = regexp.MustCompile(`Run multi-stage test|operator.Import the release payload|operator.Import a release payload|operator.Run template|operator.Build image|Monitor cluster while tests execute|Overall|job.initialize|\[sig-arch\]\[Feature:ClusterUpgrade\] Cluster should remain functional during upgrade`)

// processTestToJobRunResults adds the tests to the provided jobresult to the provided JobResult and returns the passed, failed, flaked for the test
func processTestToJobRunResults(jobResult testgridanalysisapi.RawJobResult, job testgridv1.JobDetails, test testgridv1.Test, startCol, endCol int) (passed int, failed int, flaked int) {
	col := 0
	for _, result := range test.Statuses {
		if col > endCol {
			break
		}

		// the test results are run length encoded(e.g. "6 passes, 5 failures, 7 passes"), but since we are searching for a test result
		// from a specific time period, it's possible a particular run of results overlaps the start-point
		// for the time period we care about.  So we need to iterate each encoded run until we get to the column
		// we care about(a column which falls within the timestamp range we care about, then start the analysis with the remaining
		// columns in the run.
		remaining := result.Count
		if col < startCol {
			for i := 0; i < result.Count && col < startCol; i++ {
				col++
				remaining--
			}
		}
		// if after iterating above we still aren't within the column range we care about, don't do any analysis
		// on this run of results.
		if col < startCol {
			continue
		}
		switch result.Value {
		case 1, 13: // success, flake(failed one or more times but ultimately succeeded)
			for i := col; i < col+remaining && i < endCol; i++ {
				passed++
				if result.Value == 13 {
					flaked++
				}
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunURL: joburl,
					}
				}
				switch {
				case test.Name == "Overall":
					jrr.Succeeded = true
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorInstallPrefix):
					jrr.InstallOperators = append(jrr.InstallOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorInstallPrefix):],
						State: testgridanalysisapi.Success,
					})
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorUpgradePrefix):
					jrr.UpgradeOperators = append(jrr.UpgradeOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorUpgradePrefix):],
						State: testgridanalysisapi.Success,
					})
				case strings.HasSuffix(test.Name, "container setup"):
					jrr.SetupStatus = testgridanalysisapi.Success
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		case 12: // failure
			for i := col; i < col+remaining && i < endCol; i++ {
				failed++
				joburl := fmt.Sprintf("https://prow.svc.ci.openshift.org/view/gcs/%s/%s", job.Query, job.ChangeLists[i])
				jrr, ok := jobResult.JobRunResults[joburl]
				if !ok {
					jrr = testgridanalysisapi.RawJobRunResult{
						Job:       job.Name,
						JobRunURL: joburl,
					}
				}
				// only add the failing test and name if it has predictive value.  We excluded all the non-predictive ones above except for these
				// which we use to set various JobRunResult markers
				if test.Name != "Overall" && !strings.HasSuffix(test.Name, "container setup") {
					jrr.FailedTestNames = append(jrr.FailedTestNames, test.Name)
					jrr.TestFailures++
				}

				switch {
				case test.Name == "Overall":
					jrr.Failed = true
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorInstallPrefix):
					jrr.InstallOperators = append(jrr.InstallOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorInstallPrefix):],
						State: testgridanalysisapi.Failure,
					})
				case strings.HasPrefix(test.Name, testgridanalysisapi.OperatorUpgradePrefix):
					jrr.UpgradeOperators = append(jrr.UpgradeOperators, testgridanalysisapi.OperatorState{
						Name:  test.Name[len(testgridanalysisapi.OperatorUpgradePrefix):],
						State: testgridanalysisapi.Failure,
					})
				case strings.HasSuffix(test.Name, "container setup"):
					jrr.SetupStatus = testgridanalysisapi.Failure
				}
				jobResult.JobRunResults[joburl] = jrr
			}
		}
		col += remaining
	}

	util.AddTestResult(jobResult.TestResults, test.Name, passed, failed, flaked)

	return
}

func (a *Analyzer) processTest(job testgridv1.JobDetails, platforms []string, test testgridv1.Test, sig string, startCol, endCol int) {
	// strip out tests that don't have predictive or diagnostic value
	// we have to know about overall to be able to set the global success or failure.
	// we have to know about container setup to be able to set infra failures
	if test.Name != "Overall" && !strings.HasSuffix(test.Name, "container setup") && ignoreTestRegex.MatchString(test.Name) {
		return
	}

	jobResult, ok := a.RawData.JobResults[job.Name]
	if !ok {
		jobResult = testgridanalysisapi.RawJobResult{
			JobName:        job.Name,
			TestGridJobUrl: job.TestGridUrl,
			JobRunResults:  map[string]testgridanalysisapi.RawJobRunResult{},
			TestResults:    map[string]testgridanalysisapi.RawTestResult{},
		}
	}

	passed, failed, flaked := processTestToJobRunResults(jobResult, job, test, startCol, endCol)

	// we have mutated, so assign back to our intermediate value
	a.RawData.JobResults[job.Name] = jobResult

	// our aggregation and markers are correctly set above.  We allowed these two tests to be checked, but we don't want
	// actual results for them
	if test.Name == "Overall" || strings.HasSuffix(test.Name, "container setup") {
		return
	}

	util.AddTestResultToCategory("all", a.RawData.ByAll, test.Name, passed, failed, flaked)
}

func (a *Analyzer) processJobDetails(job testgridv1.JobDetails) {
	startCol, endCol := util.ComputeLookback(a.Options.StartDay, a.Options.EndDay, job.Timestamps)
	platforms := testidentification.FindPlatform(job.Name)

	for i, test := range job.Tests {
		klog.V(4).Infof("Analyzing results from %d to %d from job %s for test %s\n", startCol, endCol, job.Name, test.Name)

		test.Name = strings.TrimSpace(TagStripRegex.ReplaceAllString(test.Name, ""))
		job.Tests[i] = test

		a.processTest(job, platforms, test, testidentification.FindSig(test.Name), startCol, endCol)
	}

}

// createSyntheticTests takes the JobRunResult information and produces some pre-analysis by interpreting different types of failures
// and potentially producing synthentic test results and aggregations to better inform sippy.
// This needs to be called after all the JobDetails have been processed.
func (a *Analyzer) createSyntheticTests() {
	// make a pass to fill in install, upgrade, and infra synthentic tests.
	type synthenticTestResult struct {
		name string
		pass int
		fail int
	}
	for jobName, jobResults := range a.RawData.JobResults {
		for jrrKey, jrr := range jobResults.JobRunResults {
			isUpgrade := strings.Contains(jrr.Job, "upgrade")

			syntheticTests := map[string]*synthenticTestResult{
				testgridanalysisapi.InstallTestName:        &synthenticTestResult{name: testgridanalysisapi.InstallTestName},
				testgridanalysisapi.UpgradeTestName:        &synthenticTestResult{name: testgridanalysisapi.UpgradeTestName},
				testgridanalysisapi.InfrastructureTestName: &synthenticTestResult{name: testgridanalysisapi.InfrastructureTestName},
			}

			installFailed := false
			for _, operator := range jrr.InstallOperators {
				if operator.State == testgridanalysisapi.Failure {
					installFailed = true
					break
				}
			}
			upgradeFailed := false
			for _, operator := range jrr.UpgradeOperators {
				if operator.State == testgridanalysisapi.Failure {
					upgradeFailed = true
					break
				}
			}
			setupFailed := jrr.SetupStatus != testgridanalysisapi.Success

			if installFailed {
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InstallTestName)
				syntheticTests[testgridanalysisapi.InstallTestName].fail = 1
			} else {
				if !setupFailed { // this will be an undercount, but we only want to count installs that actually worked.
					syntheticTests[testgridanalysisapi.InstallTestName].pass = 1
				}
			}
			if setupFailed && len(jrr.InstallOperators) == 0 { // we only want to count it as an infra issue if the install did not start
				jrr.TestFailures++
				jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.InfrastructureTestName)
				syntheticTests[testgridanalysisapi.InfrastructureTestName].fail = 1
			} else {
				syntheticTests[testgridanalysisapi.InfrastructureTestName].pass = 1
			}
			if isUpgrade && !setupFailed && !installFailed { // only record upgrade status if we were able to attempt the upgrade
				if upgradeFailed || len(jrr.UpgradeOperators) == 0 {
					jrr.TestFailures++
					jrr.FailedTestNames = append(jrr.FailedTestNames, testgridanalysisapi.UpgradeTestName)
					syntheticTests[testgridanalysisapi.UpgradeTestName].fail = 1
				} else {
					syntheticTests[testgridanalysisapi.UpgradeTestName].pass = 1
				}
			}

			for testName, result := range syntheticTests {
				util.AddTestResult(jobResults.TestResults, testName, result.pass, result.fail, 0)

				util.AddTestResultToCategory("all", a.RawData.ByAll, testName, result.pass, result.fail, 0)
			}

			jobResults.JobRunResults[jrrKey] = jrr
		}

		a.RawData.JobResults[jobName] = jobResults
	}
}

func getFailedTestNamesFromJobResults(jobResults map[string]testgridanalysisapi.RawJobResult) sets.String {
	failedTestNames := sets.NewString()
	for _, jobResult := range jobResults {
		for _, jobrun := range jobResult.JobRunResults {
			failedTestNames.Insert(jobrun.FailedTestNames...)
		}
	}
	return failedTestNames
}

func (a *Analyzer) analyze() {
	for _, details := range a.TestGridJobInfo {
		klog.V(2).Infof("processing test details for job %s\n", details.Name)
		a.processJobDetails(details)
	}

	// now that we have all the JobRunResults, use them to create synthetic tests for install, upgrade, and infra
	a.createSyntheticTests()

	// now that we have all the test failures (remember we added sythentics), use that to update the bugzilla cache
	failedTestNamesAcrossAllJobRuns := getFailedTestNamesFromJobResults(a.RawData.JobResults)
	err := a.BugCache.UpdateForFailedTests(failedTestNamesAcrossAllJobRuns.List()...)
	if err != nil {
		klog.Error(err)
		a.analysisWarnings = append(a.analysisWarnings, fmt.Sprintf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error()))
	}
}

func (a *Analyzer) loadData(releases []string, storagePath string) {
	var jobFilter *regexp.Regexp
	if len(a.Options.JobFilter) > 0 {
		jobFilter = regexp.MustCompile(a.Options.JobFilter)
	}

	for _, release := range releases {

		dashboard := fmt.Sprintf(dashboardTemplate, release, "blocking")
		blockingJobs, ts, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}
		a.LastUpdateTime = ts
		for jobName, job := range blockingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := loadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error loading job details for %s: %v\n", jobName, err)
				} else {
					a.TestGridJobInfo = append(a.TestGridJobInfo, details)
				}
			}
		}
	}
	for _, release := range releases {
		dashboard := fmt.Sprintf(dashboardTemplate, release, "informing")
		informingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error load dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range informingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := loadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error loading job details for %s: %v\n", jobName, err)
				} else {
					a.TestGridJobInfo = append(a.TestGridJobInfo, details)
				}
			}
		}
	}
}

func downloadData(releases []string, filter string, storagePath string) {
	var jobFilter *regexp.Regexp
	if len(filter) > 0 {
		jobFilter = regexp.MustCompile(filter)
	}

	for _, release := range releases {

		dashboard := fmt.Sprintf(dashboardTemplate, release, "blocking")
		err := downloadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}
		blockingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range blockingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				err := downloadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
			}
		}
	}

	for _, release := range releases {

		dashboard := fmt.Sprintf(dashboardTemplate, release, "informing")
		err := downloadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}
		informingJobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range informingJobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				err := downloadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
			}
		}
	}
}

func (a *Analyzer) prepareTestReport() {
	a.Report = testreportconversion.PrepareTestReport(
		a.RawData,
		a.BugCache,
		a.Release,
		a.Options.MinTestRuns,
		a.Options.TestSuccessThreshold,
		a.Options.EndDay,
		a.analysisWarnings,
		a.LastUpdateTime,
		a.Options.FailureClusterThreshold,
	)
}

func (a *Analyzer) printReport() {
	a.prepareTestReport()
	switch a.Options.Output {
	case "json":
		a.printJsonReport()
	case "text":
		a.printTextReport()
	case "dashboard":
		a.printDashboardReport()
	}
}
func (a *Analyzer) printJsonReport() {
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(a.Report)
}

func (a *Analyzer) printDashboardReport() {
	fmt.Println("================== Summary Across All Jobs ==================")
	all := a.Report.All["all"]
	fmt.Printf("Passing test runs: %d\n", all.Successes)
	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Tests ==================")
	count := 0
	for i := 0; count < 10 && i < len(all.TestResults); i++ {
		test := all.TestResults[i]
		if (test.Successes + test.Failures) > a.Options.MinTestRuns {
			fmt.Printf("Test Name: %s\n", test.Name)
			fmt.Printf("Test Pass Percentage: %0.2f (%d runs)\n", test.PassPercentage, test.Successes+test.Failures)
			if test.Successes+test.Failures < 10 {
				fmt.Printf("WARNING: Only %d runs for this test\n", test.Successes+test.Failures)
			}
			count++
			fmt.Printf("\n")
		}
	}

	fmt.Println("\n\n================== Top 10 Most Frequently Failing Jobs ==================")
	for i, v := range a.Report.FrequentJobResults {
		fmt.Printf("Job: %s\n", v.Name)
		fmt.Printf("Job Pass Percentage: %0.2f%% (%d runs)\n", util.Percent(v.Successes, v.Failures), v.Successes+v.Failures)
		if v.Successes+v.Failures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.Successes+v.Failures)
		}
		fmt.Printf("\n")
		if i == 9 {
			break
		}
	}

	fmt.Println("\n\n================== Clustered Test Failures ==================")
	count = 0
	for _, group := range a.Report.FailureGroups {
		count += group.TestFailures
	}
	if len(a.Report.FailureGroups) != 0 {
		fmt.Printf("%d Clustered Test Failures with an average size of %d and median of %d\n", len(a.Report.FailureGroups), count/len(a.Report.FailureGroups), a.Report.FailureGroups[len(a.Report.FailureGroups)/2].TestFailures)
	} else {
		fmt.Printf("No clustered test failures observed")
	}

	fmt.Println("\n\n================== Summary By Platform ==================")
	for _, v := range a.Report.ByPlatform {
		fmt.Printf("Platform: %s\n", v.PlatformName)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", v.JobRunPassPercentage)
		if v.JobRunSuccesses+v.JobRunFailures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.JobRunSuccesses+v.JobRunFailures)
		}
		fmt.Printf("\n")
	}
}

func (a *Analyzer) printTextReport() {
	fmt.Println("================== Test Summary Across All Jobs ==================")
	all := a.Report.All["all"]
	fmt.Printf("Passing test runs: %d\n", all.Successes)
	fmt.Printf("Failing test runs: %d\n", all.Failures)
	fmt.Printf("Test Pass Percentage: %0.2f\n", all.TestPassPercentage)
	testCount := 0
	testSuccesses := 0
	testFailures := 0
	for _, test := range all.TestResults {
		fmt.Printf("\tTest Name: %s\n", test.Name)
		fmt.Printf("\tPassed: %d\n", test.Successes)
		fmt.Printf("\tFailed: %d\n", test.Failures)
		fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		testCount++
		testSuccesses += test.Successes
		testFailures += test.Failures
	}

	fmt.Println("\n\n\n================== Test Summary By Platform ==================")
	for key, by := range a.Report.ByPlatform {
		fmt.Printf("Platform: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Test Pass Percentage: %0.2f\n", by.JobRunPassPercentage)
		for _, test := range by.AllTestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			fmt.Printf("\tPassed: %d\n", test.Successes)
			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		}
		fmt.Println("")
	}

	fmt.Println("\n\n\n================== Test Summary By Job ==================")
	for key, by := range a.Report.FrequentJobResults {
		fmt.Printf("Job: %s\n", key)
		//		fmt.Printf("Passing test runs: %d\n", platform.Successes)
		//		fmt.Printf("Failing test runs: %d\n", platform.Failures)
		fmt.Printf("Job Pass Percentage: %0.2f\n", by.PassPercentage)
		for _, test := range by.TestResults {
			fmt.Printf("\tTest Name: %s\n", test.Name)
			fmt.Printf("\tPassed: %d\n", test.Successes)
			fmt.Printf("\tFailed: %d\n", test.Failures)
			fmt.Printf("\tTest Pass Percentage: %0.2f\n\n", test.PassPercentage)
		}
		fmt.Println("")
	}

	fmt.Println("\n\n\n================== Clustered Test Failures ==================")
	for _, group := range a.Report.FailureGroups {
		fmt.Printf("Job url: %s\n", group.Url)
		fmt.Printf("Number of test failures: %d\n\n", group.TestFailures)
	}

	fmt.Println("\n\n\n================== Job Pass Rates ==================")
	jobSuccesses := 0
	jobFailures := 0
	jobCount := 0

	for _, job := range a.Report.FrequentJobResults {
		fmt.Printf("Job: %s\n", job.Name)
		fmt.Printf("Job Successes: %d\n", job.Successes)
		fmt.Printf("Job Failures: %d\n", job.Failures)
		fmt.Printf("Job Pass Percentage: %0.2f\n\n", job.PassPercentage)
		jobSuccesses += job.Successes
		jobFailures += job.Failures
		jobCount++
	}

	fmt.Println("\n\n================== Job Summary By Platform ==================")
	for _, v := range a.Report.ByPlatform {
		fmt.Printf("Platform: %s\n", v.PlatformName)
		fmt.Printf("Job Succeses: %d\n", v.JobRunSuccesses)
		fmt.Printf("Job Failures: %d\n", v.JobRunFailures)
		fmt.Printf("Platform Job Pass Percentage: %0.2f%% (%d runs)\n", v.JobRunPassPercentage)
		if v.JobRunSuccesses+v.JobRunFailures < 10 {
			fmt.Printf("WARNING: Only %d runs for this job\n", v.JobRunSuccesses+v.JobRunFailures)
		}
		fmt.Printf("\n")
	}

	fmt.Println("")

	fmt.Println("\n\n================== Overall Summary ==================")
	fmt.Printf("Total Jobs: %d\n", jobCount)
	fmt.Printf("Total Job Successes: %d\n", jobSuccesses)
	fmt.Printf("Total Job Failures: %d\n", jobFailures)
	fmt.Printf("Total Job Pass Percentage: %0.2f\n\n", util.Percent(jobSuccesses, jobFailures))

	fmt.Printf("Total Tests: %d\n", testCount)
	fmt.Printf("Total Test Successes: %d\n", testSuccesses)
	fmt.Printf("Total Test Failures: %d\n", testFailures)
	fmt.Printf("Total Test Pass Percentage: %0.2f\n", util.Percent(testSuccesses, testFailures))
}

type Server struct {
	bugCache  buganalysis.BugCache
	analyzers map[string]Analyzer
	options   *Options
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	klog.Infof("Refreshing data")
	s.bugCache.Clear()

	for k, analyzer := range s.analyzers {
		analyzer.RawData = testgridanalysisapi.RawData{
			ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
			JobResults: make(map[string]testgridanalysisapi.RawJobResult),
		}

		analyzer.loadData([]string{analyzer.Release}, analyzer.Options.LocalData)
		analyzer.analyze()
		analyzer.prepareTestReport()
		s.analyzers[k] = analyzer
	}

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	klog.Infof("Refresh complete")
}

func (s *Server) printHtmlReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if _, ok := s.analyzers[release]; !ok {
		html.WriteLandingPage(w, s.options.Releases)
		return
	}
	html.PrintHtmlReport(w, req, s.analyzers[release].Report, s.analyzers[release+"-prev"].Report, s.options.EndDay, 15)
}

func (s *Server) printJSONReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	releaseReports := make(map[string][]sippyprocessingv1.TestReport)
	if release == "all" {
		// return all available json reports
		// store [currentReport, prevReport] in a slice
		for _, r := range s.options.Releases {
			if _, ok := s.analyzers[r]; ok {
				releaseReports[r] = []sippyprocessingv1.TestReport{s.analyzers[r].Report, s.analyzers[r+"-prev"].Report}
			} else {
				klog.Errorf("unable to load test report for release version %s", r)
				continue
			}
		}
		api.PrintJSONReport(w, req, releaseReports, s.options.EndDay, 15)
		return
	} else if _, ok := s.analyzers[release]; !ok {
		// return a 404 error along with the list of available releases in the detail section
		errMsg := map[string]interface{}{
			"code":   "404",
			"detail": fmt.Sprintf("No valid release specified, valid releases are: %v", s.options.Releases),
		}
		errMsgBytes, _ := json.Marshal(errMsg)
		w.WriteHeader(http.StatusNotFound)
		w.Write(errMsgBytes)
		return
	}
	releaseReports[release] = []sippyprocessingv1.TestReport{s.analyzers[release].Report, s.analyzers[release+"-prev"].Report}
	api.PrintJSONReport(w, req, releaseReports, s.options.EndDay, 15)
}

func (s *Server) detailed(w http.ResponseWriter, req *http.Request) {

	release := "4.5"
	t := req.URL.Query().Get("release")
	if t != "" {
		release = t
	}

	startDay := 0
	t = req.URL.Query().Get("startDay")
	if t != "" {
		startDay, _ = strconv.Atoi(t)
	}

	endDay := startDay + 7
	t = req.URL.Query().Get("endDay")
	if t != "" {
		endDay, _ = strconv.Atoi(t)
	}

	testSuccessThreshold := 98.0
	t = req.URL.Query().Get("testSuccessThreshold")
	if t != "" {
		testSuccessThreshold, _ = strconv.ParseFloat(t, 64)
	}

	jobFilter := ""
	t = req.URL.Query().Get("jobFilter")
	if t != "" {
		jobFilter = t
	}

	minTestRuns := 10
	t = req.URL.Query().Get("minTestRuns")
	if t != "" {
		minTestRuns, _ = strconv.Atoi(t)
	}

	fct := 10
	t = req.URL.Query().Get("failureClusterThreshold")
	if t != "" {
		fct, _ = strconv.Atoi(t)
	}

	jobTestCount := math.MaxInt32
	t = req.URL.Query().Get("jobTestCount")
	if t != "" {
		jobTestCount, _ = strconv.Atoi(t)
	}

	opt := &Options{
		StartDay:                startDay,
		EndDay:                  endDay,
		TestSuccessThreshold:    testSuccessThreshold,
		JobFilter:               jobFilter,
		MinTestRuns:             minTestRuns,
		FailureClusterThreshold: fct,
	}

	analyzer := Analyzer{
		Release: release,
		Options: opt,
		RawData: testgridanalysisapi.RawData{
			ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
			JobResults: make(map[string]testgridanalysisapi.RawJobResult),
		},
		BugCache: s.bugCache,
	}
	analyzer.loadData([]string{release}, s.options.LocalData)
	analyzer.analyze()
	analyzer.prepareTestReport()

	// prior 7 day period
	optCopy := *opt
	optCopy.StartDay = endDay + 1
	optCopy.EndDay = endDay + 8
	prevAnalyzer := Analyzer{
		Release: release,
		Options: &optCopy,
		RawData: testgridanalysisapi.RawData{
			ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
			JobResults: make(map[string]testgridanalysisapi.RawJobResult),
		},
		BugCache: s.bugCache,
	}
	prevAnalyzer.loadData([]string{release}, s.options.LocalData)
	prevAnalyzer.analyze()
	prevAnalyzer.prepareTestReport()

	html.PrintHtmlReport(w, req, analyzer.Report, prevAnalyzer.Report, opt.EndDay, jobTestCount)

}

func (s *Server) serve(opts *Options) {
	http.DefaultServeMux.HandleFunc("/", s.printHtmlReport)
	http.DefaultServeMux.HandleFunc("/json", s.printJSONReport)
	http.DefaultServeMux.HandleFunc("/detailed", s.detailed)
	http.DefaultServeMux.HandleFunc("/refresh", s.refresh)
	//go func() {
	klog.Infof("Serving reports on %s ", opts.ListenAddr)
	if err := http.ListenAndServe(opts.ListenAddr, nil); err != nil {
		klog.Exitf("Server exited: %v", err)
	}
	//}()
}

type Options struct {
	LocalData               string
	Releases                []string
	StartDay                int
	EndDay                  int
	TestSuccessThreshold    float64
	JobFilter               string
	MinTestRuns             int
	Output                  string
	FailureClusterThreshold int
	FetchData               string
	ListenAddr              string
	Server                  bool
}

func main() {
	opt := &Options{
		EndDay:                  7,
		TestSuccessThreshold:    99.99,
		MinTestRuns:             10,
		Output:                  "json",
		FailureClusterThreshold: 10,
		StartDay:                0,
		ListenAddr:              ":8080",
		Releases:                []string{"4.4"},
	}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			if err := opt.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.LocalData, "local-data", opt.LocalData, "Path to testgrid data from local disk")
	flags.StringArrayVar(&opt.Releases, "release", opt.Releases, "Which releases to analyze (one per arg instance)")
	flags.IntVar(&opt.StartDay, "start-day", opt.StartDay, "Analyze data starting from this day")
	flags.IntVar(&opt.EndDay, "end-day", opt.EndDay, "Look at job runs going back to this day")
	flags.Float64Var(&opt.TestSuccessThreshold, "test-success-threshold", opt.TestSuccessThreshold, "Filter results for tests that are more than this percent successful")
	flags.StringVar(&opt.JobFilter, "job-filter", opt.JobFilter, "Only analyze jobs that match this regex")
	flags.StringVar(&opt.FetchData, "fetch-data", opt.FetchData, "Download testgrid data to directory specified for future use with --local-data")
	flags.IntVar(&opt.MinTestRuns, "min-test-runs", opt.MinTestRuns, "Ignore tests with less than this number of runs")
	flags.IntVar(&opt.FailureClusterThreshold, "failure-cluster-threshold", opt.FailureClusterThreshold, "Include separate report on job runs with more than N test failures, -1 to disable")
	flags.StringVarP(&opt.Output, "output", "o", opt.Output, "Output format for report: json, text")
	flag.StringVar(&opt.ListenAddr, "listen", opt.ListenAddr, "The address to serve analysis reports on")
	flags.BoolVar(&opt.Server, "server", opt.Server, "Run in web server mode (serve reports over http)")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *Options) Run() error {
	switch o.Output {
	case "json", "text", "dashboard":
	default:
		return fmt.Errorf("invalid output type: %s\n", o.Output)
	}

	if len(o.FetchData) != 0 {
		downloadData(o.Releases, o.JobFilter, o.FetchData)
		return nil
	}
	if !o.Server {
		analyzer := Analyzer{
			Options: o,
			RawData: testgridanalysisapi.RawData{
				ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
				JobResults: make(map[string]testgridanalysisapi.RawJobResult),
			},
			BugCache: buganalysis.NewBugCache(),
		}

		analyzer.loadData(o.Releases, o.LocalData)
		analyzer.analyze()
		analyzer.printReport()
	}

	if o.Server {
		server := Server{
			bugCache:  buganalysis.NewBugCache(),
			analyzers: make(map[string]Analyzer),
			options:   o,
		}
		for _, release := range o.Releases {
			// most recent 7 day period (days 0-7)
			analyzer := Analyzer{
				Release: release,
				Options: o,
				RawData: testgridanalysisapi.RawData{
					ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
					JobResults: make(map[string]testgridanalysisapi.RawJobResult),
				},
				BugCache: server.bugCache,
			}
			analyzer.loadData([]string{release}, o.LocalData)
			analyzer.analyze()
			analyzer.prepareTestReport()
			server.analyzers[release] = analyzer

			// prior 7 day period (days 7-14)
			optCopy := *o
			optCopy.EndDay = 14
			optCopy.StartDay = 7
			analyzer = Analyzer{
				Release: release,
				Options: &optCopy,
				RawData: testgridanalysisapi.RawData{
					ByAll:      make(map[string]testgridanalysisapi.AggregateTestsResult),
					JobResults: make(map[string]testgridanalysisapi.RawJobResult),
				},
				BugCache: server.bugCache,
			}
			analyzer.loadData([]string{release}, o.LocalData)
			analyzer.analyze()
			analyzer.prepareTestReport()
			server.analyzers[release+"-prev"] = analyzer
		}
		server.serve(o)
	}

	return nil
}
