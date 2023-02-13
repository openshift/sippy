package api

import (
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/filter"
)

type CountByDate struct {
	Date            string  `json:"date"`
	Group           string  `json:"group"`
	PassPercentage  float64 `json:"pass_percentage"`
	FlakePercentage float64 `json:"flake_percentage"`
	FailPercentage  float64 `json:"fail_percentage"`
	Runs            int     `json:"runs"`
	Passes          int     `json:"passes"`
	Flakes          int     `json:"flakes"`
	Failures        int     `json:"failures"`
}

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

func GetTestAnalysisByJobFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	results := make(map[string][]CountByDate)

	jq := dbc.DB.Table("prow_test_analysis_by_job_14d_matview").
		Select(`test_id,
			test_name,
			to_date(date::text, 'YYYY-MM-DD'::text)::text as date,
			prow_jobs.release,
			job_name as group,
			runs,
			passes,
			flakes,
			failures,
			variants,
			passes * 100.0 / NULLIF(runs, 0) AS pass_percentage,
			flakes * 100.0 / NULLIF(runs, 0) AS flake_percentage,
			failures * 100.0 / NULLIF(runs, 0) AS fail_percentage`).
		Joins("INNER JOIN prow_jobs on prow_jobs.name = job_name").
		Where("prow_jobs.release = ?", release).
		Where("test_name = ?", testName).
		Order("date ASC")

	var allowedVariants, blockedVariants []string
	if filters != nil {
		for _, f := range filters.Items {
			if f.Field == "variants" {
				if f.Not {
					blockedVariants = append(blockedVariants, f.Value)
				} else {
					allowedVariants = append(allowedVariants, f.Value)
				}
			}
		}
	}

	for _, bv := range blockedVariants {
		jq = jq.Where("? != ANY(variants)", bv)
	}

	for _, av := range allowedVariants {
		jq = jq.Where("? = ANY(variants)", av)
	}

	r := jq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by job")
		return nil, r.Error
	}

	for _, row := range rows {
		results[row.Group] = append(results[row.Group], row)
	}

	return results, nil
}

func GetTestAnalysisByVariantFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	results := make(map[string][]CountByDate)

	vq := dbc.DB.Table("prow_test_analysis_by_variant_14d_matview").
		Where("release = ?", release).
		Where("test_name = ?", testName).
		Where("date < ?", reportEnd).
		Select(`to_date(date::text, 'YYYY-MM-DD'::text)::text as date,
			variant as group,
			runs,
			passes,
			flakes,
			failures,
			passes * 100.0 / NULLIF(runs, 0) AS pass_percentage,
			flakes * 100.0 / NULLIF(runs, 0) AS flake_percentage,
			failures * 100.0 / NULLIF(runs, 0) AS fail_percentage`).
		Order("date ASC")

	var allowedVariants, blockedVariants []string
	if filters != nil {
		for _, f := range filters.Items {
			if f.Field == "variants" {
				if f.Not {
					blockedVariants = append(blockedVariants, f.Value)
				} else {
					allowedVariants = append(allowedVariants, f.Value)
				}
			}
		}

		if len(blockedVariants) > 0 {
			vq = vq.Where("variant NOT IN ?", blockedVariants)
		}

		if len(allowedVariants) > 0 {
			vq = vq.Where("variant IN ?", allowedVariants)
		}
	}

	r := vq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by variant")
		return nil, r.Error
	}

	for _, row := range rows {
		results[row.Group] = append(results[row.Group], row)
	}

	return results, nil
}

func PrintTestAnalysisJSONFromDB(dbc *db.DB, w http.ResponseWriter, req *http.Request, release, testName string, reportEnd time.Time) error {
	results := apiTestByDayresults{
		ByDay: make(map[string]testResultDay),
	}

	filters, err := filter.ExtractFilters(req)
	if err != nil {
		return err
	}

	// We're using two views, one for by variant and one for by job, thus we will do
	// two queries and combine the results into the struct we need.
	var byVariantAnalysisRows []models.TestAnalysisRow
	vq := dbc.DB.Table("prow_test_analysis_by_variant_14d_matview").
		Where("release = ?", release).
		Where("test_name = ?", testName).
		Select(`test_id,
	       test_name,
    	   date,
	       release,
	       variant,
	       runs,
	       passes,
	       flakes,
    	   failures`).
		Group("test_id, test_name, date, release, variant, runs, passes, flakes, failures")

	var allowedVariants, blockedVariants []string
	if filters != nil {
		for _, f := range filters.Items {
			if f.Field == "variants" {
				if f.Not {
					blockedVariants = append(blockedVariants, f.Value)
				} else {
					allowedVariants = append(allowedVariants, f.Value)
				}
			}
		}

		if len(blockedVariants) > 0 {
			vq = vq.Where("variant NOT IN ?", blockedVariants)
		}

		if len(allowedVariants) > 0 {
			vq = vq.Where("variant IN ?", allowedVariants)
		}
	}

	r := vq.Scan(&byVariantAnalysisRows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by variant")
		return r.Error
	}

	// Reset analysis rows and now we query from the by job view
	byJobAnalysisRows := []models.TestAnalysisRow{}
	jq := dbc.DB.Table("prow_test_analysis_by_job_14d_matview").
		Select(`test_id,
			test_name,
			date,
			prow_jobs.release,
			job_name,
			runs,
			passes,
			flakes,
			failures,
			ARRAY_AGG(variants) as variants`).
		Joins("INNER JOIN prow_jobs on prow_jobs.name = job_name").
		Where("prow_jobs.release = ?", release).
		Where("test_name = ?", testName).
		Group("test_id, test_name, date, prow_jobs.release, job_name, runs, passes, flakes, failures")

	for _, bv := range blockedVariants {
		jq = jq.Where("? != ANY(variants)", bv)
	}

	for _, av := range allowedVariants {
		jq = jq.Where("? = ANY(variants)", av)
	}

	r = jq.Scan(&byJobAnalysisRows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by job")
		return r.Error
	}

	allRows := append(byVariantAnalysisRows, byJobAnalysisRows...)

	for _, row := range allRows {

		// need to know the report end, if the date is greater than that skip the row
		// tried modifying the query but still get extra rows / days
		// Where("date < ? ", reportEnd.UTC()).
		if row.Date.After(reportEnd) {
			continue
		}

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
			if _, ok := dayResult.ByVariant[row.Variant]; ok {
				// Should not happen if our query is correct.
				return fmt.Errorf("test '%s' showed duplicate variant '%s' row on date '%s'", testName, row.Variant, date)
			}
			dayResult.ByVariant[row.Variant] = &counts{
				Runs:     row.Runs,
				Passes:   row.Passes,
				Flakes:   row.Flakes,
				Failures: row.Failures,
			}
		} else {
			// Assuming that if row.Variant is not set, row.JobName must be.
			if _, ok := dayResult.ByJob[briefName(row.JobName)]; !ok {
				dayResult.ByJob[briefName(row.JobName)] = &counts{
					Runs:     row.Runs,
					Passes:   row.Passes,
					Flakes:   row.Flakes,
					Failures: row.Failures,
				}
			} else {
				// the briefName() function will map to the same value for some jobs, this appears to be intentional.
				// As such if we see a brief job name that we already have, we need to increment it's counters.
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
