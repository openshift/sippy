package pgwriter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
)

// TestRow holds raw JUnit test data before ID resolution.
type TestRow struct {
	ProwJobRunID        uint
	ProwJobID           uint
	ProwJobRunTimestamp time.Time
	ProwJobRunRelease   string
	TestName            string
	SuiteName           string
	Status              int
	Duration            float64
	Output              *string
	Lifecycle           string
}

// RunRow holds a single prow job run to be persisted.
type RunRow struct {
	ID             uint
	Cluster        string
	Duration       time.Duration
	ProwJobID      uint
	ProwJobRelease string
	URL            string
	GCSBucket      string
	Timestamp      time.Time
	OverallResult  sippyprocessingv1.JobOverallResult
	TestFailures   int
	TestFlakes     int
	Succeeded      bool
	Labels         []string
}

// AnnotationRow holds a key-value annotation for a prow job run.
type AnnotationRow struct {
	ProwJobRunID        uint
	Key                 string
	Value               string
	ProwJobRunRelease   string
	ProwJobRunTimestamp time.Time
}

// PullRequestRow holds pull request metadata to be upserted.
type PullRequestRow struct {
	Org      string
	Repo     string
	Link     string
	SHA      string
	Author   string
	Title    string
	Number   int
	MergedAt *time.Time
}

// PullRequestAssocRow links a prow job run to a pull request.
type PullRequestAssocRow struct {
	ProwJobRunID        uint
	Link                string
	SHA                 string
	ProwJobRunRelease   string
	ProwJobRunTimestamp time.Time
}

// JobRunResult aggregates all rows produced by processing a single prow job run.
type JobRunResult struct {
	Run              RunRow
	Annotations      []AnnotationRow
	PullRequests     []PullRequestRow
	PullRequestAssoc []PullRequestAssocRow
	Tests            []TestRow
}

// ErrProwJobRunAlreadyExists indicates that an idempotent parent insert did
// not win ownership because the unique Prow job run ID already exists.
var ErrProwJobRunAlreadyExists = errors.New("prow job run already exists")

