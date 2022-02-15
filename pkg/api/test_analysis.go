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

			// Warning: this is incrementing overall job runs regardless if the job actually included our test or not.
			// Can lead to wildly inaccurate results. New version of API below has this fixed.
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
	results := apiTestByDayresults{
		ByDay: make(map[string]testResultDay),
	}

	// We're using two views, one for by variant and one for by job, thus we will do
	// two queries and combine the results into the struct we need.

	var byVariantAnalysisRows []models.TestAnalysisRow
	q1 := `
SELECT test_id, 
       test_name,
       date,
       release, 
       variant, 
       runs,
       passes,
       flakes,
       failures
FROM prow_test_analysis_by_variant_14d_matview
WHERE release = @release AND test_name = @testname
GROUP BY test_id, test_name, date, release, variant, runs, passes, flakes, failures
`
	r := db.DB.Raw(q1,
		sql.Named("release", release),
		sql.Named("testname", testName)).Scan(&byVariantAnalysisRows)
	if r.Error != nil {
		klog.Error(r.Error)
		return r.Error
	}

	// Reset analysis rows and now we query from the by job view
	byJobAnalysisRows := []models.TestAnalysisRow{}
	q2 := `
SELECT test_id, 
       test_name,
       date,
       release, 
       job_name, 
       runs,
       passes,
       flakes,
       failures
FROM prow_test_analysis_by_job_14d_matview
WHERE release = @release AND test_name = @testname
GROUP BY test_id, test_name, date, release, job_name, runs, passes, flakes, failures
`
	r = db.DB.Raw(q2,
		sql.Named("release", release),
		sql.Named("testname", testName)).Scan(&byJobAnalysisRows)
	if r.Error != nil {
		klog.Error(r.Error)
		return r.Error
	}

	allRows := append(byVariantAnalysisRows, byJobAnalysisRows...)

	for _, row := range allRows {
		date := row.Date.Format("2006-01-02")

		var dayResult testResultDay
		if _, ok := results.ByDay[date]; !ok {
			dayResult = testResultDay{
				ByVariant: make(map[string]*counts),
				ByJob:     make(map[string]*counts),
			}
		} else {
			dayResult = results.ByDay[date]
		}

		// We're reusing the same model object when we query by variant or job, so we fork based on what field is set
		if row.Variant != "" {
			if _, ok := dayResult.ByVariant[row.Variant]; !ok {
				dayResult.ByVariant[row.Variant] = &counts{
					Runs:     row.Runs,
					Passes:   row.Passes,
					Flakes:   row.Flakes,
					Failures: row.Failures,
				}
			} else {
				// Should not happen if our query is correct.
				return fmt.Errorf("test '%s' showed duplicate variant '%s' row on date '%s'", testName, row.Variant, date)
			}
		} else {
			// Assuming that if row.Variant is not set, row.JobName must be.
			if _, ok := dayResult.ByJob[briefName(row.JobName)]; !ok {
				//klog.Infof("adding job %s (briefname: %s) on date %s", row.JobName, briefName(row.JobName), date)
				dayResult.ByJob[briefName(row.JobName)] = &counts{
					Runs:     row.Runs,
					Passes:   row.Passes,
					Flakes:   row.Flakes,
					Failures: row.Failures,
				}
			} else {
				// the briefName() function will map to the same value for some jobs, this appears to be intentional.
				// As such if we see a brief job name that we already have, we need to increment it's counters.
				//klog.Infof("incrementing counters for job %s (briefname: %s) on date %s", row.JobName, briefName(row.JobName), date)
				dayResult.ByJob[briefName(row.JobName)].Runs += row.Runs
				dayResult.ByJob[briefName(row.JobName)].Passes += row.Passes
				dayResult.ByJob[briefName(row.JobName)].Flakes += row.Flakes
				dayResult.ByJob[briefName(row.JobName)].Failures += row.Failures
			}

			// Increment our overall counter using the rows with job names, as these are distinct.
			// (unlike variants which can overlap and would cause double counted test runs)
			dayResult.Overall.Runs += row.Runs
			dayResult.Overall.Passes += row.Passes
			dayResult.Overall.Flakes += row.Flakes
			dayResult.Overall.Failures += row.Failures
		}

		results.ByDay[date] = dayResult
	}

	RespondWithJSON(http.StatusOK, w, results)
	return nil
}
