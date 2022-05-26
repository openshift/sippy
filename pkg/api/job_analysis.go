package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/util"
)

const PeriodDay = "day"
const PeriodHour = "hour"

type analysisResult struct {
	TotalRuns        int                                        `json:"total_runs"`
	ResultCount      map[v1sippyprocessing.JobOverallResult]int `json:"result_count"`
	TestFailureCount map[string]int                             `json:"test_count"`
}

type apiJobAnalysisResult struct {
	ByPeriod map[string]analysisResult `json:"by_period"`
}

func PrintJobAnalysisJSON(w http.ResponseWriter, req *http.Request, curr, prev v1sippyprocessing.TestReport) {
	var fil *filter.Filter

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		fil = &filter.Filter{}
		if err := json.Unmarshal([]byte(queryFilter), fil); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = PeriodDay
	} else if period != PeriodDay && period != PeriodHour {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Period must be one of: day, hour"})
		return
	}

	results := apiJobAnalysisResult{
		ByPeriod: make(map[string]analysisResult),
	}

	allJobs := append(curr.ByJob, prev.ByJob...)
	var timestampFilter *filter.Filter
	for index, job := range allJobs {
		prevJob := util.FindJobResultForJobName(job.Name, prev.ByJob)
		if fil != nil {
			newItems := make([]filter.FilterItem, 0)
			timestampItems := make([]filter.FilterItem, 0)
			for _, item := range fil.Items {
				if item.Field != "timestamp" {
					newItems = append(newItems, item)
				} else {
					timestampItems = append(timestampItems, item)
				}
				fil.Items = newItems

				if len(timestampItems) > 0 {
					timestampFilter = &filter.Filter{
						Items:        timestampItems,
						LinkOperator: fil.LinkOperator,
					}
				}
			}

			include, err := fil.Filter(jobResultToAPI(index, &allJobs[index], prevJob))
			if err != nil {
				RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
				return
			}

			if !include {
				continue
			}
		}

		for _, run := range job.AllRuns {
			if timestampFilter != nil {
				include, err := timestampFilter.Filter(jobRunToAPIJobRun(0, job, run))
				if err != nil {
					RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
					return
				}

				if !include {
					continue
				}
			}

			var date string
			if period == PeriodDay {
				date = time.Unix(int64(run.Timestamp/1000), 0).UTC().Format("2006-01-02")
			} else {
				date = time.Unix(int64(run.Timestamp/1000), 0).UTC().Format("2006-01-02 15:00")
			}

			var result analysisResult
			if _, ok := results.ByPeriod[date]; !ok {
				result = analysisResult{
					ResultCount:      make(map[v1sippyprocessing.JobOverallResult]int),
					TestFailureCount: make(map[string]int),
				}
			} else {
				result = results.ByPeriod[date]
			}

			result.TotalRuns++

			// Count failures for each test
			for _, test := range run.FailedTestNames {
				if _, ok := result.TestFailureCount[test]; !ok {
					result.TestFailureCount[test] = 1
				} else {
					result.TestFailureCount[test]++
				}
			}

			// Count results for each job
			if _, ok := result.ResultCount[run.OverallResult]; !ok {
				result.ResultCount[run.OverallResult] = 1
			} else {
				result.ResultCount[run.OverallResult]++
			}

			results.ByPeriod[date] = result
		}
	}

	RespondWithJSON(http.StatusOK, w, results)
}