var (
	runCols = []db.TempColumn[RunRow]{
		{Name: "id", Type: "bigint NOT NULL", Value: func(r *RunRow) any { return r.ID }},
		{Name: "cluster", Type: "text NOT NULL DEFAULT ''", Value: func(r *RunRow) any { return r.Cluster }},
		{Name: "duration", Type: "bigint NOT NULL DEFAULT 0", Value: func(r *RunRow) any { return int64(r.Duration) }},
		{Name: "prow_job_id", Type: "bigint NOT NULL", Value: func(r *RunRow) any { return r.ProwJobID }},
		{Name: "prow_job_release", Type: "text NOT NULL", Value: func(r *RunRow) any { return r.ProwJobRelease }},
		{Name: "url", Type: "text NOT NULL DEFAULT ''", Value: func(r *RunRow) any { return r.URL }},
		{Name: "gcs_bucket", Type: "text NOT NULL DEFAULT ''", Value: func(r *RunRow) any { return r.GCSBucket }},
		{Name: "timestamp", Type: "timestamptz NOT NULL", Value: func(r *RunRow) any { return r.Timestamp }},
		{Name: "overall_result", Type: "text NOT NULL DEFAULT ''", Value: func(r *RunRow) any { return string(r.OverallResult) }},
		{Name: "test_failures", Type: "integer NOT NULL DEFAULT 0", Value: func(r *RunRow) any { return r.TestFailures }},
		{Name: "test_flakes", Type: "integer NOT NULL DEFAULT 0", Value: func(r *RunRow) any { return r.TestFlakes }},
		{Name: "succeeded", Type: "boolean NOT NULL DEFAULT false", Value: func(r *RunRow) any { return r.Succeeded }},
		{Name: "labels", Type: "text[]", Value: func(r *RunRow) any { return r.Labels }},
	}
	annCols = []db.TempColumn[AnnotationRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(a *AnnotationRow) any { return a.ProwJobRunID }},
		{Name: "key", Type: "text NOT NULL", Value: func(a *AnnotationRow) any { return a.Key }},
		{Name: "value", Type: "text NOT NULL DEFAULT ''", Value: func(a *AnnotationRow) any { return a.Value }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(a *AnnotationRow) any { return a.ProwJobRunRelease }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(a *AnnotationRow) any { return a.ProwJobRunTimestamp }},
	}
	prCols = []db.TempColumn[PullRequestRow]{
		{Name: "org", Type: "text NOT NULL", Value: func(p *PullRequestRow) any { return p.Org }},
		{Name: "repo", Type: "text NOT NULL", Value: func(p *PullRequestRow) any { return p.Repo }},
		{Name: "link", Type: "text NOT NULL", Value: func(p *PullRequestRow) any { return p.Link }},
		{Name: "sha", Type: "text NOT NULL", Value: func(p *PullRequestRow) any { return p.SHA }},
		{Name: "author", Type: "text NOT NULL DEFAULT ''", Value: func(p *PullRequestRow) any { return p.Author }},
		{Name: "title", Type: "text NOT NULL DEFAULT ''", Value: func(p *PullRequestRow) any { return p.Title }},
		{Name: "number", Type: "integer NOT NULL DEFAULT 0", Value: func(p *PullRequestRow) any { return p.Number }},
		{Name: "merged_at", Type: "timestamptz", Value: func(p *PullRequestRow) any { return p.MergedAt }},
	}
	prAssocCols = []db.TempColumn[PullRequestAssocRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(p *PullRequestAssocRow) any { return p.ProwJobRunID }},
		{Name: "link", Type: "text NOT NULL", Value: func(p *PullRequestAssocRow) any { return p.Link }},
		{Name: "sha", Type: "text NOT NULL", Value: func(p *PullRequestAssocRow) any { return p.SHA }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(p *PullRequestAssocRow) any { return p.ProwJobRunRelease }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(p *PullRequestAssocRow) any { return p.ProwJobRunTimestamp }},
	}
	testCols = []db.TempColumn[TestRow]{
		{Name: "prow_job_run_id", Type: "bigint NOT NULL", Value: func(r *TestRow) any { return r.ProwJobRunID }},
		{Name: "prow_job_id", Type: "bigint NOT NULL", Value: func(r *TestRow) any { return r.ProwJobID }},
		{Name: "prow_job_run_timestamp", Type: "timestamptz NOT NULL", Value: func(r *TestRow) any { return r.ProwJobRunTimestamp }},
		{Name: "prow_job_run_release", Type: "text NOT NULL", Value: func(r *TestRow) any { return r.ProwJobRunRelease }},
		{Name: "test_name", Type: "text NOT NULL", Value: func(r *TestRow) any { return r.TestName }},
		{Name: "suite_name", Type: "text NOT NULL DEFAULT ''", Value: func(r *TestRow) any { return r.SuiteName }},
		{Name: "status", Type: "integer NOT NULL", Value: func(r *TestRow) any { return r.Status }},
		{Name: "duration", Type: "double precision NOT NULL DEFAULT 0", Value: func(r *TestRow) any { return r.Duration }},
		{Name: "output", Type: "text", Value: func(r *TestRow) any { return r.Output }},
		{Name: "lifecycle", Type: "text NOT NULL DEFAULT 'blocking'", Value: func(r *TestRow) any { return r.Lifecycle }},
	}
)

// Write persists a batch of job run results to the database within a single
// transaction. It creates temp tables, copies raw rows via the COPY protocol,
// then INSERTs/UPSERTs into permanent tables including tests, suites,
// prow_job_run_tests, test_daily_totals, and test_cumulative_summaries.
func Write(ctx context.Context, dbc *db.DB, currentDate civil.Date, batch []JobRunResult) error {
	return writeJobRuns(ctx, dbc, currentDate, batch, false)
}

// WriteSingleIdempotent persists one fully prepared job run. A writer that
// does not win the parent-row insert returns ErrProwJobRunAlreadyExists
// without writing children or summaries.
func WriteSingleIdempotent(ctx context.Context, dbc *db.DB, currentDate civil.Date, result JobRunResult) error {
	return writeJobRuns(ctx, dbc, currentDate, []JobRunResult{result}, true)
}

