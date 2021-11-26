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
	"github.com/openshift/sippy/pkg/db/models"
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

	RespondWithJSON(http.StatusOK, w, jobs.
		sort(req).
		limit(req))
}

// PrintDBJobsReport renders a filtered summary of matching jobs.
func PrintDBJobsReport(w http.ResponseWriter, req *http.Request,
	dbc *db.DB, release string) {

	var filter *Filter

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		filter = &Filter{}
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	period := req.URL.Query().Get("period")
	jobsResult, err := BuildJobResults(dbc, period, release)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, jobsResult.
		sort(req).
		limit(req))
}

func BuildJobResults(dbc *db.DB, period, release string) (jobsAPIResult, error) {
	now := time.Now()

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

	// Load all ProwJobs and create a map of db ID to ProwJob:
	var allJobs []models.ProwJob
	dbr := dbc.DB.Find(&allJobs).Where("release = ?", release)
	if dbr.Error != nil {
		klog.Error(dbr)
		return []apitype.Job{}, dbr.Error
	}
	allJobsByID := mapProwJobsByID(allJobs)
	klog.V(3).Infof("Found %d jobs", len(allJobs))

	// Load all ProwJobRuns in our period timestamp range:
	var jobRunsInPeriod []models.ProwJobRun
	dbr = dbc.DB.
		Joins("JOIN prow_jobs ON prow_jobs.id = prow_job_runs.prow_job_id").
		Where("timestamp BETWEEN ? AND ?", startDate, now).
		Where("prow_jobs.release = ?", release).Find(&jobRunsInPeriod)
	if dbr.Error != nil {
		klog.Error(dbr)
		return []apitype.Job{}, dbr.Error
	}
	klog.V(3).Infof("Found %d job runs in period between %s and %s",
		len(jobRunsInPeriod),
		startDate.Format(time.RFC3339),
		now.Format(time.RFC3339))

	// Build a map of job ID to the API job result we'll return.
	// Iterate all job results, incrementing counters on the job result we'll return.
	apiJobResults := map[uint]*apitype.Job{}
	for _, jobRun := range jobRunsInPeriod {
		job := allJobsByID[jobRun.ProwJobID]
		if _, ok := apiJobResults[job.ID]; !ok {

			apiJobResults[job.ID] = &apitype.Job{
				ID:          int(job.ID),
				Name:        job.Name,
				BriefName:   briefName(job.Name),
				Variants:    job.Variants,
				TestGridURL: job.TestGridURL,
			}
		}

		// NOTE: Previous period includes current, thus all results increment previous counters,
		// and if the timestamp is *after* our boundary date, they also increment current counters.
		jobAcc := apiJobResults[job.ID]

		jobAcc.PreviousRuns++
		if jobRun.Timestamp.After(boundaryDate) {
			jobAcc.CurrentRuns++
		}

		if jobRun.Succeeded {
			jobAcc.PreviousPasses++
			if jobRun.Timestamp.After(boundaryDate) {
				jobAcc.CurrentPasses++
			}
		}

		if jobRun.Failed {
			jobAcc.PreviousFails++
			if jobRun.InfrastructureFailure {
				jobAcc.PreviousInfraFails++
			}
			if jobRun.Timestamp.After(boundaryDate) {
				jobAcc.CurrentFails++
				if jobRun.InfrastructureFailure {
					jobAcc.CurrentInfraFails++
				}
			}
		}

	}

	// Now that we've processed all job results, calculate percentages on the results we'll return:
	finalAPIJobResult := make([]apitype.Job, 0, len(apiJobResults))
	for _, v := range apiJobResults {
		if v.CurrentRuns > 0 {
			v.CurrentPassPercentage = float64(v.CurrentPasses) / float64(v.CurrentRuns) * 100
			v.CurrentProjectedPassPercentage = float64(v.CurrentPasses+v.CurrentInfraFails) / float64(v.CurrentRuns) * 100
		}
		if v.PreviousRuns > 0 {
			v.PreviousPassPercentage = float64(v.PreviousPasses) / float64(v.PreviousRuns) * 100
			v.PreviousProjectedPassPercentage = float64(v.PreviousPasses+v.PreviousInfraFails) / float64(v.PreviousRuns) * 100
		}
		v.NetImprovement = v.CurrentPassPercentage - v.PreviousPassPercentage
		finalAPIJobResult = append(finalAPIJobResult, *v)
	}

	elapsed := time.Since(now)
	klog.Infof("BuildJobResult completed in %s with %d results", elapsed, len(finalAPIJobResult))

	// TODO: temporary print to json for testing
	for _, jRep := range finalAPIJobResult {
		if jRep.Name == "periodic-ci-openshift-release-master-nightly-4.10-e2e-vsphere-serial" {
			bytes, err := json.MarshalIndent(jRep, "", "  ")
			if err != nil {
				fmt.Println("Can't serialize", jRep)
			}
			fmt.Println(string(bytes))
		}
	}
	return finalAPIJobResult, nil
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

func mapProwJobsByID(allProwJobs []models.ProwJob) map[uint]models.ProwJob {
	result := map[uint]models.ProwJob{}
	for _, pj := range allProwJobs {
		result[pj.ID] = pj
	}
	return result
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
