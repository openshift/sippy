package gateststatus

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// GATestStatusLoader populates prow_ga_raw_test_data for releases that have reached GA.
//
// The loader has two conceptual phases:
//  1. Fetch: for each GA release, query BigQuery once for all configured windows
//     and persist the raw results in prow_ga_raw_test_data. This runs only when
//     the raw data is missing or the GA date changed, or when forced.
//  2. Aggregate: handled by the prow_ga_test_statuses_matview, which joins
//     raw data with current dimension tables on each refresh cycle.
type GATestStatusLoader struct {
	ctx      context.Context
	dbc      *db.DB
	bqClient *bqcachedclient.Client
	force    bool
	releases []string
	errs     []error
}

func New(ctx context.Context, dbc *db.DB, bqClient *bqcachedclient.Client, force bool, releases []string) *GATestStatusLoader {
	return &GATestStatusLoader{
		ctx:      ctx,
		dbc:      dbc,
		bqClient: bqClient,
		force:    force,
		releases: releases,
	}
}

func (l *GATestStatusLoader) Name() string    { return "ga-test-status" }
func (l *GATestStatusLoader) Errors() []error { return l.errs }

func (l *GATestStatusLoader) Load() {
	start := time.Now()

	allReleases, err := l.gaReleases()
	if err != nil {
		l.errs = append(l.errs, err)
		return
	}
	if len(allReleases) == 0 {
		log.Info("ga-test-status: no GA releases found, skipping")
		return
	}

	loaded := 0
	for _, rel := range allReleases {
		gaDate := *rel.GADate
		if !l.force && rel.LoadedGADate != nil && *rel.LoadedGADate == gaDate {
			log.WithField("release", rel.Release).Debug("ga-test-status: already loaded, skipping")
			continue
		}
		if err := l.loadRelease(rel.Release, gaDate); err != nil {
			l.errs = append(l.errs, fmt.Errorf("release %s: %w", rel.Release, err))
		} else {
			loaded++
		}
	}

	log.WithField("elapsed", time.Since(start)).
		WithField("loaded", loaded).
		Info("ga-test-status: load complete")
}

func (l *GATestStatusLoader) loadRelease(release string, gaDate civil.Date) error {
	rLog := log.WithField("release", release)
	gaEnd := utils.GAWindowEnd(gaDate)

	var windows []releaseWindow
	for _, windowDays := range utils.GAWindows {
		windows = append(windows, releaseWindow{
			Release:    release,
			WindowDays: int64(windowDays),
			StartDate:  utils.GAWindowStart(gaDate, windowDays),
			EndDate:    gaEnd,
		})
	}

	bqStart := time.Now()
	rLog.Info("ga-test-status: fetching all windows from BigQuery")
	rows, err := l.fetchFromBigQuery(windows)
	if err != nil {
		return fmt.Errorf("fetching from BigQuery: %w", err)
	}
	rLog.WithField("bq_rows", len(rows)).
		WithField("bq_elapsed", time.Since(bqStart)).
		Info("ga-test-status: fetched from BigQuery")

	pgStart := time.Now()
	if err := l.persist(release, gaDate, rows); err != nil {
		return fmt.Errorf("persisting: %w", err)
	}
	rLog.WithField("pg_elapsed", time.Since(pgStart)).
		Info("ga-test-status: persisted to Postgres")

	return nil
}

func (l *GATestStatusLoader) persist(release string, gaDate civil.Date, rows []stagingRow) error {
	sqlDB, err := l.dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if releaseErr := stdlib.ReleaseConn(sqlDB, conn); releaseErr != nil {
			log.WithError(releaseErr).Error("failed to release pgx conn")
		}
	}()

	if len(rows) > 0 {
		copyStart := time.Now()
		cleanup, err := db.CopyToTempTable(l.ctx, conn, "tmp_ga_raw", rows, tempCols)
		if err != nil {
			return fmt.Errorf("copying to temp table: %w", err)
		}
		defer cleanup()
		log.WithField("rows", len(rows)).
			WithField("elapsed", time.Since(copyStart)).
			Info("ga-test-status: copied to temp table")
	}

	tx, err := conn.Begin(l.ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(l.ctx); rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
			log.WithError(rollbackErr).Error("failed to rollback transaction")
		}
	}()

	if _, err := tx.Exec(l.ctx,
		"DELETE FROM prow_ga_raw_test_data WHERE release = $1", release); err != nil {
		return fmt.Errorf("deleting existing raw rows: %w", err)
	}

	if len(rows) > 0 {
		insertStart := time.Now()
		result, err := tx.Exec(l.ctx, `
			INSERT INTO prow_ga_raw_test_data
				(release, window_days, test_id, prow_job_id, suite_id, passes, failures, flakes, runs)
			SELECT
				$1, tmp.window_days, t.id, pj.id, COALESCE(s.id, 0),
				tmp.passes, tmp.failures, tmp.flakes, tmp.runs
			FROM tmp_ga_raw tmp
			INNER JOIN tests t ON t.name = tmp.test_name AND t.deleted_at IS NULL
			INNER JOIN prow_jobs pj ON pj.name = tmp.job_name AND pj.deleted_at IS NULL
			LEFT JOIN suites s ON s.name = tmp.suite_name AND s.deleted_at IS NULL
		`, release)
		if err != nil {
			return fmt.Errorf("INSERT...SELECT from temp table: %w", err)
		}
		log.WithField("rows", result.RowsAffected()).
			WithField("elapsed", time.Since(insertStart)).
			Info("ga-test-status: inserted from temp table")
	}

	if _, err := tx.Exec(l.ctx,
		"UPDATE release_definitions SET ga_data_loaded_date = $1 WHERE release = $2",
		gaDate, release); err != nil {
		return fmt.Errorf("recording load status: %w", err)
	}

	commitStart := time.Now()
	if err := tx.Commit(l.ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	log.WithField("elapsed", time.Since(commitStart)).Info("ga-test-status: committed")

	return nil
}