func writeJobRuns(ctx context.Context, dbc *db.DB, currentDate civil.Date, batch []JobRunResult, idempotent bool) error {
	if len(batch) == 0 {
		return nil
	}
	if idempotent && len(batch) != 1 {
		return fmt.Errorf("idempotent job run write requires exactly one result")
	}

	sqlDB, err := dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	var runs []RunRow
	var anns []AnnotationRow
	var prs []PullRequestRow
	var prAssocs []PullRequestAssocRow
	var tests []TestRow
	for i := range batch {
		runs = append(runs, batch[i].Run)
		anns = append(anns, batch[i].Annotations...)
		prs = append(prs, batch[i].PullRequests...)
		prAssocs = append(prAssocs, batch[i].PullRequestAssoc...)
		tests = append(tests, batch[i].Tests...)
	}

	copyStart := time.Now()
	cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_prow_job_runs", runs, runCols)
	if err != nil {
		return err
	}
	defer cleanup()

	if len(anns) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_annotations", anns, annCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(prs) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_pull_requests", prs, prCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(prAssocs) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_pr_assocs", prAssocs, prAssocCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if len(tests) > 0 {
		cleanup, err := db.CopyToTempTable(ctx, conn, "tmp_job_run_tests", tests, testCols)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	log.WithField("elapsed", time.Since(copyStart)).Debug("copied batch to temp tables")

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := insertJobRuns(ctx, tx, idempotent); err != nil {
		return err
	}
	if err := insertJobRunIDMap(ctx, tx); err != nil {
		return err
	}
	if err := insertAnnotations(ctx, tx, len(anns)); err != nil {
		return err
	}
	if err := upsertPullRequests(ctx, tx, len(prs)); err != nil {
		return err
	}
	if err := insertPRAssociations(ctx, tx, len(prAssocs)); err != nil {
		return err
	}
	if len(tests) > 0 {
		if err := insertTestResults(ctx, tx); err != nil {
			return err
		}
		if err := upsertSummaryTables(ctx, tx, currentDate); err != nil {
			return err
		}
	}

	stepStart := time.Now()
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing job run batch: %w", err)
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("committed transaction")

	log.WithFields(log.Fields{
		"runs":  len(batch),
		"tests": len(tests),
	}).Info("job run batch committed")

	return nil
}

func insertJobRuns(ctx context.Context, tx pgx.Tx, idempotent bool) error {
	stepStart := time.Now()
	query := `
		INSERT INTO prow_job_runs (id, cluster, duration, prow_job_id, prow_job_release,
			url, gcs_bucket, timestamp, overall_result, test_failures, test_flakes,
			succeeded, failed, infrastructure_failure, known_failure, labels, created_at, updated_at)
		SELECT id, cluster, duration, prow_job_id, prow_job_release,
			url, gcs_bucket, timestamp, overall_result, test_failures, test_flakes,
			succeeded, false, false, false, labels, NOW(), NOW()
		FROM tmp_prow_job_runs
	`
	if idempotent {
		query += `
			ON CONFLICT (id) DO NOTHING
			RETURNING id
		`
		var id uint
		if err := tx.QueryRow(ctx, query).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProwJobRunAlreadyExists
			}
			return fmt.Errorf("inserting idempotent prow_job_run: %w", err)
		}
	} else if _, err := tx.Exec(ctx, query); err != nil {
		return fmt.Errorf("inserting prow_job_runs: %w", err)
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("inserted prow_job_runs")
	return nil
}

func insertJobRunIDMap(ctx context.Context, tx pgx.Tx) error {
	stepStart := time.Now()
	if _, err := tx.Exec(ctx, `
		INSERT INTO prow_job_run_id_map (id, prow_job_release, timestamp)
		SELECT id, prow_job_release, timestamp
		FROM tmp_prow_job_runs
		ON CONFLICT (id) DO NOTHING
	`); err != nil {
		return fmt.Errorf("inserting prow_job_run_id_map: %w", err)
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("inserted prow_job_run_id_map")
	return nil
}

func insertAnnotations(ctx context.Context, tx pgx.Tx, count int) error {
	if count == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prow_job_run_annotations (prow_job_run_id, key, value,
			prow_job_run_release, prow_job_run_timestamp, created_at, updated_at)
		SELECT prow_job_run_id, key, value, prow_job_run_release, prow_job_run_timestamp, NOW(), NOW()
		FROM tmp_annotations
	`); err != nil {
		return fmt.Errorf("inserting prow_job_run_annotations: %w", err)
	}
	return nil
}

func upsertPullRequests(ctx context.Context, tx pgx.Tx, count int) error {
	if count == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prow_pull_requests (org, repo, link, sha, author, title, number, merged_at, created_at, updated_at)
		SELECT DISTINCT ON (link, sha) org, repo, link, sha, author, title, number, merged_at, NOW(), NOW()
		FROM tmp_pull_requests ORDER BY link, sha, merged_at DESC NULLS LAST
		ON CONFLICT (link, sha) DO UPDATE SET
			merged_at = COALESCE(EXCLUDED.merged_at, prow_pull_requests.merged_at),
			author = CASE WHEN prow_pull_requests.author = '' THEN EXCLUDED.author ELSE prow_pull_requests.author END,
			title = CASE WHEN prow_pull_requests.title = '' THEN EXCLUDED.title ELSE prow_pull_requests.title END,
			updated_at = NOW()
	`); err != nil {
		return fmt.Errorf("upserting prow_pull_requests: %w", err)
	}
	return nil
}

