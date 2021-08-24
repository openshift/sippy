package api

import (
	"net/http"
	"time"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type testResultDay struct {
	Runs             int            `json:"runs"`
	FailureCount     int            `json:"failure_count"`
	FailureByVariant map[string]int `json:"failure_by_variant"`
	RunsByVariant    map[string]int `json:"runs_by_variant"`
}

type apiTestByDayresults struct {
	ByDay map[string]testResultDay `json:"by_day"`
}

func PrintTestAnalysisJSON(w http.ResponseWriter, req *http.Request, curr, prev v1sippyprocessing.TestReport) {
	testName := req.URL.Query().Get("test")
	if testName == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "Test name is required.",
		})
		return
	}

	results := apiTestByDayresults{
		ByDay: make(map[string]testResultDay),
	}

	for _, job := range append(curr.ByJob, prev.ByJob...) {
		for _, run := range job.AllRuns {
			date := time.Unix(int64(run.Timestamp/1000), 0).UTC().Format("2006-01-02")
			var result testResultDay
			if _, ok := results.ByDay[date]; !ok {
				result = testResultDay{
					FailureCount:     0,
					FailureByVariant: make(map[string]int),
					RunsByVariant:    make(map[string]int),
				}
			} else {
				result = results.ByDay[date]
			}

			result.Runs++
			for _, variant := range job.Variants {
				if _, ok := result.RunsByVariant[variant]; !ok {
					result.RunsByVariant[variant] = 1
				} else {
					result.RunsByVariant[variant]++
				}
			}

			for _, test := range run.FailedTestNames {
				if test == testName {
					result.FailureCount++

					for _, variant := range job.Variants {
						if _, ok := result.FailureByVariant[variant]; !ok {
							result.FailureByVariant[variant] = 1
						} else {
							result.FailureByVariant[variant]++
						}
					}
				}
			}

			results.ByDay[date] = result
		}
	}

	RespondWithJSON(http.StatusOK, w, results)
}
