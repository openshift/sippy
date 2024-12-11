package api

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
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

func GetTestAnalysisOverallFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	jq := dbc.DB.Table("test_analysis_by_job_by_dates").
		Select(`test_id,
			test_name,
			to_date((date at time zone 'UTC')::text, 'YYYY-MM-DD'::text)::text as date,
			'overall' as group,
			SUM(runs) as runs,
			SUM(passes) as passes,
			SUM(flakes) as flakes,
			SUM(failures) as failures,
			SUM(passes) * 100.0 / NULLIF(SUM(runs), 0) AS pass_percentage,
			SUM(flakes) * 100.0 / NULLIF(SUM(runs), 0) AS flake_percentage,
			SUM(failures) * 100.0 / NULLIF(SUM(runs), 0) AS fail_percentage`).
		Joins("JOIN prow_jobs on prow_jobs.name = job_name").
		Where("test_analysis_by_job_by_dates.release = ?", release).
		Where("test_name = ?", testName).
		Where("date >= ?", time.Now().Add(24*14*time.Hour)).
		Order("date ASC").
		Group("date, test_id, test_name, test_analysis_by_job_by_dates.release")

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
		jq = jq.Where("? != ANY(prow_jobs.variants)", bv)
	}

	for _, av := range allowedVariants {
		jq = jq.Where("? = ANY(prow_jobs.variants)", av)
	}

	r := jq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis by job")
		return nil, r.Error
	}

	result := make(map[string][]CountByDate)
	result["overall"] = rows
	return result, nil
}

func GetTestAnalysisByJobFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	results := make(map[string][]CountByDate)

	overallResult, err := GetTestAnalysisOverallFromDB(dbc, filters, release, testName, reportEnd)
	if err != nil {
		return nil, err
	}
	if overall, ok := overallResult["overall"]; ok {
		results["overall"] = overall
	}

	jq := dbc.DB.Table("test_analysis_by_job_by_dates").
		Select(`test_id,
			test_name,
			to_date((date at time zone 'UTC')::text, 'YYYY-MM-DD'::text)::text as date,
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
		Where("date <= ?", reportEnd).
		Where("date >= ?", reportEnd.Add(24*14*time.Hour)).
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

	overallResult, err := GetTestAnalysisOverallFromDB(dbc, filters, release, testName, reportEnd)
	if err != nil {
		return nil, err
	}
	if overall, ok := overallResult["overall"]; ok {
		results["overall"] = overall
	}

	vq := dbc.DB.Table("prow_test_analysis_by_variant_14d_view").
		Where("release = ?", release).
		Where("test_name = ?", testName).
		Where("date <= ?", reportEnd).
		Select(`to_date((date at time zone 'UTC')::text, 'YYYY-MM-DD'::text)::text as date,
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
