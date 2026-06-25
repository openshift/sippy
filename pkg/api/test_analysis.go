package api

import (
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

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

func extractVariantFilters(filters *filter.Filter) (allowed, blocked []string) {
	if filters == nil {
		return nil, nil
	}
	for _, f := range filters.Items {
		if f.Field == "variants" {
			if f.Not {
				blocked = append(blocked, f.Value)
			} else {
				allowed = append(allowed, f.Value)
			}
		}
	}
	return allowed, blocked
}

func applyVariantCombinationFilters(q *gorm.DB, allowed, blocked []string) *gorm.DB {
	for _, bv := range blocked {
		q = q.Where("tds.variant_combination_id NOT IN (SELECT id FROM variant_combinations WHERE ? = ANY(variants))", bv)
	}
	for _, av := range allowed {
		q = q.Where("tds.variant_combination_id IN (SELECT id FROM variant_combinations WHERE ? = ANY(variants))", av)
	}
	return q
}

var testAnalysisAggColumns = []string{
	"SUM(tds.runs) AS runs",
	"SUM(tds.successes) AS passes",
	"SUM(tds.flakes) AS flakes",
	"SUM(tds.failures) AS failures",
	"SUM(tds.successes) * 100.0 / NULLIF(SUM(tds.runs), 0) AS pass_percentage",
	"SUM(tds.flakes) * 100.0 / NULLIF(SUM(tds.runs), 0) AS flake_percentage",
	"SUM(tds.failures) * 100.0 / NULLIF(SUM(tds.runs), 0) AS fail_percentage",
}

func selectColumns(columns ...string) string {
	return strings.Join(append(columns, testAnalysisAggColumns...), ", ")
}

func testAnalysisBaseQuery(dbc *db.DB, filters *filter.Filter, release, testName string, since time.Time) *gorm.DB {
	q := dbc.DB.Table("test_daily_summaries tds").
		Joins("JOIN tests t ON t.id = tds.test_id").
		Where("tds.release = ?", release).
		Where("t.name = ?", testName).
		Where("tds.summary_date >= ?", since).
		Order("tds.summary_date ASC")

	allowed, blocked := extractVariantFilters(filters)
	return applyVariantCombinationFilters(q, allowed, blocked)
}

func GetTestAnalysisOverallFromDB(dbc *db.DB, filters *filter.Filter, release, testName string, reportEnd time.Time) (map[string][]CountByDate, error) {
	var rows []CountByDate
	jq := testAnalysisBaseQuery(dbc, filters, release, testName, reportEnd.Add(-24*14*time.Hour)).
		Select(selectColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.summary_date::text AS date`,
			`'overall' AS "group"`,
		)).
		Where("tds.summary_date <= ?", reportEnd).
		Group("tds.summary_date, tds.test_id, t.name, tds.release")

	r := jq.Scan(&rows)
	if r.Error != nil {
		log.WithError(r.Error).Error("error querying test analysis overall")
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

	jq := testAnalysisBaseQuery(dbc, filters, release, testName, reportEnd.Add(-24*14*time.Hour)).
		Select(selectColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.summary_date::text AS date`,
			"pj.release",
			`pj.name AS "group"`,
			"pj.variants",
		)).
		Joins("INNER JOIN prow_jobs pj ON pj.id = tds.prow_job_id").
		Where("pj.release = ?", release).
		Where("tds.summary_date <= ?", reportEnd).
		Group("tds.summary_date, tds.test_id, t.name, pj.release, pj.name, pj.variants")

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

	inner := dbc.DB.Table("test_daily_summaries tds").
		Select(selectColumns(
			"tds.test_id",
			`t.name AS test_name`,
			`tds.summary_date::text AS date`,
			`unnest(vc.variants) AS "group"`,
			"tds.release",
		)).
		Joins("JOIN tests t ON t.id = tds.test_id").
		Joins("JOIN variant_combinations vc ON vc.id = tds.variant_combination_id").
		Where("tds.release = ?", release).
		Where("t.name = ?", testName).
		Where("tds.summary_date >= ?", reportEnd.Add(-24*14*time.Hour)).
		Where("tds.summary_date <= ?", reportEnd).
		Group("t.name, t.id, tds.test_id, tds.summary_date, unnest(vc.variants), tds.release").
		Order("tds.summary_date ASC")

	vq := dbc.DB.Table("(?) AS analysis", inner).Select("*")
	allowed, blocked := extractVariantFilters(filters)
	if len(blocked) > 0 {
		vq = vq.Where(`"group" NOT IN ?`, blocked)
	}
	if len(allowed) > 0 {
		vq = vq.Where(`"group" IN ?`, allowed)
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
