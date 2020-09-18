package sippyserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
	"github.com/openshift/sippy/pkg/testgridanalysis/testreportconversion"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
	"k8s.io/klog"
)

// TODO I think many of these are dead/inappropriate
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

type Analyzer struct {
	Options        Options
	LastUpdateTime time.Time
	Release        string

	BugCache buganalysis.BugCache
}

func (a *Analyzer) getTestGridData(releases []string, storagePath string) []testgridv1.JobDetails {
	testGridJobDetails := []testgridv1.JobDetails{}

	var jobFilter *regexp.Regexp
	if len(a.Options.JobFilter) > 0 {
		jobFilter = regexp.MustCompile(a.Options.JobFilter)
	}

	for _, release := range releases {
		dashboard := fmt.Sprintf(testgridhelpers.DashboardTemplate, release, "blocking")
		blockingJobs, ts, err := testgridhelpers.LoadJobSummaries(dashboard, storagePath)
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
					testGridJobDetails = append(testGridJobDetails, details)
				}
			}
		}
	}
	for _, release := range releases {
		dashboard := fmt.Sprintf(testgridhelpers.DashboardTemplate, release, "informing")
		informingJobs, _, err := testgridhelpers.LoadJobSummaries(dashboard, storagePath)
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
					testGridJobDetails = append(testGridJobDetails, details)
				}
			}
		}
	}

	return testGridJobDetails
}

// PrepareTestReport is expensive.  It gathers test grid data
func (a *Analyzer) PrepareTestReport() sippyprocessingv1.TestReport {
	testGridJobDetails := a.getTestGridData([]string{a.Release}, a.Options.LocalData)
	rawJobResultOptions := testgridconversion.ProcessingOptions{StartDay: a.Options.StartDay, EndDay: a.Options.EndDay}
	rawJobResults := rawJobResultOptions.ProcessTestGridDataIntoRawJobResults(testGridJobDetails)
	bugCacheWarnings := updateBugCacheForJobResults(a.BugCache, rawJobResults)

	return testreportconversion.PrepareTestReport(
		rawJobResults,
		a.BugCache,
		a.Release,
		a.Options.MinTestRuns,
		a.Options.TestSuccessThreshold,
		a.Options.EndDay,
		bugCacheWarnings,
		a.LastUpdateTime,
		a.Options.FailureClusterThreshold,
	)
}

// updateBugCacheForJobResults looks up all the bugs related to every failing test in the jobResults and returns a list of
// warnings/errors that happened looking up the data
func updateBugCacheForJobResults(bugCache buganalysis.BugCache, rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	// now that we have all the test failures (remember we added sythentics), use that to update the bugzilla cache
	failedTestNamesAcrossAllJobRuns := getFailedTestNamesFromJobResults(rawJobResults.JobResults)
	err := bugCache.UpdateForFailedTests(failedTestNamesAcrossAllJobRuns.List()...)
	if err != nil {
		klog.Error(err)
		warnings = append(warnings, fmt.Sprintf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error()))
	}

	return warnings
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

func loadJobDetails(dashboard, jobName, storagePath string) (testgridv1.JobDetails, error) {
	details := testgridv1.JobDetails{
		Name: jobName,
	}

	url := fmt.Sprintf("https://testgrid.k8s.io/%s/table?&show-stale-tests=&tab=%s&grid=old", dashboard, jobName)

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
