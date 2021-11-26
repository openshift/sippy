package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	gosort "sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"k8s.io/klog"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	workloadmetricsv1 "github.com/openshift/sippy/pkg/apis/workloadmetrics/v1"
	"github.com/openshift/sippy/pkg/util"
)

type jobsAPIResult []apitype.Job

func (jobs jobsAPIResult) sort(req *http.Request) jobsAPIResult {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))

	if sortField == "" {
		sortField = "current_pass_percentage"
	}

	if sort == "" {
		sort = apitype.SortAscending
	}

	gosort.Slice(jobs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return compare(jobs[i], jobs[j], sortField)
		}
		return compare(jobs[j], jobs[i], sortField)
	})

	return jobs
}

func (jobs jobsAPIResult) limit(req *http.Request) jobsAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(jobs) >= limit {
		return jobs[:limit]
	}

	return jobs
}

func briefName(job string) string {
	briefName := regexp.MustCompile("periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-")
	return briefName.ReplaceAllString(job, "")
}

func jobResultToAPI(id int, current, previous *v1sippyprocessing.JobResult) apitype.Job {
	job := apitype.Job{
		ID:                             id,
		Name:                           current.Name,
		Variants:                       current.Variants,
		BriefName:                      briefName(current.Name),
		CurrentPassPercentage:          current.PassPercentage,
		CurrentProjectedPassPercentage: current.PassPercentageWithoutInfrastructureFailures,
		CurrentRuns:                    current.Failures + current.Successes,
	}

	if previous != nil {
		job.PreviousPassPercentage = previous.PassPercentage
		job.PreviousProjectedPassPercentage = previous.PassPercentageWithoutInfrastructureFailures
		job.PreviousRuns = previous.Failures + previous.Successes
		job.NetImprovement = current.PassPercentage - previous.PassPercentage
	}

	job.Bugs = current.BugList
	job.AssociatedBugs = current.AssociatedBugList
	job.TestGridURL = current.TestGridURL

	if strings.Contains(job.Name, "-upgrade") {
		job.Tags = []string{"upgrade"}
	}

	return job
}

// PrintJobsReport renders a filtered summary of matching jobs.
func PrintJobsReport(w http.ResponseWriter, req *http.Request,
	dbc *db.DB,
	currReport,
	twoDayReport,
	prevReport v1sippyprocessing.TestReport,
	manager testidentification.VariantManager) {

	var filter *Filter
	currentPeriod := currReport.ByJob
	twoDayPeriod := twoDayReport.ByJob
	previousPeriod := prevReport.ByJob

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		filter = &Filter{}
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	jobs := jobsAPIResult{}

	// If requesting a two day report, we make the comparison between the last
	// period (typically 7 days) and the last two days.
	// Otherwise the default of last 7 days vs last 14 days.
	var current, previous []v1sippyprocessing.JobResult
	period := req.URL.Query().Get("period")
	switch period {
	case "twoDay":
		current = twoDayPeriod
		previous = currentPeriod
	default:
		current = currentPeriod
		previous = previousPeriod
	}

	for idx, jobResult := range current {
		prevResult := util.FindJobResultForJobName(jobResult.Name, previous)
		job := jobResultToAPI(idx, &current[idx], prevResult)

		if filter != nil {
			include, err := filter.Filter(job)
			if err != nil {
				RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
				return
			}

			if !include {
				continue
			}
		}

		jobs = append(jobs, job)
	}

	if dbc != nil {
		_, err := BuildJobResults(dbc, period)
		if err != nil {
			RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Report generation error: " + err.Error()})
		}
	}

	RespondWithJSON(http.StatusOK, w, jobs.
		sort(req).
		limit(req))
}