func PrintJobAnalysisJSONFromDB(w http.ResponseWriter, req *http.Request, dbc *db.DB, release string) {
	fil, err := filter.ExtractFilters(req)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
		return
	}

	// This API is a bit special, since we are largely interested in filtering the jobs list,
	// but there's a case for filtering by the time stamp on a job run.
	jobRunsFilter := &filter.Filter{
		LinkOperator: fil.LinkOperator,
	}
	jobFilter := &filter.Filter{
		LinkOperator: fil.LinkOperator,
	}

	for _, f := range fil.Items {
		if f.Field == "timestamp" {
			ms, err := strconv.ParseInt(f.Value, 0, 64)
			if err != nil {
				RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": err.Error()})
				return
			}

			f.Value = time.Unix(0, ms*int64(time.Millisecond)).Format("2006-01-02T15:04:05-0700")
			jobRunsFilter.Items = append(jobRunsFilter.Items, f)
		} else {
			jobFilter.Items = append(jobFilter.Items, f)
		}
	}

	period := req.URL.Query().Get("period")
	if period == "" {
		period = PeriodDay
	}

	table, err := jobResultsFromDB(req, dbc.DB, release)
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job analysis report:" + table.Error.Error()})
		return
	}

	q, err := filter.ApplyFilters(req, jobFilter, "name", table, apitype.Job{})
	if err != nil {
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": "Error building job run report:" + err.Error()})
		return
	}

	jobs := make([]int, 0)
	q.Pluck("id", &jobs)

	// Next is sum up individual job results
	type resultSum struct {
		Period         time.Time
		TotalRuns      int
		Success        int `gorm:"column:S"`
		Running        int `gorm:"column:R"`
		FailureE2E     int `gorm:"column:F"`
		FailureOther   int `gorm:"column:f"`
		Upgrade        int `gorm:"column:U"`
		Install        int `gorm:"column:I"`
		Infrastructure int `gorm:"column:N"`
		NoResult       int `gorm:"column:n"`
	}
	sums := make([]resultSum, 0)
	sumResults := dbc.DB.Table("prow_job_runs").
		Select(fmt.Sprintf(`date_trunc('%s', timestamp)        AS period,
	           count(*)                                              AS total_runs,
	           sum(case when overall_result = 'S' then 1 else 0 end) AS "S",
	           sum(case when overall_result = 'F' then 1 else 0 end) AS "F",
	           sum(case when overall_result = 'f' then 1 else 0 end) AS "f",
	           sum(case when overall_result = 'U' then 1 else 0 end) AS "U",
	           sum(case when overall_result = 'I' then 1 else 0 end) AS "I",
	           sum(case when overall_result = 'N' then 1 else 0 end) AS "N",
	           sum(case when overall_result = 'n' then 1 else 0 end) AS "n",
	           sum(case when overall_result = 'R' then 1 else 0 end) AS "R"`, period)).
		Joins("INNER JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where("prow_jobs.id IN ?", jobs).
		Group(fmt.Sprintf(`date_trunc('%s', timestamp)`, period))

	jobRunsFilter.ToSQL(sumResults, apitype.JobRun{}).Scan(&sums)

	// collect the results
	results := apiJobAnalysisResult{
		ByPeriod: make(map[string]analysisResult),
	}
	var formatter string
	if period == PeriodDay {
		formatter = "2006-01-02"
	} else {
		formatter = "2006-01-02 15:00"
	}

	for _, sum := range sums {
		results.ByPeriod[sum.Period.UTC().Format(formatter)] = analysisResult{
			TotalRuns: sum.TotalRuns,
			ResultCount: map[v1sippyprocessing.JobOverallResult]int{
				v1sippyprocessing.JobSucceeded:             sum.Success,
				v1sippyprocessing.JobRunning:               sum.Running,
				v1sippyprocessing.JobTestFailure:           sum.FailureE2E,
				v1sippyprocessing.JobInfrastructureFailure: sum.Infrastructure,
				v1sippyprocessing.JobUpgradeFailure:        sum.Upgrade,
				v1sippyprocessing.JobInstallFailure:        sum.Install,
				v1sippyprocessing.JobNoResults:             sum.NoResult,
				v1sippyprocessing.JobUnknown:               sum.FailureOther,
			},
			TestFailureCount: map[string]int{},
		}
	}
	type testResult struct {
		Period   time.Time
		TestName string
		Count    int
	}
	tr := make([]testResult, 0)

	jr := dbc.DB.Table("prow_job_failed_tests_by_day_matview").
		Select("period, test_name, count").
		Where("prow_job_id IN ?", jobs)

	jobRunsFilter.ToSQL(jr, apitype.JobRun{}).Scan(&tr)

	for _, t := range tr {
		dateKey := t.Period.UTC().Format(formatter)
		results.ByPeriod[dateKey].TestFailureCount[t.TestName] = t.Count
	}

	RespondWithJSON(http.StatusOK, w, results)
}
