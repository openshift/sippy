package api

import (
	"net/http"
	"time"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type counts struct {
	Runs     int `json:"runs"`
	Failures int `json:"failures"`
}

type testResultDay struct {
	Overall   counts             `json:"overall"`
	ByVariant map[string]*counts `json:"by_variant"`
	ByJob     map[string]*counts `json:"by_job"`
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
					Overall: counts{
						Failures: 0,
						Runs:     0,
					},
					ByVariant: make(map[string]*counts),
					ByJob:     make(map[string]*counts),
				}
			} else {
				result = results.ByDay[date]
			}

			result.Overall.Runs++

			// Runs by job
			if _, ok := result.ByJob[briefName(job.Name)]; !ok {
				result.ByJob[briefName(job.Name)] = &counts{
					Runs: 1,
				}
			} else {
				result.ByJob[briefName(job.Name)].Runs++
			}

			// Runs by variant
			for _, variant := range job.Variants {
				if _, ok := result.ByVariant[variant]; !ok {
					result.ByVariant[variant] = &counts{
						Runs: 1,
					}
				} else {
					result.ByVariant[variant].Runs++
				}
			}

			for _, test := range run.FailedTestNames {
				if test == testName {
					result.Overall.Failures++
					result.ByJob[briefName(job.Name)].Failures++
					for _, variant := range job.Variants {
						result.ByVariant[variant].Failures++
					}
				}
			}

			results.ByDay[date] = result
		}
	}

	RespondWithJSON(http.StatusOK, w, results)
}