func BuildJobResults(dbc *db.DB, period string) (jobsAPIResult, error) {
	now := time.Now()
	jobReports := jobsAPIResult{}

	// TODO: use actual start/num days settings from CLI once we understand what should
	// be happening here, previous seems to include current. See PrepareStandardTestReports.
	// Note that the CLI/most of the code says "start date" for what is actually the
	// end of the date range, and does a walk back.
	currDays := 7
	prevDays := 14
	if period == "twoDay" {
		currDays = 2
		prevDays = 9 // 7 + 2
	}
	// TODO: 2 here because i thought the default was curr period vs prev period,
	// but I don't see where we look further than 7 days in default config.
	// TODO: forcing to 7 days for now
	startDate := time.Now().Add(time.Duration(-1*prevDays*24) * time.Hour)
	boundaryDate := time.Now().Add(time.Duration(-1*currDays*24) * time.Hour)
	klog.Infof("BuildJobResult from %s -> %s -> %s", startDate, boundaryDate, time.Now())

	jobPassesAndFailsQuery := `SELECT 
jr.prow_job_id, 
j.name,
j.release,
j.variants,
j.test_grid_url,
(SELECT COUNT(*) FROM prow_job_runs WHERE prow_job_id = jr.prow_job_id AND succeeded = 't' AND timestamp BETWEEN ? AND ?) as passes, 
(SELECT COUNT(*) FROM prow_job_runs WHERE prow_job_id = jr.prow_job_id AND succeeded = 'f' AND timestamp BETWEEN ? AND ?) as fails,
(SELECT COUNT(*) FROM prow_job_runs WHERE prow_job_id = jr.prow_job_id AND infrastructure_failure = 't' AND timestamp BETWEEN ? AND ?) as infrastructure_fails
FROM prow_job_runs AS jr, prow_jobs AS j 
WHERE jr.timestamp BETWEEN ? AND ?
  AND jr.prow_job_id = j.id
GROUP BY jr.prow_job_id, j.name, j.release, j.test_grid_url, j.variants`
	var currentJobPassFails []jobPassFailCounts
	r := dbc.DB.Raw(jobPassesAndFailsQuery, boundaryDate, now, boundaryDate, now, boundaryDate, now, boundaryDate, now).Scan(&currentJobPassFails)
	if r.Error != nil {
		klog.Error(r.Error)
		return jobReports, r.Error
	}
	klog.Infof("found %d unique jobs in current period", len(currentJobPassFails))

	var prevJobPassFails []jobPassFailCounts
	r = dbc.DB.Raw(jobPassesAndFailsQuery, startDate, now, startDate, now, startDate, now, startDate, now).Scan(&prevJobPassFails)
	if r.Error != nil {
		klog.Error(r.Error)
		return jobReports, r.Error
	}
	klog.Infof("found %d unique jobs in prior period", len(prevJobPassFails))

	for _, jr := range currentJobPassFails {

		runs := jr.Passes + jr.Fails
		var passPercentage float64
		var projectedPassPercentage float64
		if runs > 0 {
			passPercentage = (float64(jr.Passes) / float64(runs)) * 100
			projectedPassPercentage = (float64(jr.Passes+jr.InfrastructureFails) / float64(runs)) * 100
		}

		job := apitype.Job{
			ID:                             jr.ProwJobID,
			Name:                           jr.Name,
			Variants:                       jr.Variants,
			BriefName:                      briefName(jr.Name),
			CurrentPassPercentage:          passPercentage,
			CurrentProjectedPassPercentage: projectedPassPercentage,
			CurrentRuns:                    runs,
		}

		prevJobIdx := findPrevJobPassFails(prevJobPassFails, jr.ProwJobID)

		if prevJobIdx >= 0 {
			prevJob := prevJobPassFails[prevJobIdx]
			prevRuns := prevJob.Passes + prevJob.Fails
			var prevPassPercentage float64
			var prevProjectedPassPercentage float64
			if prevRuns > 0 {
				prevPassPercentage = (float64(prevJob.Passes) / float64(prevRuns)) * 100
				prevProjectedPassPercentage = (float64(prevJob.Passes+prevJob.InfrastructureFails) / float64(prevRuns)) * 100
			}

			job.PreviousPassPercentage = prevPassPercentage
			job.PreviousProjectedPassPercentage = prevProjectedPassPercentage
			job.PreviousRuns = prevRuns
			job.NetImprovement = passPercentage - prevPassPercentage
		}

		//job.Bugs = current.BugList
		//job.AssociatedBugs = current.AssociatedBugList
		job.TestGridURL = jr.TestGridURL

		if strings.Contains(job.Name, "-upgrade") {
			job.Tags = []string{"upgrade"}
		}

		jobReports = append(jobReports, job)
	}

	elapsed := time.Since(now)
	klog.Infof("BuildJobResult completed in: %s", elapsed)

	// TODO: temporary print to json for testing
	for _, jRep := range jobReports {
		if jRep.Name == "periodic-ci-openshift-release-master-nightly-4.10-e2e-vsphere-serial" {
			bytes, err := json.MarshalIndent(jRep, "", "  ")
			if err != nil {
				fmt.Println("Can't serialize", jobReports)
			}
			fmt.Println(string(bytes))
		}
	}
	return jobReports, nil
}

