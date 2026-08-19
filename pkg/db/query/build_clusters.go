package query

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func HasBuildClusterData(dbc *db.DB, release string) (bool, error) {
	count := int64(0)
	res := dbc.DB.Table("prow_job_runs").
		Where(`cluster != '' AND cluster IS NOT NULL`).
		Where("prow_job_release = ?", release).
		Where("timestamp > NOW() - INTERVAL '14 days'").
		Count(&count)
	return count > 0, res.Error
}

func BuildClusterHealth(dbc *db.DB, release string, start, boundary, end time.Time) ([]models.BuildClusterHealthReport, error) {
	results := make([]models.BuildClusterHealthReport, 0)

	rawResults := dbc.DB.Select(`
		ROW_NUMBER() OVER() AS id,
		cluster,
		coalesce(count(case when succeeded = true AND timestamp >= @start AND timestamp < @boundary then 1 end), 0) as previous_passes,
		coalesce(count(case when succeeded = false AND timestamp >= @start AND timestamp < @boundary then 1 end), 0) as previous_fails,
		coalesce(count(case when timestamp >= @start AND timestamp < @boundary then 1 end), 0) as previous_runs,
		coalesce(count(case when succeeded = true AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_passes,
		coalesce(count(case when succeeded = false AND timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_fails,
		coalesce(count(case when timestamp BETWEEN @boundary AND @end then 1 end), 0) as current_runs
`, sql.Named("start", start), sql.Named("boundary", boundary), sql.Named("end", end)).
		Table("prow_job_runs").
		Joins("JOIN prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id").
		Where(`cluster != '' AND cluster IS NOT NULL`).
		Where("prow_jobs.kind = 'periodic'").
		Where("prow_job_runs.prow_job_release = ?", release).
		Where("prow_job_runs.timestamp BETWEEN @start AND @end", sql.Named("start", start), sql.Named("end", end)).
		Group("cluster")

	q := dbc.DB.Table("(?) as results", rawResults).
		Select(`*,
		current_passes * 100.0 / NULLIF(current_runs, 0) AS current_pass_percentage,
       previous_passes * 100.0 / NULLIF(previous_runs, 0) AS previous_pass_percentage,
       (current_passes * 100.0 / NULLIF(current_runs, 0)) - (previous_passes * 100.0 / NULLIF(previous_runs, 0)) AS net_improvement
`).Scan(&results)

	return results, q.Error
}

func BuildClusterAnalysis(dbc *db.DB, release, period string) ([]models.BuildClusterHealth, error) {
	results := make([]models.BuildClusterHealth, 0)

	q := dbc.DB.Raw(fmt.Sprintf(`
SELECT
    cluster,
    date_trunc('%s', timestamp) as period,
    count(*) AS total_runs,
    sum(case when overall_result = 'S' then 1 else 0 end) AS passes,
    sum(case when overall_result != 'S' then 1 else 0 end) AS failures,
    sum(case when overall_result = 'S' then 1 else 0 end) * 100.0 / count(*) AS pass_percentage
FROM
    prow_job_runs
JOIN
	prow_jobs ON prow_job_runs.prow_job_id = prow_jobs.id
WHERE
    cluster is not null
AND
	prow_jobs.kind = 'periodic'
AND
    prow_job_runs.prow_job_release = @release
AND
    cluster != ''
AND
    timestamp > NOW() - INTERVAL '14 DAY'
GROUP BY cluster, period
`, period), sql.Named("release", release)).Scan(&results)
	return results, q.Error
}
