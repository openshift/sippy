package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	gosort "sort"
	"strconv"
	"strings"
	"text/template"
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

	jobsResult, err := BuildJobResults(dbc, period, release, filter)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job report:" + err.Error()})
		return
	}

	RespondWithJSON(http.StatusOK, w, jobsResult.
		sort(req).
		limit(req))
}

func BuildJobResults(dbc *db.DB, period, release string, filter *Filter) (jobsAPIResult, error) {
	now := time.Now()

	var jobReports []apitype.Job
	jobsQuery := `WITH results AS (
        select prow_jobs.name as pj_name,
				prow_jobs.variants as pj_variants,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN NOW() - INTERVAL '{{.startInterval}}' AND NOW() - INTERVAL '{{.boundaryInterval}}' then 1 end), 0) as previous_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN NOW() - INTERVAL '{{.startInterval}}' AND NOW() - INTERVAL '{{.boundaryInterval}}' then 1 end), 0) as previous_failures,
                coalesce(count(case when timestamp BETWEEN NOW() - INTERVAL '{{.startInterval}}' AND NOW() - INTERVAL '{{.boundaryInterval}}' then 1 end), 0) as previous_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN NOW() - INTERVAL '{{.startInterval}}' AND NOW() - INTERVAL '{{.boundaryInterval}}' then 1 end), 0) as previous_infra_fails,
                coalesce(count(case when succeeded = true AND timestamp BETWEEN NOW() - INTERVAL '{{.boundaryInterval}}' AND NOW() - INTERVAL '{{.endInterval}}' then 1 end), 0) as current_passes,
                coalesce(count(case when succeeded = false AND timestamp BETWEEN NOW() - INTERVAL '{{.boundaryInterval}}' AND NOW() - INTERVAL '{{.endInterval}}' then 1 end), 0) as current_fails,        
                coalesce(count(case when timestamp BETWEEN NOW() - INTERVAL '{{.boundaryInterval}}' AND NOW() - INTERVAL '{{.endInterval}}' then 1 end), 0) as current_runs,
                coalesce(count(case when infrastructure_failure = true AND timestamp BETWEEN NOW() - INTERVAL '{{.boundaryInterval}}' AND NOW() - INTERVAL '{{.endInterval}}' then 1 end), 0) as current_infra_fails
        FROM prow_job_runs 
        JOIN prow_jobs 
                ON prow_jobs.id = prow_job_runs.prow_job_id                 
                and timestamp BETWEEN NOW() - INTERVAL '{{.startInterval}}' AND NOW() - INTERVAL '{{.endInterval}}'
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
JOIN prow_jobs ON prow_jobs.name = results.pj_name
`
	// Using a string template here to replace params we re-use many times in the query. This is sub-optimal
	// but all attempts to use a named param with gorm have hit a weird error likely in the postgresql libraries
	// complaining about a syntax error near $1 (which doesn't exist in the logged query, and the logged query
	// works fine when run separately). For now it's text templating.
	//
	// However this likely means we need to watch out for sql injection, so any test getting subbed into the query
	// should not come directly from the API request's query parameters. We must translate/validate explicitly and
	// carefully.
	intervals := map[string]interface{}{"startInterval": "14 DAY", "boundaryInterval": "7 DAY", "endInterval": "14 DAY"}
	t := template.Must(template.New("").Parse(jobsQuery))
	sb := &strings.Builder{}
	t.Execute(sb, intervals)

	r := dbc.DB.Raw(sb.String()).Scan(&jobReports)
	if r.Error != nil {
		klog.Error(r.Error)
		return []apitype.Job{}, r.Error
	}

	// Apply filtering to what we pulled from the db. Perfect world we'd incorporate this into the query instead.
	filteredJobReports := make([]apitype.Job, 0, len(jobReports))
	for _, jobReport := range jobReports {
		if filter != nil {
			include, err := filter.Filter(jobReport)
			if err != nil {
				return []apitype.Job{}, err
			}

			if !include {
				continue
			}
		}

		filteredJobReports = append(filteredJobReports, jobReport)
	}

	elapsed := time.Since(now)
	klog.Infof("BuildJobResult completed in %s with %d results from db, filtered down to %s", elapsed, len(jobReports), len(filteredJobReports))

	return filteredJobReports, nil
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
