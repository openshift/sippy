package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/openshift/sippy/pkg/util"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type analysisResult struct {
	TotalRuns        int                                        `json:"total_runs"`
	ResultCount      map[v1sippyprocessing.JobOverallResult]int `json:"result_count"`
	TestFailureCount map[string]int                             `json:"test_count"`
}

type apiJobAnalysisResult struct {
	ByPeriod map[string]analysisResult `json:"by_period"`
}

func PrintJobAnalysisJSON(w http.ResponseWriter, req *http.Request, curr, prev v1sippyprocessing.TestReport) {
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
	if period == "" {
		period = "day"
	}

	results := apiJobAnalysisResult{
		ByPeriod: make(map[string]analysisResult),
	}

	allJobs := append(curr.ByJob, prev.ByJob...)
	var timestampFilter *Filter
	for index, job := range allJobs {
		prevJob := util.FindJobResultForJobName(job.Name, prev.ByJob)
		if filter != nil {
			newItems := make([]FilterItem, 0)
			timestampItems := make([]FilterItem, 0)
			for _, item := range filter.Items {
				if item.Field != "timestamp" {
					newItems = append(newItems, item)
				} else {
					timestampItems = append(timestampItems, item)
				}
				filter.Items = newItems

				if len(timestampItems) > 0 {
					timestampFilter = &Filter{
						Items:        timestampItems,
						LinkOperator: filter.LinkOperator,
					}
				}
			}

			include, err := filter.Filter(jobResultToAPI(index, &allJobs[index], prevJob))
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
			if period == "day" {
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