func insertPRAssociations(ctx context.Context, tx pgx.Tx, count int) error {
	if count == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prow_job_run_prow_pull_requests (prow_job_run_id, prow_pull_request_id,
			prow_job_run_release, prow_job_run_timestamp)
		SELECT tmp.prow_job_run_id, pp.id, tmp.prow_job_run_release, tmp.prow_job_run_timestamp
		FROM tmp_pr_assocs tmp
		INNER JOIN prow_pull_requests pp ON pp.link = tmp.link AND pp.sha = tmp.sha
	`); err != nil {
		return fmt.Errorf("inserting prow_job_run_prow_pull_requests: %w", err)
	}
	return nil
}

func insertTestResults(ctx context.Context, tx pgx.Tx) error {
	stepStart := time.Now()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tests (name, created_at, updated_at)
		SELECT DISTINCT test_name, NOW(), NOW() FROM tmp_job_run_tests
		ON CONFLICT (name) DO UPDATE SET deleted_at = NULL, updated_at = NOW()
		WHERE tests.deleted_at IS NOT NULL
	`); err != nil {
		return fmt.Errorf("ensuring tests exist: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO suites (name, created_at, updated_at)
		SELECT DISTINCT suite_name, NOW(), NOW() FROM tmp_job_run_tests
		WHERE suite_name != ''
		ON CONFLICT (name) DO UPDATE SET deleted_at = NULL, updated_at = NOW()
		WHERE suites.deleted_at IS NOT NULL
	`); err != nil {
		return fmt.Errorf("ensuring suites exist: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO prow_job_run_tests (prow_job_run_id, prow_job_id, prow_job_run_timestamp,
				prow_job_run_release, test_id, suite_id, status, duration, lifecycle, created_at, updated_at)
			SELECT tmp.prow_job_run_id, tmp.prow_job_id, tmp.prow_job_run_timestamp,
				tmp.prow_job_run_release, t.id, s.id, tmp.status, tmp.duration, tmp.lifecycle, NOW(), NOW()
			FROM tmp_job_run_tests tmp
			INNER JOIN tests t ON t.name = tmp.test_name AND t.deleted_at IS NULL
			LEFT JOIN suites s ON s.name = tmp.suite_name AND s.deleted_at IS NULL
			RETURNING id, test_id, suite_id,
				prow_job_run_id, prow_job_run_timestamp, prow_job_run_release
		)
		INSERT INTO prow_job_run_test_outputs (prow_job_run_test_id, prow_job_run_test_timestamp,
			prow_job_run_test_release, output, created_at, updated_at)
		SELECT ins.id, ins.prow_job_run_timestamp, ins.prow_job_run_release, tmp.output, NOW(), NOW()
		FROM inserted ins
		JOIN tests t ON t.id = ins.test_id
		JOIN tmp_job_run_tests tmp ON tmp.test_name = t.name AND tmp.prow_job_run_id = ins.prow_job_run_id
		LEFT JOIN suites s2 ON s2.name = tmp.suite_name AND s2.deleted_at IS NULL
		WHERE tmp.output IS NOT NULL
			AND ins.suite_id IS NOT DISTINCT FROM s2.id
	`); err != nil {
		return fmt.Errorf("inserting prow_job_run_tests: %w", err)
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("inserted prow_job_run_tests + outputs")
	return nil
}

type releaseDate struct {
	release string
	date    civil.Date
}

func upsertSummaryTables(ctx context.Context, tx pgx.Tx, currentDate civil.Date) error {
	if err := createBatchDeltas(ctx, tx); err != nil {
		return err
	}

	tomorrow := currentDate.AddDays(1)
	var hasFuture bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM batch_deltas WHERE batch_date > $1)`, tomorrow).Scan(&hasFuture); err != nil {
		return fmt.Errorf("checking for future-dated batch rows: %w", err)
	}
	if hasFuture {
		return fmt.Errorf("batch contains test results dated after %s; refusing to write summaries", tomorrow)
	}

	releaseDates, err := queryReleaseDates(ctx, tx)
	if err != nil {
		return err
	}

	stepStart := time.Now()
	for _, rd := range releaseDates {
		if err := ensureDailyTotalRows(ctx, tx, rd.date, rd.release); err != nil {
			return err
		}
		if err := updateDailyTotals(ctx, tx, rd.date, rd.release); err != nil {
			return err
		}
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("upserted test_daily_totals")

	stepStart = time.Now()
	releaseMinDate := make(map[string]civil.Date)
	for _, rd := range releaseDates {
		if earliest, ok := releaseMinDate[rd.release]; !ok || rd.date.Before(earliest) {
			releaseMinDate[rd.release] = rd.date
		}
	}
	for release, minDate := range releaseMinDate {
		for day := minDate; !day.After(tomorrow); day = day.AddDays(1) {
			if err := ensureCumulativeSummaryRows(ctx, tx, day, release); err != nil {
				return err
			}
			if err := updateCumulativeSummaries(ctx, tx, day, release); err != nil {
				return err
			}
		}
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("upserted test_cumulative_summaries")
	return nil
}

func createBatchDeltas(ctx context.Context, tx pgx.Tx) error {
	stepStart := time.Now()
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE batch_deltas ON COMMIT DROP AS
		SELECT
			t.id AS test_id, tmp.prow_job_id, COALESCE(s.id, 0) AS suite_id,
			tmp.lifecycle,
			tmp.prow_job_run_release AS release,
			date(tmp.prow_job_run_timestamp) AS batch_date,
			COUNT(*) FILTER (WHERE tmp.status = 1) AS successes,
			COUNT(*) FILTER (WHERE tmp.status = 12) AS failures,
			COUNT(*) FILTER (WHERE tmp.status = 13) AS flakes,
			COUNT(*) AS runs,
			MIN(tmp.prow_job_run_timestamp) FILTER (WHERE tmp.status = 12) AS min_failure_ts,
			MAX(tmp.prow_job_run_timestamp) FILTER (WHERE tmp.status = 12) AS max_failure_ts,
			MIN(tmp.prow_job_run_timestamp) FILTER (WHERE tmp.status = 1) AS min_success_ts,
			MAX(tmp.prow_job_run_timestamp) FILTER (WHERE tmp.status = 1) AS max_success_ts
		FROM tmp_job_run_tests tmp
		INNER JOIN tests t ON t.name = tmp.test_name AND t.deleted_at IS NULL
		INNER JOIN tmp_prow_job_runs r ON r.id = tmp.prow_job_run_id
			AND r.prow_job_release = tmp.prow_job_run_release
			AND r.timestamp = tmp.prow_job_run_timestamp
		LEFT JOIN suites s ON s.name = tmp.suite_name AND s.deleted_at IS NULL
		WHERE (r.labels IS NULL OR NOT (r.labels @> ARRAY['InfraFailure']))
		GROUP BY t.id, tmp.prow_job_id, COALESCE(s.id, 0), tmp.lifecycle,
			tmp.prow_job_run_release, date(tmp.prow_job_run_timestamp)
	`); err != nil {
		return fmt.Errorf("materializing batch deltas: %w", err)
	}
	log.WithField("elapsed", time.Since(stepStart)).Debug("materialized batch_deltas")
	return nil
}

