package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"k8s.io/klog"
)

type counts struct {
	Runs     int `json:"runs"`
	Failures int `json:"failures"`

	Passes int `json:"passes"`
	Flakes int `json:"flakes"`
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

func PrintTestAnalysisJSONFromDB(db *db.DB, w http.ResponseWriter, release, testName string) error {
	start := time.Now()
	results := apiTestByDayresults{
		ByDay: make(map[string]testResultDay),
	}

	// We're using two views, one for by variant and one for by job, thus we will do
	// two queries and combine the results into the struct we need.

	var analysisRows []models.TestAnalysisByVariantRow
	q := `
SELECT test_name,
       date,
       release, 
       variant, 
       runs,
       passes,
       flakes,
       failures
FROM prow_test_analysis_by_variant_14d_matview
WHERE release = @release AND test_name = @testsubstrings
GROUP BY test_name, date, release, variant, runs, passes, flakes, failures
`
	r := db.DB.Raw(q,
		sql.Named("release", release),
		sql.Named("testsubstrings", testName)).Scan(&analysisRows)
	if r.Error != nil {
		klog.Error(r.Error)
		return r.Error
	}

	elapsed := time.Since(start)

	klog.Infof("Queried test analysis rows in %s with %d results from db", elapsed, len(analysisRows))

	for _, row := range analysisRows {
		date := row.Date.Format("2006-01-02")

		var dayResult testResultDay
		if _, ok := results.ByDay[date]; !ok {
			dayResult = testResultDay{
				Overall: counts{
					Failures: 0,
					Runs:     0,
					Passes:   0,
					Flakes:   0,
				},
				ByVariant: make(map[string]*counts),
				ByJob:     make(map[string]*counts),
			}
		} else {
			dayResult = results.ByDay[date]
		}

		// TODO: not iterating job runs here anymore.
		//dayResult.Overall.Runs++

		/*
			// Runs by job
			if _, ok := dayResult.ByJob[briefName(job.Name)]; !ok {
				dayResult.ByJob[briefName(job.Name)] = &counts{
					Runs: 1,
				}
			} else {
				dayResult.ByJob[briefName(job.Name)].Runs++
			}
		*/

		// Runs by variant
		if _, ok := dayResult.ByVariant[row.Variant]; !ok {
			dayResult.ByVariant[row.Variant] = &counts{
				Runs:     row.Runs,
				Passes:   row.Passes,
				Flakes:   row.Flakes,
				Failures: row.Failures,
			}
		} else {
			// Should not happen if our query is correct.
			return fmt.Errorf("error")
		}

		results.ByDay[date] = dayResult
	}

	RespondWithJSON(http.StatusOK, w, results)
	return nil
}
