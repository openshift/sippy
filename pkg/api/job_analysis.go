package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/openshift/sippy/pkg/util"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type dayResult struct {
	TotalRuns        int                                        `json:"total_runs"`
	ResultCount      map[v1sippyprocessing.JobOverallResult]int `json:"result_count"`
	TestFailureCount map[string]int                             `json:"test_count"`
}

type apiJobAnalysisResult struct {
	ByDay map[string]dayResult `json:"by_day"`
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

	results := apiJobAnalysisResult{
		ByDay: make(map[string]dayResult),
	}

	allJobs := append(curr.ByJob, prev.ByJob...)
	for index, job := range allJobs {
		prevJob := util.FindJobResultForJobName(job.Name, prev.ByJob)
		if filter != nil {
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
			date := time.Unix(int64(run.Timestamp/1000), 0).UTC().Format("2006-01-02")

			var result dayResult
			if _, ok := results.ByDay[date]; !ok {
				result = dayResult{
					ResultCount:      make(map[v1sippyprocessing.JobOverallResult]int),
					TestFailureCount: make(map[string]int),
				}
			} else {
				result = results.ByDay[date]
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

			results.ByDay[date] = result
		}
	}

	RespondWithJSON(http.StatusOK, w, results)
}
