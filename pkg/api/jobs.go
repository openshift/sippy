package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	gosort "sort"
	"strconv"
	"strings"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"k8s.io/klog"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	workloadmetricsv1 "github.com/openshift/sippy/pkg/apis/workloadmetrics/v1"
	"github.com/openshift/sippy/pkg/util"
)

type jobsAPIResult []apitype.Job

const periodTwoDay = "twoDay"

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

	return job
}

// PrintJobsReport renders a filtered summary of matching jobs.
func PrintJobsReport(w http.ResponseWriter, req *http.Request, currReport, twoDayReport, prevReport v1sippyprocessing.TestReport) {

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
	case periodTwoDay:
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
	/*
		currDays := 7
		prevDays := 14
		if period == periodTwoDay {
			currDays = 2
			prevDays = 9 // 7 + 2
		}
	*/

	var jobReports []apitype.Job
	jobsQuery := `WITH results AS (
        select prow_jobs.name as pj_name,
				prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end), 0) as previous_failures,
                coalesce(count(case when timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW() - INTERVAL '7 DAY' then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND timestamp > NOW() - INTERVAL '7 DAY' then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp > NOW() - INTERVAL '7 DAY' then 1 end), 0) as current_fails,        
                coalesce(count(case when timestamp > NOW() - INTERVAL '7 DAY' then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp > NOW() - INTERVAL '7 DAY' then 1 end), 0) as current_infra_fails
        FROM prow_job_runs 
        JOIN prow_jobs 
                ON prow_jobs.id = prow_job_runs.prow_job_id                 
                and timestamp BETWEEN NOW() - INTERVAL '14 DAY' AND NOW()
        group by prow_jobs.name, prow_jobs.variants
)
SELECT *,
	REGEXP_REPLACE(results.pj_name, 'periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-', '') as brief_name,
        current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
        (current_passes + current_infra_fails) * 100.0 / NULLIF(current_runs, 0) AS current_projected_pass_percentage,
        current_fails * 100.0 / NULLIF(current_runs, 0) AS current_failure_percentage,
        previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
        (previous_passes + previous_infra_fails) * 100.0 / NULLIF(previous_runs, 0) AS previous_projected_pass_percentage,
        previous_failures * 100.0 / NULLIF(previous_runs, 0) AS previous_failure_percentage,
        (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
FROM results
JOIN prow_jobs ON prow_jobs.name = results.pj_name;
`
	r := dbc.DB.Raw(jobsQuery).Scan(&jobReports)
	if r.Error != nil {
		klog.Error(r.Error)
		return []apitype.Job{}, r.Error
	}

	elapsed := time.Since(now)
	klog.Infof("BuildJobResult completed in %s with %d results", elapsed, len(jobReports))

	return jobReports, nil
}

func mapProwJobsByID(allProwJobs []models.ProwJob) map[uint]models.ProwJob {
	result := map[uint]models.ProwJob{}
	for _, pj := range allProwJobs {
		result[pj.ID] = pj
	}
	return result
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