type jobPassFailCounts struct {
	ProwJobID           int
	Name                string
	Release             string
	Variants            pq.StringArray `gorm:"type:text[]"`
	TestGridURL         string
	Passes              int
	Fails               int
	InfrastructureFails int
}

// Find the previous job pass/fail in the slice for the given job ID, if any.
// Returns slice index if found, -1 if not.
func findPrevJobPassFails(jobs []jobPassFailCounts, jobID int) int {
	for i, pjpf := range jobs {
		if pjpf.ProwJobID == jobID {
			return i
		}
	}
	return -1
}

type jobDetail struct {
	Name    string                           `json:"name"`
	Results []v1sippyprocessing.JobRunResult `json:"results"`
}

type jobDetailAPIResult struct {
	Jobs  []jobDetail `json:"jobs"`
	Start int         `json:"start"`
	End   int         `json:"end"`
}

func (jobs jobDetailAPIResult) limit(req *http.Request) jobDetailAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(jobs.Jobs) >= limit {
		jobs.Jobs = jobs.Jobs[:limit]
	}

	return jobs
}

// PrintJobDetailsReport renders the detailed list of runs for matching jobs.
func PrintJobDetailsReport(w http.ResponseWriter, req *http.Request, current, previous []v1sippyprocessing.JobResult) {
	var min, max int
	jobs := make([]jobDetail, 0)
	jobName := req.URL.Query().Get("job")

	for _, jobResult := range current {
		if jobName != "" && !strings.Contains(jobResult.Name, jobName) {
			continue
		}

		prevResult := util.FindJobResultForJobName(jobResult.Name, previous)
		jobRuns := append(jobResult.AllRuns, prevResult.AllRuns...)

		for _, result := range jobRuns {
			if result.Timestamp < min || min == 0 {
				min = result.Timestamp
			}

			if result.Timestamp > max || max == 0 {
				max = result.Timestamp
			}
		}

		jobDetail := jobDetail{
			Name:    jobResult.Name,
			Results: jobRuns,
		}

		jobs = append(jobs, jobDetail)
	}

	RespondWithJSON(http.StatusOK, w, jobDetailAPIResult{
		Jobs:  jobs,
		Start: min,
		End:   max,
	}.limit(req))
}

// PrintPerfscaleWorkloadMetricsReport renders a filtered summary of matching scale jobs.
func PrintPerfscaleWorkloadMetricsReport(w http.ResponseWriter, req *http.Request, release string, currScaleJobReports []workloadmetricsv1.WorkloadMetricsRow) {

	var filter *Filter
	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		filter = &Filter{}
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	filteredScaleJobs := []*workloadmetricsv1.WorkloadMetricsRow{}
	for idx, row := range currScaleJobReports {
		if release != "" && row.Release != release {
			continue
		}

		if filter != nil {
			include, err := filter.Filter(&currScaleJobReports[idx])
			if err != nil {
				RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
				return
			}

			if !include {
				continue
			}
		}

		filteredScaleJobs = append(filteredScaleJobs, &currScaleJobReports[idx])
	}

	RespondWithJSON(http.StatusOK, w, filteredScaleJobs)

}
