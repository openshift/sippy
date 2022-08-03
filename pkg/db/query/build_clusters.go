package query

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func HasBuildClusterData(dbc *db.DB) (bool, error) {
	count := int64(0)
	res := dbc.DB.Table("prow_job_runs").Where(`cluster != '' AND cluster IS NOT NULL`).Count(&count)
	return count > 0, res.Error
}

func BuildClusterHealth(db *db.DB, start, boundary, end time.Time) ([]models.BuildClusterHealthReport, error) {
	results := make([]models.BuildClusterHealthReport, 0)

	rawResults := db.DB.Select(`
		ROW_NUMBER() OVER() AS id,
		cluster,
		coalesce(count(case when succeeded = true AND timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_passes,
		coalesce(count(case when succeeded = false AND timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_failures,
		coalesce(count(case when timestamp BETWEEN @start AND @boundary then 1 end), 0) as previous_runs,
		coalesce(count(case when succeeded = true AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_passes,
		coalesce(count(case when succeeded = false AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_fails,
		coalesce(count(case when timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_runs
`, sql.Named("start", start), sql.Named("boundary", boundary), sql.Named("end", end)).
		Table("prow_job_runs").
		Where(`cluster != '' AND cluster IS NOT NULL`).
		Group("cluster")

	q := db.DB.Table("(?) as results", rawResults).
		Select(`*,
		current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
`).Scan(&results)

	return results, q.Error
}

func BuildClusterAnalysis(db *db.DB, period string) ([]models.BuildClusterHealth, error) {
	results := make([]models.BuildClusterHealth, 0)

	q := db.DB.Raw(fmt.Sprintf(`
WITH results AS (
SELECT
    cluster,
    date_trunc('%s', timestamp) as period,
    count(*) AS total_runs,
    sum(case when overall_result = 'S' then 1 else 0 end) AS passes,
    sum(case when overall_result != 'S' then 1 else 0 end) AS failures,
    sum(case when overall_result = 'S' then 1 else 0 end) * 100.0 / count(*) AS pass_percentage
FROM
    prow_job_runs
WHERE
    cluster is not null
AND
    cluster != ''
AND
    timestamp > NOW() - INTERVAL '14 DAY'
GROUP BY cluster, period),
percentages AS (
    SELECT
        period,
        sum(passes) * 100.0 / sum(total_runs) as mean_success
    FROM results
    GROUP BY period
)
SELECT
    results.cluster,
    results.period,
    results.total_runs,
    results.passes,
    results.failures,
    results.pass_percentage
FROM
    results
LEFT JOIN
    percentages on results.period = percentages.period
`, period)).Scan(&results)
	return results, q.Error
}