func queryReleaseDates(ctx context.Context, tx pgx.Tx) ([]releaseDate, error) {
	rows, err := tx.Query(ctx, `
		SELECT release, batch_date FROM batch_deltas GROUP BY release, batch_date
	`)
	if err != nil {
		return nil, fmt.Errorf("querying batch delta release-dates: %w", err)
	}
	defer rows.Close()
	var releaseDates []releaseDate
	for rows.Next() {
		var release string
		var date time.Time
		if err := rows.Scan(&release, &date); err != nil {
			return nil, fmt.Errorf("scanning release-date: %w", err)
		}
		releaseDates = append(releaseDates, releaseDate{release: release, date: civil.DateOf(date)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating release-dates: %w", err)
	}
	return releaseDates, nil
}

func ensureDailyTotalRows(ctx context.Context, tx pgx.Tx, day civil.Date, release string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO test_daily_totals (test_id, prow_job_id, suite_id, lifecycle, release, date,
			successes, failures, flakes, runs)
		SELECT bd.test_id, bd.prow_job_id, bd.suite_id, bd.lifecycle, $2, $1, 0, 0, 0, 0
		FROM batch_deltas bd
		WHERE bd.release = $2 AND bd.batch_date = $1
			AND NOT EXISTS (
				SELECT 1 FROM test_daily_totals dt
				WHERE dt.test_id = bd.test_id
					AND dt.prow_job_id = bd.prow_job_id
					AND dt.suite_id = bd.suite_id
					AND dt.lifecycle = bd.lifecycle
					AND dt.release = $2
					AND dt.date = $1
			)
	`, day, release); err != nil {
		return fmt.Errorf("ensuring test_daily_totals rows for release %s date %s: %w", release, day, err)
	}
	return nil
}

func updateDailyTotals(ctx context.Context, tx pgx.Tx, day civil.Date, release string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE test_daily_totals dt SET
			successes = dt.successes + bd.successes,
			failures = dt.failures + bd.failures,
			flakes = dt.flakes + bd.flakes,
			runs = dt.runs + bd.runs,
			first_failure_timestamp = LEAST(dt.first_failure_timestamp, bd.min_failure_ts),
			last_failure_timestamp = GREATEST(dt.last_failure_timestamp, bd.max_failure_ts),
			first_success_timestamp = LEAST(dt.first_success_timestamp, bd.min_success_ts),
			last_success_timestamp = GREATEST(dt.last_success_timestamp, bd.max_success_ts)
		FROM batch_deltas bd
		WHERE bd.release = $2 AND bd.batch_date = $1
			AND dt.test_id = bd.test_id
			AND dt.prow_job_id = bd.prow_job_id
			AND dt.suite_id = bd.suite_id
			AND dt.lifecycle = bd.lifecycle
			AND dt.release = $2
			AND dt.date = $1
	`, day, release); err != nil {
		return fmt.Errorf("updating test_daily_totals for release %s date %s: %w", release, day, err)
	}
	return nil
}

func ensureCumulativeSummaryRows(ctx context.Context, tx pgx.Tx, day civil.Date, release string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO test_cumulative_summaries (date, test_id, prow_job_id, suite_id, lifecycle, release,
			prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs)
		SELECT $1, bd.test_id, bd.prow_job_id, bd.suite_id, bd.lifecycle, $2, 0, 0, 0, 0
		FROM batch_deltas bd
		WHERE bd.release = $2 AND bd.batch_date <= $1
			AND NOT EXISTS (
				SELECT 1 FROM test_cumulative_summaries cs
				WHERE cs.date = $1
					AND cs.release = $2
					AND cs.test_id = bd.test_id
					AND cs.prow_job_id = bd.prow_job_id
					AND cs.suite_id = bd.suite_id
					AND cs.lifecycle = bd.lifecycle
			)
		GROUP BY bd.test_id, bd.prow_job_id, bd.suite_id, bd.lifecycle
	`, day, release); err != nil {
		return fmt.Errorf("ensuring test_cumulative_summaries rows for release %s date %s: %w", release, day, err)
	}
	return nil
}

func updateCumulativeSummaries(ctx context.Context, tx pgx.Tx, day civil.Date, release string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE test_cumulative_summaries cs SET
			prefix_sum_successes = cs.prefix_sum_successes + bd.sum_successes,
			prefix_sum_failures = cs.prefix_sum_failures + bd.sum_failures,
			prefix_sum_flakes = cs.prefix_sum_flakes + bd.sum_flakes,
			prefix_sum_runs = cs.prefix_sum_runs + bd.sum_runs,
			prefix_max_last_failure = GREATEST(cs.prefix_max_last_failure, bd.max_last_failure),
			prefix_max_last_success = GREATEST(cs.prefix_max_last_success, bd.max_last_success)
		FROM (
			SELECT test_id, prow_job_id, suite_id, lifecycle,
				SUM(successes) AS sum_successes, SUM(failures) AS sum_failures,
				SUM(flakes) AS sum_flakes, SUM(runs) AS sum_runs,
				MAX(max_failure_ts) AS max_last_failure,
				MAX(max_success_ts) AS max_last_success
			FROM batch_deltas
			WHERE release = $2 AND batch_date <= $1
			GROUP BY test_id, prow_job_id, suite_id, lifecycle
		) bd
		WHERE cs.date = $1
			AND cs.release = $2
			AND cs.test_id = bd.test_id
			AND cs.prow_job_id = bd.prow_job_id
			AND cs.suite_id = bd.suite_id
			AND cs.lifecycle = bd.lifecycle
	`, day, release); err != nil {
		return fmt.Errorf("updating test_cumulative_summaries for release %s date %s: %w", release, day, err)
	}
	return nil
}