var tempCols = []db.TempColumn[stagingRow]{
	{Name: "window_days", Type: "integer", Value: func(r *stagingRow) any { return int(r.WindowDays) }},
	{Name: "test_name", Type: "text", Value: func(r *stagingRow) any { return r.TestName }},
	{Name: "job_name", Type: "text", Value: func(r *stagingRow) any { return r.JobName }},
	{Name: "suite_name", Type: "text", Value: func(r *stagingRow) any { return r.Suite }},
	{Name: "passes", Type: "bigint", Value: func(r *stagingRow) any { return r.Passes }},
	{Name: "failures", Type: "bigint", Value: func(r *stagingRow) any { return r.Failures }},
	{Name: "flakes", Type: "bigint", Value: func(r *stagingRow) any { return r.Flakes }},
	{Name: "runs", Type: "bigint", Value: func(r *stagingRow) any { return r.Runs }},
}

func (l *GATestStatusLoader) gaReleases() ([]models.ReleaseDefinition, error) {
	var defs []models.ReleaseDefinition
	query := l.dbc.DB.WithContext(l.ctx).Where("ga_date < CURRENT_DATE")
	if len(l.releases) > 0 {
		query = query.Where("release IN ?", l.releases)
	}
	if err := query.Find(&defs).Error; err != nil {
		return nil, fmt.Errorf("querying release_definitions: %w", err)
	}
	return defs, nil
}

func (l *GATestStatusLoader) fetchFromBigQuery(windows []releaseWindow) ([]stagingRow, error) {
	earliestStart := windows[0].StartDate
	latestEnd := windows[0].EndDate
	for _, w := range windows[1:] {
		if w.StartDate.Before(earliestStart) {
			earliestStart = w.StartDate
		}
		if w.EndDate.After(latestEnd) {
			latestEnd = w.EndDate
		}
	}

	query := l.bqClient.Query(l.ctx, bqlabel.GATestStatusLoader, fmt.Sprintf(`
		WITH params AS (
			SELECT * FROM UNNEST(@windows)
		),
		deduped AS (
			SELECT
				p.release,
				p.window_days,
				junit.test_name,
				jobs.prowjob_job_name AS job_name,
				COALESCE(junit.testsuite, '') AS suite_name,
				CASE WHEN junit.flake_count > 0 THEN 0 ELSE junit.success_val END AS adjusted_success,
				CASE WHEN junit.flake_count > 0 THEN 1 ELSE 0 END AS is_flake,
				ROW_NUMBER() OVER(
					PARTITION BY p.release, p.window_days, junit.file_path, junit.test_name, junit.testsuite
					ORDER BY CASE WHEN junit.flake_count > 0 THEN 0 WHEN junit.success_val > 0 THEN 1 ELSE 2 END
				) AS row_num
			FROM params p
			JOIN %[1]s.junit ON junit.release = p.release
				AND junit.modified_time >= DATETIME(@earliest_start)
				AND junit.modified_time < DATETIME(@latest_end)
				AND junit.modified_time >= DATETIME(p.start_date)
				AND junit.modified_time < DATETIME(p.end_date)
				AND junit.skipped = FALSE
			JOIN %[1]s.jobs jobs ON junit.prowjob_build_id = jobs.prowjob_build_id
				AND jobs.prowjob_start >= DATETIME(p.start_date)
				AND jobs.prowjob_start < DATETIME(p.end_date)
			JOIN %[1]s.job_variants jv ON jobs.prowjob_job_name = jv.job_name
				AND jv.variant_name = 'Release' AND jv.variant_value = p.release
		)
		SELECT
			release,
			window_days,
			test_name,
			job_name,
			suite_name,
			SUM(adjusted_success) AS passes,
			SUM(CASE WHEN adjusted_success = 0 AND is_flake = 0 THEN 1 ELSE 0 END) AS failures,
			SUM(is_flake) AS flakes,
			COUNT(*) AS runs
		FROM deduped
		WHERE row_num = 1
		GROUP BY release, window_days, test_name, job_name, suite_name
	`, l.bqClient.Dataset))
	query.Parameters = []bigquery.QueryParameter{
		{Name: "windows", Value: windows},
		{Name: "earliest_start", Value: earliestStart},
		{Name: "latest_end", Value: latestEnd},
	}

	iter, err := query.Read(l.ctx)
	if err != nil {
		return nil, fmt.Errorf("executing query: %w", err)
	}

	var result []stagingRow
	for {
		var row stagingRow
		err := iter.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading row: %w", err)
		}
		result = append(result, row)
	}
	return result, nil
}

// releaseWindow defines a single (release, window) combination for the batched BQ query.
// Passed as an ARRAY<STRUCT<...>> parameter.
type releaseWindow struct {
	Release    string     `bigquery:"release"`
	WindowDays int64      `bigquery:"window_days"`
	StartDate  civil.Date `bigquery:"start_date"`
	EndDate    civil.Date `bigquery:"end_date"`
}

type stagingRow struct {
	Release    string `bigquery:"release"`
	WindowDays int64  `bigquery:"window_days"`
	TestName   string `bigquery:"test_name"`
	JobName    string `bigquery:"job_name"`
	Suite      string `bigquery:"suite_name"`
	Passes     int64  `bigquery:"passes"`
	Failures   int64  `bigquery:"failures"`
	Flakes     int64  `bigquery:"flakes"`
	Runs       int64  `bigquery:"runs"`
}