// CarryForwardCumulativeSummaries fills any gap days between the latest
// existing cumulative summary data and currentDate+1 (tomorrow) for each
// release. If the loader was down for multiple days, this catches up all
// missed days. Subsequent calls for the same date range are no-ops.
// Releases are processed in parallel with up to 4 workers.
func CarryForwardCumulativeSummaries(ctx context.Context, dbc *db.DB, currentDate civil.Date, releases []string) error {
	tomorrow := currentDate.AddDays(1)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	for _, release := range releases {
		g.Go(func() error {
			return carryForwardRelease(ctx, dbc, release, tomorrow)
		})
	}

	return g.Wait()
}

func carryForwardRelease(ctx context.Context, dbc *db.DB, release string, tomorrow civil.Date) error {
	var hasData bool
	if err := dbc.DB.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM test_cumulative_summaries
			WHERE release = ?
		)`, release).Scan(&hasData).Error; err != nil {
		return fmt.Errorf("checking data existence for release %s: %w", release, err)
	}
	if !hasData {
		log.WithField("release", release).Debug("no cumulative summary data exists, skipping carry-forward")
		return nil
	}

	latestDate, err := findLatestDateWithData(ctx, dbc, release, tomorrow)
	if err != nil {
		return fmt.Errorf("finding latest date for release %s: %w", release, err)
	}

	nextDay := latestDate.AddDays(1)
	if nextDay.After(tomorrow) {
		log.WithField("release", release).Debug("cumulative summaries already up to date through tomorrow")
		return nil
	}

	start := time.Now()
	var totalRows int64
	for day := nextDay; !day.After(tomorrow); day = day.AddDays(1) {
		prevDay := day.AddDays(-1)
		result := dbc.DB.WithContext(ctx).Exec(`
			INSERT INTO test_cumulative_summaries (date, test_id, prow_job_id, suite_id, lifecycle, release,
				prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs,
				prefix_max_last_failure, prefix_max_last_success)
			SELECT
				?, test_id, prow_job_id, suite_id, lifecycle, release,
				prefix_sum_successes, prefix_sum_failures, prefix_sum_flakes, prefix_sum_runs,
				prefix_max_last_failure, prefix_max_last_success
			FROM test_cumulative_summaries
			WHERE date = ? AND release = ?`, day, prevDay, release)
		if result.Error != nil {
			return fmt.Errorf("carrying forward cumulative summaries for release %s to %s: %w", release, day, result.Error)
		}
		totalRows += result.RowsAffected
	}
	log.WithFields(log.Fields{
		"release": release,
		"rows":    totalRows,
		"from":    nextDay,
		"through": tomorrow,
		"elapsed": time.Since(start),
	}).Info("carried forward cumulative summaries")
	return nil
}

// findLatestDateWithData walks backwards from startDate looking for the most
// recent date that has cumulative summary rows for the given release. Each
// EXISTS check targets a single partition, avoiding a full MAX(date) scan.
// Returns an error if no data is found within maxLookbackDays.
func findLatestDateWithData(ctx context.Context, dbc *db.DB, release string, startDate civil.Date) (civil.Date, error) {
	const maxLookbackDays = 30
	earliest := startDate.AddDays(-maxLookbackDays)
	for day := startDate; !day.Before(earliest); day = day.AddDays(-1) {
		var exists bool
		if err := dbc.DB.WithContext(ctx).Raw(`
			SELECT EXISTS(
				SELECT 1 FROM test_cumulative_summaries
				WHERE date = ? AND release = ?
			)`, day, release).Scan(&exists).Error; err != nil {
			return civil.Date{}, fmt.Errorf("checking date %s: %w", day, err)
		}
		if exists {
			return day, nil
		}
	}
	return civil.Date{}, fmt.Errorf("no cumulative summary data found for release %s within %d days of %s", release, maxLookbackDays, startDate)
}
